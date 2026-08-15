package tts

import (
	"fmt"
	"math"
	"strings"

	"utautts/internal/audio"
	"utautts/internal/connection"
	"utautts/internal/frontend"
	"utautts/internal/openjtalk"
	"utautts/internal/plan"
	"utautts/internal/plugin"
	"utautts/internal/prosody"
	"utautts/internal/render"
	"utautts/internal/voicebank"
)

type Config struct {
	VoicebankPath           string
	Voicebank               *voicebank.Bank
	Text                    string
	Reading                 string
	Dictionary              map[string]string
	Tone                    string
	MoraDurationMS          float64
	PauseDurationMS         float64
	ReleaseMS               float64
	ProsodyModelPath        string
	ProsodyModel            *prosody.Model
	ManualPitchPath         string
	ManualPitch             *prosody.ManualPitchFile
	ProsodyFeatures         []prosody.FeatureFrame
	ProsodyPitchOnly        bool
	OpenJTalkPath           string
	OpenJTalkDictionaryPath string
	PitchFactors            []float64
	ApplyPitch              bool
	IntonationStrength      float64
	Renderer                string
	RendererCapabilities    *plugin.Capabilities
	WorldlinePath           string
	WorldlineBridgePath     string
	WorldlineR2MelPath      string
	WorldlineR2VocoderPath  string
	OnnxDeviceID            int
	UTAUResamplerPath       string
	BoundaryBridgeMS        float64
	BoundaryBridgeThreshold float64
	PitchCurve              *render.PitchCurve
	SelectionMode           voicebank.SelectionMode
	JoinModelPath           string
	JoinScoreScale          float64
}

type Result struct {
	Voicebank *voicebank.Bank
	Plan      *plan.Plan
	Audio     *audio.PCM
}

