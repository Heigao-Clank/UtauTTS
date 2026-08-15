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
	MoraDurationsMS         []float64
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
		if strings.TrimSpace(runtimeText) == "" {
			// Kana-only requests do not have a surface text. Open JTalk can
			// still analyze the supplied reading, and passing an empty string
			// here would otherwise fail before synthesis starts.
			runtimeText = reading
		}
		runtimeConfig := openjtalk.Config{
			HelperPath: cfg.OpenJTalkPath, DictionaryPath: cfg.OpenJTalkDictionaryPath,
		}
		runtimeFeatures, err = analyzeOpenJTalkCached(runtimeText, runtimeConfig)
		if err != nil {
			return nil, fmt.Errorf("analyze runtime prosody features: %w", err)
		}
		alignedFeatures, alignmentErr := alignRuntimeProsodyFeatures(morae, runtimeFeatures)
		if alignmentErr != nil {
			fallbackFeatures, fallbackErr := analyzeOpenJTalkCached(reading, runtimeConfig)
			if fallbackErr == nil {
				alignedFeatures, fallbackErr = alignRuntimeProsodyFeatures(morae, fallbackFeatures)
			}
			if fallbackErr != nil {
				return nil, fmt.Errorf("align runtime prosody features: %w", alignmentErr)
			}
		}
		prosodyFeatures = alignedFeatures
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
		MoraDurationsMS:  cfg.MoraDurationsMS,
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
	for index, duration := range cfg.MoraDurationsMS {
		if math.IsNaN(duration) || math.IsInf(duration, 0) {
			return fmt.Errorf("mora durations: value %d must be finite, got %v", index, duration)
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

func alignRuntimeProsodyFeatures(morae []frontend.Mora, analysis *openjtalk.Analysis) ([]prosody.FeatureFrame, error) {
	if analysis == nil {
		return nil, fmt.Errorf("Open JTalk analysis is nil")
	}
	if len(analysis.Morae) != len(analysis.Features) {
		return nil, fmt.Errorf("Open JTalk returned %d morae and %d feature frames", len(analysis.Morae), len(analysis.Features))
	}
	if len(morae) == 0 {
		return nil, nil
	}
	if len(analysis.Morae) == 0 {
		return make([]prosody.FeatureFrame, len(morae)), nil
	}

	indices := alignRuntimeMoraIndices(morae, analysis.Morae)
	aligned := make([]prosody.FeatureFrame, len(morae))
	for index, analyzedIndex := range indices {
		if analyzedIndex >= 0 {
			aligned[index] = cloneFeatureFrame(analysis.Features[analyzedIndex])
			continue
		}
		if morae[index].Pause {
			aligned[index] = prosody.FeatureFrame{}
			continue
		}
		aligned[index] = cloneFeatureFrame(nearestRuntimeFeature(indices, analysis.Features, index))
	}
	return aligned, nil
}

type runtimeMoraAlignmentCell struct {
	cost float64
	op   byte
}

const (
	runtimeAlignmentSkipCost   = 1.1
	runtimeAlignmentChangeCost = 1.8
)

func alignRuntimeMoraIndices(morae []frontend.Mora, analyzed []string) []int {
	rows := len(morae) + 1
	columns := len(analyzed) + 1
	cells := make([][]runtimeMoraAlignmentCell, rows)
	for row := range cells {
		cells[row] = make([]runtimeMoraAlignmentCell, columns)
		for column := range cells[row] {
			cells[row][column].cost = math.Inf(1)
		}
	}
	cells[0][0] = runtimeMoraAlignmentCell{}
	for row := 1; row < rows; row++ {
		cells[row][0] = runtimeMoraAlignmentCell{
			cost: cells[row-1][0].cost + runtimeAlignmentSkipCost,
			op:   'g',
		}
	}
	for column := 1; column < columns; column++ {
		cells[0][column] = runtimeMoraAlignmentCell{
			cost: cells[0][column-1].cost + runtimeAlignmentSkipCost,
			op:   'o',
		}
	}
	for row := 1; row < rows; row++ {
		for column := 1; column < columns; column++ {
			best := runtimeMoraAlignmentCell{
				cost: cells[row-1][column-1].cost + runtimeMoraCost(morae[row-1], analyzed[column-1]),
				op:   'm',
			}
			best = chooseRuntimeAlignment(best, runtimeMoraAlignmentCell{
				cost: cells[row-1][column].cost + runtimeAlignmentSkipCost,
				op:   'g',
			})
			best = chooseRuntimeAlignment(best, runtimeMoraAlignmentCell{
				cost: cells[row][column-1].cost + runtimeAlignmentSkipCost,
				op:   'o',
			})
			cells[row][column] = best
		}
	}

	indices := make([]int, len(morae))
	for index := range indices {
		indices[index] = -1
	}
	row, column := len(morae), len(analyzed)
	for row > 0 || column > 0 {
		if row == 0 {
			column--
			continue
		}
		if column == 0 {
			row--
			continue
		}
		switch cells[row][column].op {
		case 'm':
			indices[row-1] = column - 1
			row--
			column--
		case 'g':
			row--
		case 'o':
			column--
		default:
			row--
			column--
		}
	}
	return indices
}

func chooseRuntimeAlignment(current, candidate runtimeMoraAlignmentCell) runtimeMoraAlignmentCell {
	if candidate.cost < current.cost-1e-9 {
		return candidate
	}
	if math.Abs(candidate.cost-current.cost) <= 1e-9 && runtimeAlignmentPriority(candidate.op) > runtimeAlignmentPriority(current.op) {
		return candidate
	}
	return current
}

func runtimeAlignmentPriority(operation byte) int {
	switch operation {
	case 'm':
		return 3
	case 'o':
		return 2
	case 'g':
		return 1
	default:
		return 0
	}
}

func runtimeMoraCost(mora frontend.Mora, analyzed string) float64 {
	if mora.Pause || analyzed == "" {
		if mora.Pause && analyzed == "" {
			return 0
		}
		return runtimeAlignmentChangeCost + runtimeAlignmentSkipCost
	}
	if mora.Text == analyzed {
		return 0
	}
	if analyzed == "ー" && isRuntimeVowel(mora.Vowel) {
		return 0.25
	}
	analyzedVowel := runtimeAnalyzedMoraVowel(analyzed)
	if analyzedVowel != "" && analyzedVowel == mora.Vowel {
		return 0.6
	}
	return runtimeAlignmentChangeCost
}

func isRuntimeVowel(vowel string) bool {
	switch vowel {
	case "a", "i", "u", "e", "o":
		return true
	default:
		return false
	}
}

func runtimeAnalyzedMoraVowel(analyzed string) string {
	parsed, err := frontend.ParseKana(analyzed)
	if err != nil || len(parsed) != 1 || parsed[0].Pause {
		return ""
	}
	return parsed[0].Vowel
}

func nearestRuntimeFeature(indices []int, features []prosody.FeatureFrame, target int) prosody.FeatureFrame {
	for distance := 1; distance < len(indices)+1; distance++ {
		left := target - distance
		if left >= 0 && indices[left] >= 0 {
			return features[indices[left]]
		}
		right := target + distance
		if right < len(indices) && indices[right] >= 0 {
			return features[indices[right]]
		}
	}
	return prosody.FeatureFrame{}
}

func cloneFeatureFrame(frame prosody.FeatureFrame) prosody.FeatureFrame {
	if len(frame) == 0 {
		return prosody.FeatureFrame{}
	}
	result := make(prosody.FeatureFrame, len(frame))
	for name, value := range frame {
		result[name] = value
	}
	return result
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
