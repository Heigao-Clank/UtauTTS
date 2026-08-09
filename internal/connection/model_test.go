package connection

import (
	"math"
	"testing"

	"utautts/internal/acoustic"
)

func TestSplitExamplesKeepsGroupsTogether(t *testing.T) {
	examples := []Example{
		{Voicebank: "a", GroupID: "one", Label: 1},
		{Voicebank: "a", GroupID: "one", Label: 0},
		{Voicebank: "b", GroupID: "two", Label: 1},
		{Voicebank: "b", GroupID: "two", Label: 0},
	}
	training, validation := SplitExamples(examples, SplitConfig{ValidationVoicebank: "b"})
	if len(training) != 2 || len(validation) != 2 {
		t.Fatalf("training=%d validation=%d", len(training), len(validation))
	}
	for _, example := range training {
		if example.Voicebank != "a" {
			t.Fatalf("training leaked %q", example.Voicebank)
		}
	}
	for _, example := range validation {
		if example.Voicebank != "b" {
			t.Fatalf("validation leaked %q", example.Voicebank)
		}
	}
}

func TestTrainModelLearnsAcousticDifference(t *testing.T) {
	var training, validation []Example
	for index := 0; index < 40; index++ {
		training = append(training,
			Example{Label: 1, Features: testLearningFeatures(1 + float64(index%3)*0.1)},
			Example{Label: 0, Features: testLearningFeatures(12 + float64(index%3))},
		)
	}
	for index := 0; index < 10; index++ {
		validation = append(validation,
			Example{Label: 1, Features: testLearningFeatures(1.2)},
			Example{Label: 0, Features: testLearningFeatures(13)},
		)
	}
	model, err := TrainModel(training, validation, TrainConfig{Epochs: 300, LearningRate: 0.1, L2: 0.001})
	if err != nil {
		t.Fatal(err)
	}
	if model.Metrics.AUC < 0.99 || model.Metrics.BalancedAccuracy < 0.95 {
		t.Fatalf("metrics=%+v", model.Metrics)
	}
	if model.Predict(testLearningFeatures(1)) <= model.Predict(testLearningFeatures(14)) {
		t.Fatal("smooth boundary did not receive a higher probability")
	}
}

func TestLearnedScoreIsBoundedAndKeepsContinuityBonus(t *testing.T) {
	model := &LearnedModel{
		Means: make([]float64, len(featureNames())), Scales: make([]float64, len(featureNames())),
		Weights: make([]float64, len(featureNames())), Bias: 100,
	}
	for index := range model.Scales {
		model.Scales[index] = 1
	}
	features := PairFeatures{
		PreviousOutgoing: testLearningFeatures(1).PreviousOutgoing,
		CurrentIncoming:  testLearningFeatures(1).CurrentIncoming,
		ForwardInSource:  true,
	}
	score, probability := LearnedScore(features, model)
	if score != 16 || probability < 0.99 {
		t.Fatalf("score=%f probability=%f", score, probability)
	}
}

func testLearningFeatures(delta float64) LearningFeatures {
	left, right := make([]float64, 10), make([]float64, 10)
	for index := range right {
		right[index] = delta * (1 + float64(index)/20)
	}
	return LearningFeatures{
		PreviousOutgoing: acoustic.Frame{Valid: true, RMSDB: -20, F0Hz: 200, SpectrumDB: left},
		CurrentIncoming:  acoustic.Frame{Valid: true, RMSDB: -20 + delta, F0Hz: 200 * math.Pow(2, delta/1200), SpectrumDB: right},
		SpectrumDelta:    delta, RMSDelta: delta, F0DeltaCents: delta,
	}
}
