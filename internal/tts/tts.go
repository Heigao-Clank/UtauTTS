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
		if loadedProsody.RequiresExternalFeatures() && len(prosodyFeatures) == 0 {
			runtimeFeatures, err = analyzeOpenJTalkCached(cfg.Text, openjtalk.Config{
				HelperPath: cfg.OpenJTalkPath, DictionaryPath: cfg.OpenJTalkDictionaryPath,
			})
			if err != nil {
				return nil, fmt.Errorf("analyze runtime prosody features: %w", err)
			}
			prosodyFeatures = runtimeFeatures.Features
		}
	}
	reading := cfg.Reading
	if reading == "" {
		if runtimeFeatures != nil {
			reading = runtimeFeatures.Reading
		} else {
			reading, err = frontend.ToKana(cfg.Text)
			if err != nil {
				return nil, fmt.Errorf("convert text to reading: %w", err)
			}
		}
	}
	morae, err := frontend.ParseKana(reading)
	if err != nil {
		return nil, fmt.Errorf("parse reading: %w", err)
	}
	if runtimeFeatures != nil {
		if err := validateRuntimeMoraAlignment(morae, runtimeFeatures.Morae); err != nil {
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
	if pitchCurve == nil && loadedProsody != nil && loadedProsody.HasFrameContour() && rendererSupportsFramePitch(cfg.Renderer, cfg.RendererCapabilities) {
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
	applyPitch := applyPitchEnabled(cfg)
	pcm, err := render.Render(synthesisPlan, render.Config{
		ReleaseMS:               cfg.ReleaseMS,
		IntonationStrength:      cfg.IntonationStrength,
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
		if mora.Text != want {
			return fmt.Errorf("frame %d: Open JTalk mora %q does not match reading mora %q", index, want, mora.Text)
		}
	}
	return nil
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

func joinModelScoreScale(model *connection.LearnedModel) float64 {
	if model == nil {
		return 0
	}
	return model.ScoreScale
}
