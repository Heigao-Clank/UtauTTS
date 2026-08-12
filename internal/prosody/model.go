package prosody

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"utautts/internal/frontend"
)

const ModelVersion = 3

const (
	SequenceModelVersion         = 4
	AccentSequenceModelVersion   = 5
	BoundedSequenceModelVersion  = 6
	LegacyFramePitchModelVersion = 7
	FramePitchModelVersion       = 8
	StandardAccentModelVersion   = 9
)

type Model struct {
	Version         int                  `json:"version"`
	FeatureVersion  int                  `json:"feature_version"`
	Mode            string               `json:"mode"`
	DurationWeights map[string]float64   `json:"duration_weights"`
	PitchWeights    map[string]float64   `json:"pitch_weights,omitempty"`
	EnergyWeights   map[string]float64   `json:"energy_weights,omitempty"`
	SequencePitch   *SequencePitchModel  `json:"sequence_pitch,omitempty"`
	FramePitch      *FramePitchModel     `json:"frame_pitch,omitempty"`
	StandardAccent  *StandardAccentModel `json:"standard_accent,omitempty"`
	Metrics         Metrics              `json:"metrics"`
	Training        TrainingInfo         `json:"training"`
}

type FeatureFrame map[string]float64

type SequencePitchModel struct {
	FeatureNames []string             `json:"feature_names"`
	InputWeights [][]float64          `json:"input_weights"`
	InputBias    []float64            `json:"input_bias"`
	Layers       []SequencePitchLayer `json:"layers"`
	OutputWeight []float64            `json:"output_weight"`
	OutputBias   float64              `json:"output_bias"`
	Low          float64              `json:"low"`
	High         float64              `json:"high"`
}

type SequencePitchLayer struct {
	Dilation int           `json:"dilation"`
	Weights  [][][]float64 `json:"weights"`
	Bias     []float64     `json:"bias"`
}

// FramePitchModel predicts a phrase-relative continuous cents contour. It
// shares the portable residual TCN representation with SequencePitchModel, but
// runs on fixed-duration frames rather than one position per mora.
type FramePitchModel struct {
	FeatureNames      []string             `json:"feature_names"`
	InputWeights      [][]float64          `json:"input_weights"`
	InputBias         []float64            `json:"input_bias"`
	Layers            []SequencePitchLayer `json:"layers"`
	OutputWeight      []float64            `json:"output_weight"`
	OutputBias        float64              `json:"output_bias"`
	FrameMS           float64              `json:"frame_ms"`
	LowCents          float64              `json:"low_cents"`
	HighCents         float64              `json:"high_cents"`
	RenderStrength    float64              `json:"render_strength,omitempty"`
	RenderSmoothingMS float64              `json:"render_smoothing_ms,omitempty"`
	RenderP99Cents    float64              `json:"render_p99_cents,omitempty"`
	RenderMaxCents    float64              `json:"render_max_cents,omitempty"`
}

// StandardAccentModel is a deterministic Tokyo-accent baseline. Learned
// models can later predict a bounded residual around this contour instead of
// relearning lexical accent direction from a small acoustic corpus.
type StandardAccentModel struct {
	FrameMS           float64 `json:"frame_ms"`
	AccentRangeCents  float64 `json:"accent_range_cents"`
	DeclinationCents  float64 `json:"declination_cents"`
	QuestionRiseCents float64 `json:"question_rise_cents"`
	SmoothingMS       float64 `json:"smoothing_ms"`
	P99Cents          float64 `json:"p99_cents"`
	MaxCents          float64 `json:"max_cents"`
}

type MoraTiming struct {
	StartMS    float64
	DurationMS float64
}

type PitchContour struct {
	FrameMS float64
	Cents   []float64
}

type TrainingInfo struct {
	Records int     `json:"records"`
	Tokens  int     `json:"tokens"`
	Epochs  int     `json:"epochs"`
	Rate    float64 `json:"learning_rate"`
	Seed    int64   `json:"seed"`
}

