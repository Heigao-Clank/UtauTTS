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

func TestSequencePitchUsesTemporalContext(t *testing.T) {
	model := &Model{
		Version: SequenceModelVersion, FeatureVersion: 1, Mode: "intonation_tcn",
		SequencePitch: &SequencePitchModel{
			FeatureNames: []string{"phrase_start"},
			InputWeights: [][]float64{{1}}, InputBias: []float64{0},
			Layers: []SequencePitchLayer{{
				Dilation: 1, Weights: [][][]float64{{{0.5, 0, 0}}}, Bias: []float64{0},
			}},
			OutputWeight: []float64{1}, Low: 0.9, High: 1.1,
		},
	}
	morae := []frontend.Mora{{Text: "a", Vowel: "a"}, {Text: "i", Vowel: "i"}, {Text: "u", Vowel: "u"}}
	predicted := model.Predict(morae)
	if predicted[0].PitchFactor <= predicted[1].PitchFactor {
		t.Fatalf("sequence pitch did not use the start feature: %#v", predicted)
	}
	if predicted[1].PitchFactor <= predicted[2].PitchFactor {
		t.Fatalf("temporal convolution did not carry context forward: %#v", predicted)
	}
	path := filepath.Join(t.TempDir(), "sequence.json")
	if err := model.Save(path); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadModel(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := loaded.Predict(morae); math.Abs(got[1].PitchFactor-predicted[1].PitchFactor) > 1e-9 {
		t.Fatalf("sequence model round trip changed prediction: %#v != %#v", got, predicted)
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

func TestExtractRecordUsesHTSLabels(t *testing.T) {
	const sampleRate = 16000
	data := make([]int16, sampleRate)
	for i := sampleRate / 10; i < 7*sampleRate/10; i++ {
		data[i] = int16(6000 * math.Sin(2*math.Pi*180*float64(i)/sampleRate))
	}
	directory := t.TempDir()
	wavPath := filepath.Join(directory, "speech.wav")
	if err := audio.WriteWav(wavPath, &audio.PCM{SampleRate: sampleRate, Channels: 1, Data: data}); err != nil {
		t.Fatal(err)
	}
	labelPath := filepath.Join(directory, "speech.lab")
	labels := "0 1000000 xx^xx-sil+a=x\n" +
		"1000000 3000000 sil^x-a+x=i\n" +
		"3000000 5000000 a^x-i+x=u\n" +
		"5000000 7000000 i^x-u+x=sil\n" +
		"7000000 10000000 u^x-sil+x=xx\n"
	if err := os.WriteFile(labelPath, []byte(labels), 0o644); err != nil {
		t.Fatal(err)
	}
	record, err := ExtractRecord("test", "あいう。", wavPath, ExtractConfig{HTSLabelPath: labelPath})
	if err != nil {
		t.Fatal(err)
	}
	if len(record.Tokens) != 4 {
		t.Fatalf("got %d tokens, want 4", len(record.Tokens))
	}
	wantDurations := []float64{200, 200, 200, 300}
	for i, want := range wantDurations {
		if math.Abs(record.Tokens[i].DurationMS-want) > 0.1 {
			t.Fatalf("token %d duration = %.2fms, want %.2fms", i, record.Tokens[i].DurationMS, want)
		}
	}
}
