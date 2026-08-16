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

func TestFramePitchModelPredictsBoundedContinuousContour(t *testing.T) {
	model := &Model{
		Version: FramePitchModelVersion, FeatureVersion: 1, Mode: "intonation_frame_tcn_accent_bounded",
		FramePitch: &FramePitchModel{
			FeatureNames: []string{"mora_progress"}, InputWeights: [][]float64{{2}}, InputBias: []float64{0},
			OutputWeight: []float64{100}, FrameMS: 10, LowCents: -60, HighCents: 60,
		},
	}
	morae := []frontend.Mora{{Text: "a", Vowel: "a"}, {Pause: true}, {Text: "i", Vowel: "i"}}
	timings := []MoraTiming{{StartMS: 0, DurationMS: 100}, {StartMS: 100, DurationMS: 50}, {StartMS: 150, DurationMS: 100}}
	curve := model.PredictFrameContour(morae, nil, timings, 250, false)
	if curve == nil || curve.FrameMS != 10 || len(curve.Cents) != 26 {
		t.Fatalf("curve=%+v", curve)
	}
	for index, cents := range curve.Cents {
		if cents < -60 || cents > 60 {
			t.Fatalf("frame %d out of bounds: %f", index, cents)
		}
	}
	for index := 10; index < 15; index++ {
		if curve.Cents[index] != 0 {
			t.Fatalf("pause frame %d=%f, want zero", index, curve.Cents[index])
		}
	}
}

func TestFramePitchModelAppliesRendererStrengthAfterCentering(t *testing.T) {
	makeModel := func(strength float64) *Model {
		return &Model{
			Version: FramePitchModelVersion, FeatureVersion: 1, Mode: "intonation_frame_tcn_accent_bounded",
			FramePitch: &FramePitchModel{
				FeatureNames: []string{"frame_position"}, InputWeights: [][]float64{{2}}, InputBias: []float64{0},
				OutputWeight: []float64{100}, FrameMS: 10, LowCents: -1000, HighCents: 1000, RenderStrength: strength,
				RenderSmoothingMS: 0.0001, RenderP99Cents: 10000, RenderMaxCents: 10000,
			},
		}
	}
	morae := []frontend.Mora{{Text: "a", Vowel: "a"}}
	timings := []MoraTiming{{StartMS: 0, DurationMS: 100}}
	full := makeModel(1).PredictFrameContour(morae, nil, timings, 100, false)
	half := makeModel(0.5).PredictFrameContour(morae, nil, timings, 100, false)
	for index := range full.Cents {
		if math.Abs(half.Cents[index]-full.Cents[index]*0.5) > 1e-9 {
			t.Fatalf("frame %d full=%f half=%f", index, full.Cents[index], half.Cents[index])
		}
	}
}