type Metrics struct {
	Records               int     `json:"records"`
	Tokens                int     `json:"tokens"`
	DurationMAEMS         float64 `json:"normalized_duration_mae_ms"`
	PitchMAECents         float64 `json:"pitch_mae_cents,omitempty"`
	EnergyMAEDB           float64 `json:"energy_mae_db,omitempty"`
	BaselineDurationMAEMS float64 `json:"baseline_duration_mae_ms,omitempty"`
	BaselinePitchMAECents float64 `json:"baseline_pitch_mae_cents,omitempty"`
	BaselineEnergyMAEDB   float64 `json:"baseline_energy_mae_db,omitempty"`
}

func LoadModel(path string) (*Model, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var model Model
	if err := json.Unmarshal(data, &model); err != nil {
		return nil, err
	}
	legacy := model.Version == 2 && model.FeatureVersion == 1 && model.Mode == "speech_duration_residual"
	current := model.Version == ModelVersion && model.FeatureVersion == 1 && model.Mode == "speech_prosody_residual"
	sequence := model.FeatureVersion == 1 && ((model.Version == SequenceModelVersion && model.Mode == "intonation_tcn") ||
		(model.Version == AccentSequenceModelVersion && model.Mode == "intonation_tcn_accent") ||
		(model.Version == BoundedSequenceModelVersion && model.Mode == "intonation_tcn_accent_bounded"))
	frame := model.FeatureVersion == 1 && (model.Version == LegacyFramePitchModelVersion || model.Version == FramePitchModelVersion) && model.Mode == "intonation_frame_tcn_accent_bounded"
	standardAccent := model.FeatureVersion == 1 && model.Version == StandardAccentModelVersion && model.Mode == "standard_japanese_accent"
	if !legacy && !current && !sequence && !frame && !standardAccent {
		return nil, fmt.Errorf("unsupported prosody model version %d/%d", model.Version, model.FeatureVersion)
	}
	if sequence {
		if err := validateSequencePitch(model.SequencePitch); err != nil {
			return nil, fmt.Errorf("invalid sequence pitch model: %w", err)
		}
	}
	if frame {
		if err := validateFramePitch(model.FramePitch); err != nil {
			return nil, fmt.Errorf("invalid frame pitch model: %w", err)
		}
	}
	if standardAccent {
		if err := validateStandardAccent(model.StandardAccent); err != nil {
			return nil, fmt.Errorf("invalid standard accent model: %w", err)
		}
	}
	return &model, nil
}

func (m *Model) Save(path string) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if directory := filepath.Dir(path); directory != "." {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			return err
		}
	}
	return os.WriteFile(path, data, 0o644)
}

func (m *Model) Predict(morae []frontend.Mora) []Prediction {
	return m.PredictWithFeatures(morae, nil)
}

func (m *Model) PredictWithFeatures(morae []frontend.Mora, frames []FeatureFrame) []Prediction {
	result := make([]Prediction, len(morae))
	durations := centeredFactors(m.DurationWeights, morae, 0.8, 1.25)
	pitches := centeredFactors(m.PitchWeights, morae, 0.97, 1.03)
	if m.SequencePitch != nil {
		pitches = m.SequencePitch.predict(morae, frames)
	}
	energies := centeredFactors(m.EnergyWeights, morae, 0.9, 1.1)
	for i := range morae {
		result[i] = Prediction{
			DurationFactor: durations[i],
			PitchFactor:    pitches[i],
			EnergyFactor:   energies[i],
		}
	}
	return result
}

// RequiresExternalFeatures reports whether the exported sequence model uses
// linguistic inputs that the mora-only Go frontend cannot derive by itself.
func (m *Model) RequiresExternalFeatures() bool {
	if m.StandardAccent != nil {
		return true
	}
	names := []string(nil)
	if m.SequencePitch != nil {
		names = m.SequencePitch.FeatureNames
	} else if m.FramePitch != nil {
		names = m.FramePitch.FeatureNames
	}
	for _, name := range names {
		if strings.HasPrefix(name, "accent_") || strings.HasPrefix(name, "word_") ||
			strings.HasPrefix(name, "pos=") || strings.HasPrefix(name, "pos_group1=") {
			return true
		}
	}
	return false
}