func Synthesize(cfg Config) (*Result, error) {
	if err := validateConfig(cfg); err != nil {
		return nil, err
	}
	bank := cfg.Voicebank
	var err error
	if bank == nil {
		bank, err = loadVoicebankCached(cfg.VoicebankPath)
		if err != nil {
			return nil, fmt.Errorf("load voicebank: %w", err)
		}
	}
	loadedProsody := cfg.ProsodyModel
	prosodyFeatures := cfg.ProsodyFeatures
	var runtimeFeatures *openjtalk.Analysis
	if loadedProsody == nil && cfg.ProsodyModelPath != "" {
		loadedProsody, err = loadProsodyModelCached(cfg.ProsodyModelPath)
		if err != nil {
			return nil, fmt.Errorf("load prosody model: %w", err)
		}
	}
	reading := cfg.Reading
	if reading == "" {
		reading, err = frontend.ToKanaWithDictionary(cfg.Text, cfg.Dictionary)
		if err != nil {
			return nil, fmt.Errorf("convert text to reading: %w", err)
		}
	}
	morae, err := frontend.ParseKana(reading)
	if err != nil {
		return nil, fmt.Errorf("parse reading: %w", err)
	}
	if loadedProsody != nil && loadedProsody.RequiresExternalFeatures() && len(prosodyFeatures) == 0 {
		runtimeText := frontend.ApplyDictionary(cfg.Text, cfg.Dictionary)
		runtimeFeatures, err = analyzeOpenJTalkCached(runtimeText, openjtalk.Config{
			HelperPath: cfg.OpenJTalkPath, DictionaryPath: cfg.OpenJTalkDictionaryPath,
		})
		if err != nil {
			return nil, fmt.Errorf("analyze runtime prosody features: %w", err)
		}
		prosodyFeatures, err = alignRuntimeProsodyFeatures(morae, runtimeFeatures)
		if err != nil {
			return nil, fmt.Errorf("align runtime prosody features: %w", err)
		}
	}
	var joinModel *connection.LearnedModel
	joinCostMode := "handcrafted"
	joinModelVersion := 0
	if cfg.JoinModelPath != "" {
		joinModel, err = connection.LoadLearnedModel(cfg.JoinModelPath)
		if err != nil {
			return nil, fmt.Errorf("load join model: %w", err)
		}
		joinCostMode = "learned"
		joinModelVersion = joinModel.Version
		if cfg.JoinScoreScale > 0 {
			joinModel.ScoreScale = cfg.JoinScoreScale
		}
	}
	if cfg.SelectionMode == voicebank.SelectionTargetOnly {
		joinCostMode = "none"
	}
	selections, err := bank.ResolveWithConfig(morae, voicebank.ResolveConfig{
		Tone: cfg.Tone, Mode: cfg.SelectionMode, JoinModel: joinModel,
	})
	if err != nil {
		return nil, fmt.Errorf("resolve voicebank units: %w", err)
	}
	var predictions []prosody.Prediction
	if loadedProsody != nil {
		if loadedProsody.RequiresExternalFeatures() && len(prosodyFeatures) != len(morae) {
			return nil, fmt.Errorf("prosody model %d/%s requires %d mora-level accent feature frames, got %d", loadedProsody.Version, loadedProsody.Mode, len(morae), len(prosodyFeatures))
		}
		predictions = loadedProsody.PredictWithFeatures(morae, prosodyFeatures)
		if cfg.ProsodyPitchOnly {
			for i := range predictions {
				predictions[i].DurationMS = 0
				predictions[i].DurationFactor = 1
				predictions[i].EnergyFactor = 1
			}
		}
	}
	if len(cfg.PitchFactors) > 0 {
		if len(cfg.PitchFactors) != len(morae) {
			return nil, fmt.Errorf("pitch factors: got %d values for %d morae", len(cfg.PitchFactors), len(morae))
		}
		if len(predictions) == 0 {
			predictions = make([]prosody.Prediction, len(morae))
			for i := range predictions {
				predictions[i].DurationFactor = 1
				predictions[i].EnergyFactor = 1
			}
		}
		for i, factor := range cfg.PitchFactors {
			if factor <= 0 {
				return nil, fmt.Errorf("pitch factors: value %d is %.4f, want positive", i, factor)
			}
			predictions[i].PitchFactor = factor
		}
	}
	synthesisPlan, err := plan.Build(bank, reading, morae, selections, plan.Config{
		MoraDurationMS:   cfg.MoraDurationMS,
		PauseDurationMS:  cfg.PauseDurationMS,
		Predictions:      predictions,
		SelectionMode:    cfg.SelectionMode,
		JoinCostMode:     joinCostMode,
		JoinModelVersion: joinModelVersion,
		JoinScoreScale:   joinModelScoreScale(joinModel),
	})
	if err != nil {
		return nil, fmt.Errorf("build synthesis plan: %w", err)
	}
	synthesisPlan.Text = cfg.Text
	pitchCurve := cfg.PitchCurve
	applyPitch := applyPitchEnabled(cfg)
	if pitchCurve == nil && shouldPredictFrameContour(cfg, loadedProsody) {
		timings := moraTimings(morae, synthesisPlan)
		question := strings.ContainsAny(cfg.Text, "?？")
		if contour := loadedProsody.PredictFrameContour(morae, prosodyFeatures, timings, synthesisPlan.DurationMS+cfg.ReleaseMS, question); contour != nil {
			pitchCurve = &render.PitchCurve{FrameMS: contour.FrameMS, Cents: contour.Cents}
		}
	}
	manualPitch := cfg.ManualPitch
	if manualPitch == nil && cfg.ManualPitchPath != "" {
		manualPitch, err = prosody.LoadManualPitch(cfg.ManualPitchPath)
		if err != nil {
			return nil, fmt.Errorf("load manual pitch: %w", err)
		}
	}
	if manualPitch != nil {
		if manualPitch.Reading != "" && manualPitch.Reading != reading {
			return nil, fmt.Errorf("manual pitch reading does not match synthesis reading")
		}
		timings := moraTimings(morae, synthesisPlan)
		manualContour, curveErr := manualPitch.Curve(morae, timings, synthesisPlan.DurationMS+cfg.ReleaseMS)
		if curveErr != nil {
			return nil, fmt.Errorf("build manual pitch curve: %w", curveErr)
		}
		pitchCurve = mergeManualPitchCurve(pitchCurve, manualContour, manualPitch.Mode)
		pitchCurve = render.ConstrainPitchCurve(pitchCurve, 20, 8)
	}
	intonationStrength := effectiveIntonationStrength(cfg)
	pcm, err := render.Render(synthesisPlan, render.Config{
		ReleaseMS:               cfg.ReleaseMS,
		IntonationStrength:      intonationStrength,
		ApplyPitch:              applyPitch,
		Backend:                 cfg.Renderer,
		WorldlinePath:           cfg.WorldlinePath,
		WorldlineBridgePath:     cfg.WorldlineBridgePath,
		WorldlineR2MelPath:      cfg.WorldlineR2MelPath,
		WorldlineR2VocoderPath:  cfg.WorldlineR2VocoderPath,
		OnnxDeviceID:            cfg.OnnxDeviceID,
		UTAUResamplerPath:       cfg.UTAUResamplerPath,
		BoundaryBridgeMS:        cfg.BoundaryBridgeMS,
		BoundaryBridgeThreshold: cfg.BoundaryBridgeThreshold,
		PitchCurve:              pitchCurve,
	})
	if err != nil {
		return nil, fmt.Errorf("render: %w", err)
	}
	return &Result{Voicebank: bank, Plan: synthesisPlan, Audio: pcm}, nil
}

