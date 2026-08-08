package prosody

import (
	"fmt"
	"hash/fnv"
	"math"
	"math/rand"

	"utautts/internal/frontend"
)

type TrainConfig struct {
	Epochs       int
	LearningRate float64
	L2           float64
	Seed         int64
}

type example struct {
	features map[string]float64
	target   float64
}

func Train(records []Record, cfg TrainConfig) (*Model, error) {
	if len(records) < 2 {
		return nil, fmt.Errorf("at least two records are required")
	}
	if cfg.Epochs <= 0 {
		cfg.Epochs = 30
	}
	if cfg.LearningRate <= 0 {
		cfg.LearningRate = 0.01
	}
	if cfg.L2 < 0 {
		cfg.L2 = 0
	}
	if cfg.Seed == 0 {
		cfg.Seed = 1
	}

	var duration []example
	trainRecords, validation := splitRecords(records)
	for _, record := range trainRecords {
		morae := targetsToMorae(record.Tokens)
		center := speechRateCenter(record.Tokens)
		for i, target := range record.Tokens {
			if !target.Pause && target.DurationMS > 0 {
				ratio := target.DurationMS / baselineDuration(target)
				ratio /= center
				duration = append(duration, example{featuresFor(morae, i), math.Log(clamp(ratio, 0.5, 2.0))})
			}
		}
	}
	if len(duration) == 0 {
		return nil, fmt.Errorf("dataset has insufficient duration targets")
	}
	model := &Model{
		Version: ModelVersion, FeatureVersion: 1, Mode: "speech_duration_residual",
		DurationWeights: fit(duration, cfg, cfg.Seed),
		Training:        TrainingInfo{Records: len(trainRecords), Tokens: len(duration), Epochs: cfg.Epochs, Rate: cfg.LearningRate, Seed: cfg.Seed},
	}
	model.Metrics = Evaluate(model, validation)
	return model, nil
}

func splitRecords(records []Record) (train, validation []Record) {
	for _, record := range records {
		h := fnv.New32a()
		_, _ = h.Write([]byte(record.ID))
		if h.Sum32()%10 == 0 {
			validation = append(validation, record)
		} else {
			train = append(train, record)
		}
	}
	if len(validation) == 0 {
		validation = append(validation, train[len(train)-1])
		train = train[:len(train)-1]
	}
	if len(train) == 0 {
		train = append(train, validation[len(validation)-1])
		validation = validation[:len(validation)-1]
	}
	return train, validation
}

// fit uses deterministic Adam on log targets. A Huber derivative limits the
// influence of occasional weak-alignment mistakes.
func fit(examples []example, cfg TrainConfig, seed int64) map[string]float64 {
	weights, first, second := map[string]float64{}, map[string]float64{}, map[string]float64{}
	mean := 0.0
	for _, item := range examples {
		mean += item.target
	}
	weights["bias"] = mean / float64(len(examples))
	rng := rand.New(rand.NewSource(seed))
	order := make([]int, len(examples))
	step := 0
	for epoch := 0; epoch < cfg.Epochs; epoch++ {
		for i := range order {
			order[i] = i
		}
		rng.Shuffle(len(order), func(i, j int) { order[i], order[j] = order[j], order[i] })
		for _, index := range order {
			item := examples[index]
			residual := dot(weights, item.features) - item.target
			gradient := math.Max(-0.25, math.Min(0.25, residual))
			step++
			for _, name := range sortedFeatureNames(item.features) {
				value := item.features[name]
				g := gradient*value + cfg.L2*weights[name]
				first[name] = 0.9*first[name] + 0.1*g
				second[name] = 0.999*second[name] + 0.001*g*g
				m := first[name] / (1 - math.Pow(0.9, float64(step)))
				v := second[name] / (1 - math.Pow(0.999, float64(step)))
				weights[name] -= cfg.LearningRate * m / (math.Sqrt(v) + 1e-8)
			}
		}
	}
	return weights
}

func Evaluate(model *Model, records []Record) Metrics {
	result := Metrics{Records: len(records)}
	var durationN int
	for _, record := range records {
		morae := targetsToMorae(record.Tokens)
		predicted := model.Predict(morae)
		center := speechRateCenter(record.Tokens)
		for i, target := range record.Tokens {
			if !target.Pause && target.DurationMS > 0 {
				expected := baselineDuration(target) * predicted[i].DurationFactor
				actual := target.DurationMS / center
				result.DurationMAEMS += math.Abs(expected - actual)
				durationN++
			}
		}
	}
	result.Tokens = durationN
	if durationN > 0 {
		result.DurationMAEMS /= float64(durationN)
	}
	return result
}

func speechRateCenter(targets []Target) float64 {
	var ratios []float64
	for _, target := range targets {
		if !target.Pause && target.DurationMS > 0 {
			ratios = append(ratios, target.DurationMS/baselineDuration(target))
		}
	}
	value := median(ratios)
	if value <= 0 {
		return 1
	}
	return value
}

func baselineDuration(target Target) float64 {
	if target.Pause {
		return 180
	}
	switch target.Vowel {
	case "cl":
		return 140 * 0.65
	case "n":
		return 140 * 0.9
	}
	if target.Mora == "ー" {
		return 140 * 1.2
	}
	return 140
}

func targetsToMorae(targets []Target) []frontend.Mora {
	result := make([]frontend.Mora, len(targets))
	for i, target := range targets {
		result[i] = moraFromTarget(target)
	}
	return result
}