// HasFrameContour reports whether the model produces a renderer-facing frame
// pitch curve, either learned or from the standard-accent baseline.
func (m *Model) HasFrameContour() bool {
	return m != nil && (m.FramePitch != nil || m.StandardAccent != nil)
}

func validateStandardAccent(model *StandardAccentModel) error {
	if model == nil || model.FrameMS < 1 || model.AccentRangeCents <= 0 || model.MaxCents <= 0 {
		return fmt.Errorf("invalid standard accent metadata")
	}
	return nil
}

func validateFramePitch(model *FramePitchModel) error {
	if model == nil || model.FrameMS < 1 || model.LowCents >= model.HighCents || model.LowCents > 0 || model.HighCents < 0 {
		return fmt.Errorf("invalid frame pitch metadata")
	}
	portable := &SequencePitchModel{
		FeatureNames: model.FeatureNames, InputWeights: model.InputWeights, InputBias: model.InputBias,
		Layers: model.Layers, OutputWeight: model.OutputWeight, OutputBias: model.OutputBias,
		Low: 0.01, High: 100,
	}
	return validateSequencePitch(portable)
}

func (m *Model) PredictFrameContour(morae []frontend.Mora, frames []FeatureFrame, timings []MoraTiming, durationMS float64, question bool) *PitchContour {
	if m.StandardAccent != nil {
		return m.StandardAccent.predict(morae, frames, timings, durationMS, question)
	}
	model := m.FramePitch
	if model == nil || len(morae) == 0 || len(timings) != len(morae) || validateFramePitch(model) != nil {
		return nil
	}
	count := max(2, int(math.Ceil(durationMS/model.FrameMS))+1)
	featureIndex := make(map[string]int, len(model.FeatureNames))
	for index, name := range model.FeatureNames {
		featureIndex[name] = index
	}
	baseFeatures := indexedFeatureVectors(morae, frames, featureIndex)
	hidden := len(model.InputBias)
	state := make([][]float64, count)
	speech := make([]bool, count)
	moraIndex := 0
	for frameIndex := 0; frameIndex < count; frameIndex++ {
		timeMS := float64(frameIndex) * model.FrameMS
		for moraIndex+1 < len(timings) && timeMS >= timings[moraIndex].StartMS+timings[moraIndex].DurationMS {
			moraIndex++
		}
		duration := math.Max(1, timings[moraIndex].DurationMS)
		progress := clamp((timeMS-timings[moraIndex].StartMS)/duration, 0, 1)
		framePosition := float64(frameIndex) / float64(max(1, count-1))
		state[frameIndex] = append([]float64(nil), model.InputBias...)
		for _, feature := range baseFeatures[moraIndex] {
			for output := 0; output < hidden; output++ {
				state[frameIndex][output] += model.InputWeights[output][feature.column] * feature.value
			}
		}
		addIndexedFeature(state[frameIndex], model.InputWeights, featureIndex, "mora_progress", progress)
		addIndexedFeature(state[frameIndex], model.InputWeights, featureIndex, "mora_progress2", progress*progress)
		addIndexedFeature(state[frameIndex], model.InputWeights, featureIndex, "frame_position", framePosition)
		addIndexedFeature(state[frameIndex], model.InputWeights, featureIndex, "frame_from_end", 1-framePosition)
		addIndexedFeature(state[frameIndex], model.InputWeights, featureIndex, "final_distance", 1-framePosition)
		if question {
			addIndexedFeature(state[frameIndex], model.InputWeights, featureIndex, "question_distance", 1-framePosition)
		}
		for output := range state[frameIndex] {
			state[frameIndex][output] = math.Tanh(state[frameIndex][output])
		}
		speech[frameIndex] = !morae[moraIndex].Pause
	}
	for _, layer := range model.Layers {
		next := make([][]float64, len(state))
		for position := range state {
			next[position] = make([]float64, hidden)
			for output := 0; output < hidden; output++ {
				value := state[position][output] + layer.Bias[output]
				for input := 0; input < hidden; input++ {
					for kernel := 0; kernel < 3; kernel++ {
						source := position + (kernel-1)*layer.Dilation
						if source >= 0 && source < len(state) {
							value += layer.Weights[output][input][kernel] * state[source][input]
						}
					}
				}
				next[position][output] = math.Tanh(value)
			}
		}
		state = next
	}
	values := make([]float64, count)
	var voiced []float64
	for position := range state {
		value := model.OutputBias
		for index, weight := range model.OutputWeight {
			value += weight * state[position][index]
		}
		values[position] = value
		if speech[position] {
			voiced = append(voiced, value)
		}
	}
	center := median(voiced)
	strength := model.RenderStrength
	if strength <= 0 || strength > 1 {
		strength = 1
	}
	for position := range values {
		if !speech[position] {
			values[position] = 0
		} else {
			values[position] -= center
		}
	}
	if model.RenderStrength > 0 {
		smoothingMS := model.RenderSmoothingMS
		if smoothingMS <= 0 {
			smoothingMS = 20
		}
		values = smoothFramePitchPhrases(values, speech, model.FrameMS, smoothingMS)
	}
	for position := range values {
		if speech[position] {
			values[position] *= strength
		}
	}
	if model.RenderStrength > 0 {
		p99 := model.RenderP99Cents
		if p99 <= 0 {
			p99 = 75
		}
		if observed := absolutePercentile(values, speech, 0.99); observed > p99 {
			gain := p99 / observed
			for position := range values {
				if speech[position] {
					values[position] *= gain
				}
			}
		}
		maximum := model.RenderMaxCents
		if maximum <= 0 {
			maximum = 90
		}
		for position := range values {
			values[position] = clamp(values[position], -maximum, maximum)
		}
	}
	for position := range values {
		values[position] = clamp(values[position], model.LowCents, model.HighCents)
	}
	return &PitchContour{FrameMS: model.FrameMS, Cents: values}
}