func validateConfig(cfg Config) error {
	finite := map[string]float64{
		"mora_duration_ms":          cfg.MoraDurationMS,
		"pause_duration_ms":         cfg.PauseDurationMS,
		"release_ms":                cfg.ReleaseMS,
		"intonation_strength":       cfg.IntonationStrength,
		"boundary_bridge_ms":        cfg.BoundaryBridgeMS,
		"boundary_bridge_threshold": cfg.BoundaryBridgeThreshold,
		"join_score_scale":          cfg.JoinScoreScale,
	}
	for name, value := range finite {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return fmt.Errorf("%s must be finite, got %v", name, value)
		}
	}
	for index, factor := range cfg.PitchFactors {
		if math.IsNaN(factor) || math.IsInf(factor, 0) {
			return fmt.Errorf("pitch factors: value %d must be finite, got %v", index, factor)
		}
	}
	if cfg.IntonationStrength < 0 || cfg.IntonationStrength > 1 {
		return fmt.Errorf("intonation_strength must be between 0 and 1, got %v", cfg.IntonationStrength)
	}
	if cfg.ReleaseMS < 0 {
		return fmt.Errorf("release_ms must be non-negative, got %v", cfg.ReleaseMS)
	}
	return nil
}

func mergeManualPitchCurve(base *render.PitchCurve, manual *prosody.PitchContour, mode string) *render.PitchCurve {
	if manual == nil || manual.FrameMS <= 0 || len(manual.Cents) == 0 {
		return base
	}
	result := &render.PitchCurve{FrameMS: manual.FrameMS, Cents: make([]float64, len(manual.Cents))}
	for index := range result.Cents {
		manualCents := manual.Cents[index]
		if mode == "replace" {
			result.Cents[index] = manualCents
			continue
		}
		baseCents := 0.0
		if base != nil && len(base.Cents) > 0 {
			baseCents = pitchCurveCentsAt(base, float64(index)*manual.FrameMS)
		}
		result.Cents[index] = baseCents + manualCents
	}
	return result
}

func pitchCurveCentsAt(curve *render.PitchCurve, timeMS float64) float64 {
	if curve == nil || curve.FrameMS <= 0 || len(curve.Cents) == 0 {
		return 0
	}
	position := math.Max(0, timeMS) / curve.FrameMS
	left := int(math.Floor(position))
	if left >= len(curve.Cents)-1 {
		return curve.Cents[len(curve.Cents)-1]
	}
	progress := position - float64(left)
	return curve.Cents[left]*(1-progress) + curve.Cents[left+1]*progress
}

func validateRuntimeMoraAlignment(morae []frontend.Mora, analyzed []string) error {
	if len(morae) != len(analyzed) {
		return fmt.Errorf("Open JTalk returned %d morae, Go frontend returned %d", len(analyzed), len(morae))
	}
	for index, mora := range morae {
		want := analyzed[index]
		if mora.Pause {
			want = ""
			if analyzed[index] != "" {
				return fmt.Errorf("frame %d: Open JTalk mora %q does not match pause", index, analyzed[index])
			}
			continue
		}
		if !runtimeMoraMatches(mora, want) {
			return fmt.Errorf("frame %d: Open JTalk mora %q does not match reading mora %q", index, want, mora.Text)
		}
	}
	return nil
}

