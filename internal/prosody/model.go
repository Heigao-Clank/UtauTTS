package prosody

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"

	"utautts/internal/frontend"
)

const ModelVersion = 3

const SequenceModelVersion = 4

type Model struct {
	Version         int                 `json:"version"`
	FeatureVersion  int                 `json:"feature_version"`
	Mode            string              `json:"mode"`
	DurationWeights map[string]float64  `json:"duration_weights"`
	PitchWeights    map[string]float64  `json:"pitch_weights,omitempty"`
	EnergyWeights   map[string]float64  `json:"energy_weights,omitempty"`
	SequencePitch   *SequencePitchModel `json:"sequence_pitch,omitempty"`
	Metrics         Metrics             `json:"metrics"`
	Training        TrainingInfo        `json:"training"`
}

// SequencePitchModel is a compact temporal convolution network. Its JSON form
// is deliberately runtime-independent so models trained with PyTorch can be
// evaluated by the Go synthesizer without an ONNX runtime.
type SequencePitchModel struct {
	FeatureNames []string             `json:"feature_names"`
	InputWeights [][]float64          `json:"input_weights"` // hidden x features
	InputBias    []float64            `json:"input_bias"`
	Layers       []SequencePitchLayer `json:"layers"`
	OutputWeight []float64            `json:"output_weight"`
	OutputBias   float64              `json:"output_bias"`
	Low          float64              `json:"low"`
	High         float64              `json:"high"`
}

type SequencePitchLayer struct {
	Dilation int           `json:"dilation"`
	Weights  [][][]float64 `json:"weights"` // output x input x kernel(3)
	Bias     []float64     `json:"bias"`
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
	sequence := model.Version == SequenceModelVersion && model.FeatureVersion == 1 && model.Mode == "intonation_tcn"
	if !legacy && !current && !sequence {
		return nil, fmt.Errorf("unsupported prosody model version %d/%d", model.Version, model.FeatureVersion)
	}
	if sequence {
		if err := validateSequencePitch(model.SequencePitch); err != nil {
			return nil, fmt.Errorf("invalid sequence pitch model: %w", err)
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
	result := make([]Prediction, len(morae))
	durations := centeredFactors(m.DurationWeights, morae, 0.8, 1.25)
	pitches := centeredFactors(m.PitchWeights, morae, 0.97, 1.03)
	if m.SequencePitch != nil {
		pitches = m.SequencePitch.predict(morae)
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

func (m *SequencePitchModel) predict(morae []frontend.Mora) []float64 {
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
	hidden := len(m.InputBias)
	state := make([][]float64, len(morae))
	for position := range morae {
		state[position] = append([]float64(nil), m.InputBias...)
		for name, value := range featuresFor(morae, position) {
			column, ok := featureIndex[name]
			if !ok {
				continue
			}
			for output := 0; output < hidden; output++ {
				state[position][output] += m.InputWeights[output][column] * value
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