func (model *StandardAccentModel) predict(morae []frontend.Mora, frames []FeatureFrame, timings []MoraTiming, durationMS float64, question bool) *PitchContour {
	if model == nil || len(morae) == 0 || len(frames) != len(morae) || len(timings) != len(morae) || validateStandardAccent(model) != nil {
		return nil
	}
	count := max(2, int(math.Ceil(durationMS/model.FrameMS))+1)
	values := make([]float64, count)
	speech := make([]bool, count)
	lastSpeechMora := -1
	for index := range morae {
		if !morae[index].Pause {
			lastSpeechMora = index
		}
	}
	moraIndex := 0
	for frameIndex := 0; frameIndex < count; frameIndex++ {
		timeMS := float64(frameIndex) * model.FrameMS
		for moraIndex+1 < len(timings) && timeMS >= timings[moraIndex].StartMS+timings[moraIndex].DurationMS {
			moraIndex++
		}
		if morae[moraIndex].Pause {
			continue
		}
		speech[frameIndex] = true
		feature := frames[moraIndex]
		value := -0.5 * model.AccentRangeCents
		if feature["accent_high"] >= 0.5 {
			value = 0.5 * model.AccentRangeCents
		}
		value -= model.DeclinationCents * clamp(feature["accent_position"], 0, 1)
		if question && moraIndex == lastSpeechMora && model.QuestionRiseCents > 0 {
			duration := math.Max(1, timings[moraIndex].DurationMS)
			progress := clamp((timeMS-timings[moraIndex].StartMS)/duration, 0, 1)
			if progress > 0.5 {
				rise := (progress - 0.5) / 0.5
				value += model.QuestionRiseCents * rise * rise
			}
		}
		values[frameIndex] = value
	}
	values = smoothFramePitchPhrases(values, speech, model.FrameMS, model.SmoothingMS)
	voiced := make([]float64, 0, len(values))
	for index, value := range values {
		if speech[index] {
			voiced = append(voiced, value)
		}
	}
	center := median(voiced)
	for index := range values {
		if speech[index] {
			values[index] -= center
		}
	}
	if model.P99Cents > 0 {
		if observed := absolutePercentile(values, speech, 0.99); observed > model.P99Cents {
			gain := model.P99Cents / observed
			for index := range values {
				if speech[index] {
					values[index] *= gain
				}
			}
		}
	}
	for index := range values {
		values[index] = clamp(values[index], -model.MaxCents, model.MaxCents)
	}
	return &PitchContour{FrameMS: model.FrameMS, Cents: values}
}

