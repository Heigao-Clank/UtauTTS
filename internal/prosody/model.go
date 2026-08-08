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

const ModelVersion = 1

// Model predicts speaker-independent, relative prosody. Pitch and energy are
// ratios around an utterance median, so it does not copy the corpus speaker's
// absolute voice into a UTAU voicebank.
type Model struct {
	Version         int                `json:"version"`
	FeatureVersion  int                `json:"feature_version"`
	DurationWeights map[string]float64 `json:"duration_weights"`
	PitchWeights    map[string]float64 `json:"pitch_weights"`
	EnergyWeights   map[string]float64 `json:"energy_weights"`
	Metrics         Metrics            `json:"metrics"`
	Training        TrainingInfo       `json:"training"`
}

type TrainingInfo struct {
	Records int     `json:"records"`
	Tokens  int     `json:"tokens"`
	Epochs  int     `json:"epochs"`
	Rate    float64 `json:"learning_rate"`
	Seed    int64   `json:"seed"`
}

type Metrics struct {
	Records        int     `json:"records"`
	Tokens         int     `json:"tokens"`
	DurationMAEMS  float64 `json:"duration_mae_ms"`
	PitchMAECents  float64 `json:"pitch_mae_cents"`
	EnergyRatioMAE float64 `json:"energy_ratio_mae"`
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
	if model.Version != ModelVersion || model.FeatureVersion != 1 {
		return nil, fmt.Errorf("unsupported prosody model version %d/%d", model.Version, model.FeatureVersion)
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
	for i := range morae {
		features := featuresFor(morae, i)
		duration := math.Exp(dot(m.DurationWeights, features))
		pitch, energy := 1.0, 1.0
		if !morae[i].Pause {
			pitch = math.Exp(dot(m.PitchWeights, features))
			energy = math.Exp(dot(m.EnergyWeights, features))
		}
		result[i] = Prediction{
			DurationMS:   clamp(duration, 25, 800),
			PitchFactor:  clamp(pitch, 0.75, 1.35),
			EnergyFactor: clamp(energy, 0.35, 2.0),
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