func runtimeMoraMatches(mora frontend.Mora, analyzed string) bool {
	if mora.Text == analyzed {
		return true
	}
	if analyzed != "ー" || mora.Pause {
		return false
	}
	switch mora.Vowel {
	case "a", "i", "u", "e", "o":
		return true
	default:
		return false
	}
}

func alignRuntimeProsodyFeatures(morae []frontend.Mora, analysis *openjtalk.Analysis) ([]prosody.FeatureFrame, error) {
	if len(analysis.Morae) != len(analysis.Features) {
		return nil, fmt.Errorf("Open JTalk returned %d morae and %d feature frames", len(analysis.Morae), len(analysis.Features))
	}
	if len(analysis.Morae) < len(morae) {
		return nil, fmt.Errorf("Open JTalk returned %d morae, Go frontend returned %d", len(analysis.Morae), len(morae))
	}
	if len(analysis.Morae) == len(morae) {
		if err := validateRuntimeMoraAlignment(morae, analysis.Morae); err != nil {
			return nil, err
		}
		return append([]prosody.FeatureFrame(nil), analysis.Features...), nil
	}

	aligned := make([]prosody.FeatureFrame, len(morae))
	analyzedIndex := 0
	for index, mora := range morae {
		for analyzedIndex < len(analysis.Morae) && !runtimeMoraMatches(mora, analysis.Morae[analyzedIndex]) {
			analyzedIndex++
		}
		if analyzedIndex >= len(analysis.Morae) {
			return nil, fmt.Errorf("frame %d: Open JTalk mora sequence cannot be aligned with reading mora %q", index, mora.Text)
		}
		aligned[index] = analysis.Features[analyzedIndex]
		analyzedIndex++
	}
	return aligned, nil
}

func moraTimings(morae []frontend.Mora, synthesisPlan *plan.Plan) []prosody.MoraTiming {
	byPosition := make(map[int]plan.Unit, len(synthesisPlan.Units))
	for _, unit := range synthesisPlan.Units {
		byPosition[unit.Position] = unit
	}
	timings := make([]prosody.MoraTiming, len(morae))
	cursor := 0.0
	for position := 0; position < len(morae); {
		if unit, ok := byPosition[position]; ok {
			cursor = unit.NoteStartMS
			timings[position] = prosody.MoraTiming{StartMS: cursor, DurationMS: unit.DurationMS}
			cursor += unit.DurationMS
			position++
			continue
		}
		nextPosition := position + 1
		for nextPosition < len(morae) {
			if _, ok := byPosition[nextPosition]; ok {
				break
			}
			nextPosition++
		}
		nextStart := synthesisPlan.DurationMS
		if nextPosition < len(morae) {
			nextStart = byPosition[nextPosition].NoteStartMS
		}
		duration := math.Max(0, nextStart-cursor) / float64(nextPosition-position)
		for position < nextPosition {
			timings[position] = prosody.MoraTiming{StartMS: cursor, DurationMS: duration}
			cursor += duration
			position++
		}
	}
	return timings
}

func rendererSupportsFramePitch(renderer string, capabilities *plugin.Capabilities) bool {
	if capabilities != nil {
		return capabilities.FramePitch
	}
	directories, _ := plugin.DefaultDirectories()
	items, _ := plugin.DiscoverRenderers(directories, nil)
	for _, item := range items {
		if item.ID == renderer || item.Backend == renderer {
			return item.Capabilities.FramePitch
		}
	}
	return false
}

func applyPitchEnabled(cfg Config) bool {
	return cfg.ApplyPitch || cfg.ProsodyPitchOnly
}

func shouldPredictFrameContour(cfg Config, model *prosody.Model) bool {
	return applyPitchEnabled(cfg) && model != nil && model.HasFrameContour() &&
		rendererSupportsFramePitch(cfg.Renderer, cfg.RendererCapabilities)
}

func effectiveIntonationStrength(cfg Config) float64 {
	if !applyPitchEnabled(cfg) {
		return 0
	}
	return cfg.IntonationStrength
}

func joinModelScoreScale(model *connection.LearnedModel) float64 {
	if model == nil {
		return 0
	}
	return model.ScoreScale
}