func smoothFramePitchPhrases(values []float64, speech []bool, frameMS, sigmaMS float64) []float64 {
	result := append([]float64(nil), values...)
	if frameMS <= 0 || sigmaMS <= 0 || len(values) != len(speech) {
		return result
	}
	sigma := sigmaMS / frameMS
	radius := max(1, int(math.Ceil(3*sigma)))
	weights := make([]float64, 2*radius+1)
	for offset := -radius; offset <= radius; offset++ {
		weights[offset+radius] = math.Exp(-0.5 * math.Pow(float64(offset)/sigma, 2))
	}
	for start := 0; start < len(values); {
		if !speech[start] {
			start++
			continue
		}
		end := start + 1
		for end < len(values) && speech[end] {
			end++
		}
		for position := start; position < end; position++ {
			sum, weightSum := 0.0, 0.0
			for offset := -radius; offset <= radius; offset++ {
				source := max(start, min(end-1, position+offset))
				weight := weights[offset+radius]
				sum += values[source] * weight
				weightSum += weight
			}
			result[position] = sum / weightSum
		}
		start = end
	}
	return result
}

func absolutePercentile(values []float64, mask []bool, quantile float64) float64 {
	selected := make([]float64, 0, len(values))
	for index, value := range values {
		if index < len(mask) && mask[index] {
			selected = append(selected, math.Abs(value))
		}
	}
	if len(selected) == 0 {
		return 0
	}
	sort.Float64s(selected)
	index := int(math.Ceil(clamp(quantile, 0, 1)*float64(len(selected)))) - 1
	return selected[max(0, min(len(selected)-1, index))]
}

func validateSequencePitch(model *SequencePitchModel) error {
	if model == nil {
		return fmt.Errorf("missing sequence_pitch")
	}
	hidden := len(model.InputBias)
	if hidden == 0 || len(model.InputWeights) != hidden || len(model.OutputWeight) != hidden {
		return fmt.Errorf("inconsistent hidden size")
	}
	for _, row := range model.InputWeights {
		if len(row) != len(model.FeatureNames) {
			return fmt.Errorf("input weight width is %d, want %d", len(row), len(model.FeatureNames))
		}
	}
	for _, layer := range model.Layers {
		if layer.Dilation <= 0 || len(layer.Weights) != hidden || len(layer.Bias) != hidden {
			return fmt.Errorf("invalid temporal layer dimensions")
		}
		for _, output := range layer.Weights {
			if len(output) != hidden {
				return fmt.Errorf("invalid temporal layer input size")
			}
			for _, kernel := range output {
				if len(kernel) != 3 {
					return fmt.Errorf("temporal kernel width is %d, want 3", len(kernel))
				}
			}
		}
	}
	if model.Low <= 0 || model.High < model.Low {
		return fmt.Errorf("invalid output bounds %.4f..%.4f", model.Low, model.High)
	}
	return nil
}

func (m *SequencePitchModel) predict(morae []frontend.Mora, frames []FeatureFrame) []float64 {
	result := make([]float64, len(morae))
	for i := range result {
		result[i] = 1
	}
	if len(morae) == 0 || validateSequencePitch(m) != nil {
		return result
	}
	featureIndex := make(map[string]int, len(m.FeatureNames))
	for i, name := range m.FeatureNames {
		featureIndex[name] = i
	}
	baseFeatures := indexedFeatureVectors(morae, frames, featureIndex)
	hidden := len(m.InputBias)
	state := make([][]float64, len(morae))
	for position := range morae {
		state[position] = append([]float64(nil), m.InputBias...)
		for _, feature := range baseFeatures[position] {
			for output := 0; output < hidden; output++ {
				state[position][output] += m.InputWeights[output][feature.column] * feature.value
			}
		}
		for output := range state[position] {
			state[position][output] = math.Tanh(state[position][output])
		}
	}
	for _, layer := range m.Layers {
		next := make([][]float64, len(state))
		for position := range state {
			next[position] = make([]float64, hidden)
			for output := 0; output < hidden; output++ {
				value := state[position][output] + layer.Bias[output]
				for input := 0; input < hidden; input++ {
					for kernel := 0; kernel < 3; kernel++ {
						source := position + (kernel-1)*layer.Dilation
						if source >= 0 && source < len(state) {
							value += layer.Weights[output][input][kernel] * state[source][input]
						}
					}
				}
				next[position][output] = math.Tanh(value)
			}
		}
		state = next
	}
	logs := make([]float64, len(morae))
	var speech []float64
	for position := range morae {
		if morae[position].Pause {
			continue
		}
		value := m.OutputBias
		for i, weight := range m.OutputWeight {
			value += weight * state[position][i]
		}
		logs[position] = value
		speech = append(speech, value)
	}
	if len(speech) == 0 {
		return result
	}
	center := median(speech)
	for position := range morae {
		if !morae[position].Pause {
			result[position] = clamp(math.Exp(logs[position]-center), m.Low, m.High)
		}
	}
	return result
}

