package prosody

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"

	"utautts/internal/audio"
	"utautts/internal/frontend"
)

func TestEstimateF0(t *testing.T) {
	const sampleRate = 16000
	wave := make([]float64, sampleRate/2)
	for i := range wave {
		wave[i] = 0.2 * math.Sin(2*math.Pi*200*float64(i)/sampleRate)
	}
	got := estimateMedianF0(wave, sampleRate)
	if math.Abs(got-200) > 3 {
		t.Fatalf("estimateMedianF0() = %.2f, want about 200", got)
	}
}

func TestTrainPredictSaveLoad(t *testing.T) {
	morae, err := frontend.ParseKana("あいう、えお")
	if err != nil {
		t.Fatal(err)
	}
	records := make([]Record, 20)
	for r := range records {
		targets := make([]Target, len(morae))
		for i, mora := range morae {
			duration := 90.0 + float64(i)*8
			if mora.Pause {
				duration = 180
			}
			targets[i] = Target{Position: i, Mora: mora.Text, Vowel: mora.Vowel, Pause: mora.Pause, DurationMS: duration, PitchRatio: 0.9 + float64(i)*0.03, EnergyRatio: 0.8 + float64(i)*0.05}
		}
		records[r] = Record{Version: DatasetVersion, ID: string(rune('a' + r)), Tokens: targets}
	}
	model, err := Train(records, TrainConfig{Epochs: 4, LearningRate: 0.005, Seed: 9})
	if err != nil {
		t.Fatal(err)
	}
	if len(model.PitchWeights) == 0 || len(model.EnergyWeights) == 0 {
		t.Fatal("pitch and energy models were not trained")
	}
	predictions := model.Predict(morae)
	if len(predictions) != len(morae) || predictions[0].DurationFactor < 0.8 || predictions[0].DurationFactor > 1.25 {
		t.Fatalf("invalid predictions: %#v", predictions)
	}
	for _, prediction := range predictions {
		if prediction.PitchFactor < 0.97 || prediction.PitchFactor > 1.03 || prediction.EnergyFactor < 0.9 || prediction.EnergyFactor > 1.1 {
			t.Fatalf("prosody factors out of bounds: %#v", prediction)
		}
	}
	for i, mora := range morae {
		if mora.Pause && predictions[i].DurationFactor != 1 {
			t.Fatalf("duration-only model changed pause at %d: %#v", i, predictions[i])
		}
	}
	path := filepath.Join(t.TempDir(), "nested", "model.json")
	if err := model.Save(path); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadModel(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := loaded.Predict(morae)[0].DurationFactor; math.Abs(got-predictions[0].DurationFactor) > 1e-9 {
		t.Fatalf("round trip changed prediction: %v != %v", got, predictions[0].DurationFactor)
	}
	again, err := Train(records, TrainConfig{Epochs: 4, LearningRate: 0.005, Seed: 9})
	if err != nil {
		t.Fatal(err)
	}
	firstJSON, _ := json.Marshal(model)
	secondJSON, _ := json.Marshal(again)
	if string(firstJSON) != string(secondJSON) {
		t.Fatal("training is not deterministic")
	}
}

func TestLoadLegacyDurationOnlyModel(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.json")
	data := []byte(`{"version":2,"feature_version":1,"mode":"speech_duration_residual","duration_weights":{"bias":0.1}}`)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	model, err := LoadModel(path)
	if err != nil {
		t.Fatal(err)
	}
	prediction := model.Predict([]frontend.Mora{{Text: "あ", Vowel: "a"}})[0]
	if prediction.PitchFactor != 1 || prediction.EnergyFactor != 1 {
		t.Fatalf("legacy model changed pitch or energy: %+v", prediction)
	}
}

func TestExtractRecord(t *testing.T) {
	const sampleRate = 16000
	data := make([]int16, sampleRate)
	for i := sampleRate / 10; i < 9*sampleRate/10; i++ {
		data[i] = int16(6000 * math.Sin(2*math.Pi*180*float64(i)/sampleRate))
	}
	path := filepath.Join(t.TempDir(), "speech.wav")
	if err := audio.WriteWav(path, &audio.PCM{SampleRate: sampleRate, Channels: 1, Data: data}); err != nil {
		t.Fatal(err)
	}
	record, err := ExtractRecord("test", "あいう", path, ExtractConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if len(record.Tokens) != 3 || record.MedianF0Hz < 170 || record.MedianF0Hz > 190 {
		t.Fatalf("unexpected extraction: %#v", record)
	}
}
