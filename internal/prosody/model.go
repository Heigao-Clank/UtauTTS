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

const ModelVersion = 2

type Model struct {
	Version         int                `json:"version"`
	FeatureVersion  int                `json:"feature_version"`
	Mode            string             `json:"mode"`
	DurationWeights map[string]float64 `json:"duration_weights"`
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
	Records       int     `json:"records"`
	Tokens        int     `json:"tokens"`
	DurationMAEMS float64 `json:"normalized_duration_mae_ms"`
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
	if model.Version != ModelVersion || model.FeatureVersion != 1 || model.Mode != "speech_duration_residual" {
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
	logFactors := make([]float64, len(morae))
	var speechFactors []float64
	for i := range morae {
		if morae[i].Pause {
			continue
		}
		logFactors[i] = dot(m.DurationWeights, featuresFor(morae, i))
		speechFactors = append(speechFactors, logFactors[i])
	}
	center := median(speechFactors)
	for i := range morae {
		factor := 1.0
		if !morae[i].Pause {
			factor = clamp(math.Exp(logFactors[i]-center), 0.8, 1.25)
		}
		result[i] = Prediction{
			DurationFactor: factor,
			PitchFactor:    1,
			EnergyFactor:   1,
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