func centeredFactors(weights map[string]float64, morae []frontend.Mora, low, high float64) []float64 {
	result := make([]float64, len(morae))
	logs := make([]float64, len(morae))
	var speech []float64
	for i := range morae {
		result[i] = 1
		if morae[i].Pause || len(weights) == 0 {
			continue
		}
		logs[i] = dot(weights, featuresFor(morae, i))
		speech = append(speech, logs[i])
	}
	if len(speech) == 0 {
		return result
	}
	center := median(speech)
	for i := range morae {
		if !morae[i].Pause {
			result[i] = clamp(math.Exp(logs[i]-center), low, high)
		}
	}
	return result
}

func featuresFor(morae []frontend.Mora, position int) map[string]float64 {
	current := morae[position]
	denominator := float64(max(1, len(morae)-1))
	pos := float64(position) / denominator
	result := map[string]float64{
		"bias": 1, "position": pos, "position2": pos * pos,
		"from_end": 1 - pos,
	}
	if position == 0 || morae[position-1].Pause {
		result["phrase_start"] = 1
	}
	if position == len(morae)-1 || morae[position+1].Pause {
		result["phrase_end"] = 1
	}
	addCategorical(result, "mora", current)
	if position > 0 {
		addCategorical(result, "prev", morae[position-1])
	} else {
		result["prev=<BOS>"] = 1
	}
	if position+1 < len(morae) {
		addCategorical(result, "next", morae[position+1])
	} else {
		result["next=<EOS>"] = 1
	}
	return result
}

// indexedFeatureVectors converts the static per-mora features once per
// synthesis instead of rebuilding and merging a map for every frame. Frame
// features such as mora_progress are added separately by the frame loop.
type indexedFeature struct {
	column int
	value  float64
}

func indexedFeatureVectors(morae []frontend.Mora, frames []FeatureFrame, index map[string]int) [][]indexedFeature {
	result := make([][]indexedFeature, len(morae))
	for position := range morae {
		features := featuresFor(morae, position)
		if position < len(frames) {
			for name, value := range frames[position] {
				features[name] = value
			}
		}
		row := make([]indexedFeature, 0, len(features))
		for name, value := range features {
			if column, ok := index[name]; ok {
				if value != 0 {
					row = append(row, indexedFeature{column: column, value: value})
				}
			}
		}
		result[position] = row
	}
	return result
}

func addIndexedFeature(state []float64, weights [][]float64, index map[string]int, name string, value float64) {
	column, ok := index[name]
	if !ok || value == 0 {
		return
	}
	for output := range state {
		state[output] += weights[output][column] * value
	}
}

func addCategorical(features map[string]float64, prefix string, mora frontend.Mora) {
	if mora.Pause {
		features[prefix+"=<PAUSE>"] = 1
		return
	}
	features[prefix+"="+mora.Text] = 1
	features[prefix+"_vowel="+mora.Vowel] = 1
}

func dot(weights, features map[string]float64) float64 {
	result := 0.0
	for _, name := range sortedFeatureNames(features) {
		result += weights[name] * features[name]
	}
	return result
}

func sortedFeatureNames(features map[string]float64) []string {
	names := make([]string, 0, len(features))
	for name := range features {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func clamp(value, low, high float64) float64 {
	return math.Max(low, math.Min(high, value))
}