func TestFramePitchModelSafetyLimitsEffectiveContour(t *testing.T) {
	model := &Model{
		Version: FramePitchModelVersion, FeatureVersion: 1, Mode: "intonation_frame_tcn_accent_bounded",
		FramePitch: &FramePitchModel{
			FeatureNames: []string{"frame_position"}, InputWeights: [][]float64{{8}}, InputBias: []float64{-4},
			OutputWeight: []float64{1000}, FrameMS: 10, LowCents: -250, HighCents: 250,
			RenderStrength: 0.32, RenderSmoothingMS: 20, RenderP99Cents: 75, RenderMaxCents: 90,
		},
	}
	curve := model.PredictFrameContour(
		[]frontend.Mora{{Text: "a", Vowel: "a"}}, nil,
		[]MoraTiming{{StartMS: 0, DurationMS: 200}}, 200, false,
	)
	mask := make([]bool, len(curve.Cents))
	for index := range mask {
		mask[index] = true
		if math.Abs(curve.Cents[index]) > 90.000001 {
			t.Fatalf("frame %d exceeded renderer maximum: %f", index, curve.Cents[index])
		}
	}
	if got := absolutePercentile(curve.Cents, mask, 0.99); got > 75.000001 {
		t.Fatalf("p99=%f, want <=75", got)
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

func TestProsodyMultitaskModelPredictsMoraDurationAndLoads(t *testing.T) {
	model := &Model{
		Version: ProsodyMultitaskModelVersion, FeatureVersion: 2, Mode: "prosody_multitask_tcn",
		MoraDuration: &SequencePitchModel{
			FeatureNames: []string{"position"},
			InputWeights: [][]float64{{1}}, InputBias: []float64{0},
			OutputWeight: []float64{2}, Low: 0.5, High: 2,
		},
		FramePitch: &FramePitchModel{
			FeatureNames: []string{"frame_position"}, InputWeights: [][]float64{{1}}, InputBias: []float64{0},
			OutputWeight: []float64{1}, FrameMS: 10, LowCents: -120, HighCents: 120,
		},
	}
	morae := []frontend.Mora{
		{Text: "a", Vowel: "a"}, {Text: "i", Vowel: "i"}, {Text: "u", Vowel: "u"},
	}
	predicted := model.Predict(morae)
	if predicted[0].DurationFactor >= 1 || predicted[2].DurationFactor <= 1 {
		t.Fatalf("mora duration head did not produce relative factors: %#v", predicted)
	}
	if !model.HasFrameContour() {
		t.Fatal("multitask model did not report frame contour")
	}
	path := filepath.Join(t.TempDir(), "prosody-multitask-v1.json")
	if err := model.Save(path); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadModel(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := loaded.Predict(morae)[2].DurationFactor; math.Abs(got-predicted[2].DurationFactor) > 1e-9 {
		t.Fatalf("multitask round trip changed duration prediction: %v != %v", got, predicted[2].DurationFactor)
	}
}

func TestProsodyMultitaskModelReportsExternalFeaturesFromEitherHead(t *testing.T) {
	model := &Model{
		Version: ProsodyMultitaskModelVersion, FeatureVersion: 2, Mode: "prosody_multitask_tcn",
		MoraDuration: &SequencePitchModel{
			FeatureNames: []string{"accent_high"}, InputWeights: [][]float64{{1}}, InputBias: []float64{0},
			OutputWeight: []float64{1}, Low: 0.5, High: 2,
		},
		FramePitch: &FramePitchModel{
			FeatureNames: []string{"frame_position"}, InputWeights: [][]float64{{1}}, InputBias: []float64{0},
			OutputWeight: []float64{1}, FrameMS: 10, LowCents: -120, HighCents: 120,
		},
	}
	if !model.RequiresExternalFeatures() {
		t.Fatal("mora duration accent features were not reported")
	}
}

func TestLoadAccentSequenceModelVersions(t *testing.T) {
	for _, test := range []struct {
		version int
		mode    string
	}{
		{AccentSequenceModelVersion, "intonation_tcn_accent"},
		{BoundedSequenceModelVersion, "intonation_tcn_accent_bounded"},
	} {
		t.Run(test.mode, func(t *testing.T) {
			model := &Model{
				Version: test.version, FeatureVersion: 1, Mode: test.mode,
				SequencePitch: &SequencePitchModel{
					FeatureNames: []string{"bias"},
					InputWeights: [][]float64{{1}}, InputBias: []float64{0},
					OutputWeight: []float64{1}, Low: 0.97, High: 1.03,
				},
			}
			path := filepath.Join(t.TempDir(), "model.json")
			if err := model.Save(path); err != nil {
				t.Fatal(err)
			}
			loaded, err := LoadModel(path)
			if err != nil {
				t.Fatal(err)
			}
			if loaded.Version != test.version || loaded.Mode != test.mode {
				t.Fatalf("loaded model = %d/%q", loaded.Version, loaded.Mode)
			}
		})
	}
}

func TestAccentSequenceModelUsesExternalFeatureFrames(t *testing.T) {
	model := &Model{
		Version: AccentSequenceModelVersion, FeatureVersion: 1, Mode: "intonation_tcn_accent",
		SequencePitch: &SequencePitchModel{
			FeatureNames: []string{"accent_high"},
			InputWeights: [][]float64{{2}}, InputBias: []float64{0},
			OutputWeight: []float64{1}, Low: 0.8, High: 1.2,
		},
	}
	if !model.RequiresExternalFeatures() {
		t.Fatal("accent model did not report its external feature requirement")
	}
	morae := []frontend.Mora{{Text: "あ", Vowel: "a"}, {Text: "い", Vowel: "i"}}
	predicted := model.PredictWithFeatures(morae, []FeatureFrame{{"accent_high": 1}, {"accent_high": 0}})
	if predicted[0].PitchFactor <= predicted[1].PitchFactor {
		t.Fatalf("external accent feature did not affect prediction: %#v", predicted)
	}
}

func TestStandardAccentContourFollowsHighLowPattern(t *testing.T) {
	model := &Model{
		Version: StandardAccentModelVersion, FeatureVersion: 1, Mode: "standard_japanese_accent",
		StandardAccent: &StandardAccentModel{
			FrameMS: 10, AccentRangeCents: 70, DeclinationCents: 10,
			QuestionRiseCents: 35, SmoothingMS: 20, P99Cents: 65, MaxCents: 80,
		},
	}
	if !model.RequiresExternalFeatures() || !model.HasFrameContour() {
		t.Fatal("standard accent model did not report its feature/contour requirements")
	}
	morae := []frontend.Mora{{Text: "あ", Vowel: "a"}, {Text: "い", Vowel: "i"}, {Text: "う", Vowel: "u"}}
	frames := []FeatureFrame{
		{"accent_high": 0, "accent_position": 0.33},
		{"accent_high": 1, "accent_position": 0.66},
		{"accent_high": 0, "accent_position": 1},
	}
	timings := []MoraTiming{{StartMS: 0, DurationMS: 100}, {StartMS: 100, DurationMS: 100}, {StartMS: 200, DurationMS: 100}}
	contour := model.PredictFrameContour(morae, frames, timings, 300, false)
	if contour == nil {
		t.Fatal("standard accent model returned no contour")
	}
	if contour.Cents[15] <= contour.Cents[5] || contour.Cents[25] >= contour.Cents[15] {
		t.Fatalf("contour did not follow low-high-low accent: %.2f %.2f %.2f", contour.Cents[5], contour.Cents[15], contour.Cents[25])
	}
	path := filepath.Join(t.TempDir(), "standard-accent.json")
	if err := model.Save(path); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadModel(path); err != nil {
		t.Fatal(err)
	}
}

func TestPhraseAnchorV9ProducesSmoothContourAndLoads(t *testing.T) {
	model := &Model{
		Version: StandardAccentModelVersion, FeatureVersion: 1, Mode: "intonation_phrase_anchor_v9",
		PhrasePitch: &PhrasePitchModel{
			FeatureNames: []string{"bias"},
			Weights:      [][]float64{{0}, {0}, {0}, {0}}, Bias: []float64{0, 20, -10, 30},
			FrameMS: 10, LowCents: -120, HighCents: 120,
			AccentRangeCents: 60, DeclinationCents: 10, SmoothingMS: 20,
			P99Cents: 90, MaxCents: 100,
		},
	}
	morae := []frontend.Mora{{Text: "a", Vowel: "a"}, {Text: "i", Vowel: "i"}, {Text: "u", Vowel: "u"}}
	frames := []FeatureFrame{
		{"accent_phrase_start": 1, "accent_phrase_position": 1, "accent_position": 0.33, "accent_nucleus_position": 0.66, "accent_high": 0},
		{"accent_position": 0.66, "accent_nucleus_position": 0.66, "accent_high": 1},
		{"accent_phrase_end": 1, "accent_position": 1, "accent_nucleus_position": 0.66, "accent_high": 0},
	}
	timings := []MoraTiming{{StartMS: 0, DurationMS: 100}, {StartMS: 100, DurationMS: 100}, {StartMS: 200, DurationMS: 100}}
	curve := model.PredictFrameContour(morae, frames, timings, 300, false)
	if curve == nil || len(curve.Cents) < 3 {
		t.Fatal("v9 phrase model returned no contour")
	}
	for _, value := range curve.Cents {
		if value < -120 || value > 120 {
			t.Fatalf("v9 contour exceeded bounds: %f", value)
		}
	}
	if curve.Cents[0] == curve.Cents[len(curve.Cents)-1] {
		t.Fatalf("v9 anchors did not affect contour: %v", curve.Cents)
	}
	path := filepath.Join(t.TempDir(), "v9.json")
	if err := model.Save(path); err != nil {
		t.Fatal(err)
	}
	if loaded, err := LoadModel(path); err != nil || loaded.PhrasePitch == nil {
		t.Fatalf("v9 model did not round-trip: model=%#v err=%v", loaded, err)
	}
	model.Mode = "intonation_phrase_anchor_v9_1"
	v91Path := filepath.Join(t.TempDir(), "v9-1.json")
	if err := model.Save(v91Path); err != nil {
		t.Fatal(err)
	}
	if loaded, err := LoadModel(v91Path); err != nil || loaded.PhrasePitch == nil {
		t.Fatalf("v9.1 model did not round-trip: model=%#v err=%v", loaded, err)
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
