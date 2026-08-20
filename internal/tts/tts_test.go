package tts

import (
	"context"
	"errors"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"utautts/internal/frontend"
	"utautts/internal/openjtalk"
	"utautts/internal/plan"
	"utautts/internal/plugin"
	"utautts/internal/prosody"
	"utautts/internal/render"
)

func TestSynthesizeHonorsCanceledContextBeforeLoadingInputs(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Synthesize(Config{Context: ctx, VoicebankPath: filepath.Join(t.TempDir(), "missing")})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}

func TestMoraTimingsIncludePausesMissingFromPlanUnits(t *testing.T) {
	morae := []frontend.Mora{{Text: "a"}, {Pause: true}, {Text: "i"}}
	p := &plan.Plan{DurationMS: 380, Units: []plan.Unit{
		{Position: 0, NoteStartMS: 0, DurationMS: 100},
		{Position: 2, NoteStartMS: 280, DurationMS: 100},
	}}
	got := moraTimings(morae, p)
	if len(got) != 3 || got[0].DurationMS != 100 || got[1].StartMS != 100 || got[1].DurationMS != 180 || got[2].StartMS != 280 {
		t.Fatalf("timings=%+v", got)
	}
}

func TestMergeManualPitchCurveAddsToLearnedCurve(t *testing.T) {
	base := &render.PitchCurve{FrameMS: 20, Cents: []float64{10, 30, 50}}
	manual := &prosody.PitchContour{FrameMS: 10, Cents: []float64{0, 10, 20, 30, 40}}
	got := mergeManualPitchCurve(base, manual, "offset")
	want := []float64{10, 30, 50, 70, 90}
	for index, value := range want {
		if math.Abs(got.Cents[index]-value) > 1e-9 {
			t.Fatalf("merged[%d] = %.2f, want %.2f", index, got.Cents[index], value)
		}
	}
}

func TestMergeManualPitchCurveCanReplaceLearnedCurve(t *testing.T) {
	base := &render.PitchCurve{FrameMS: 10, Cents: []float64{100, 100}}
	manual := &prosody.PitchContour{FrameMS: 10, Cents: []float64{0, -20}}
	got := mergeManualPitchCurve(base, manual, "replace")
	if got.Cents[0] != 0 || got.Cents[1] != -20 {
		t.Fatalf("replacement curve = %#v", got.Cents)
	}
}

func TestScaleAutomaticPitchCurveUsesTheConfiguredStrength(t *testing.T) {
	base := &render.PitchCurve{FrameMS: 10, Cents: []float64{20, -40}}
	if got := scaleAutomaticPitchCurve(base, 0); got != nil {
		t.Fatal("zero intonation strength kept the automatic curve")
	}
	if got := scaleAutomaticPitchCurve(base, 1); got != base {
		t.Fatal("normal intonation strength unnecessarily copied the curve")
	}
	got := scaleAutomaticPitchCurve(base, 2)
	if got == base || !reflect.DeepEqual(got.Cents, []float64{40, -80}) {
		t.Fatalf("amplified curve = %#v", got)
	}
	if !reflect.DeepEqual(base.Cents, []float64{20, -40}) {
		t.Fatal("amplifying the curve mutated the source")
	}
}

func TestIntonationStrengthAcceptsAmplificationRange(t *testing.T) {
	if err := validateConfig(Config{IntonationStrength: render.MaxIntonationStrength}); err != nil {
		t.Fatalf("maximum intonation strength rejected: %v", err)
	}
	if err := validateConfig(Config{IntonationStrength: render.MaxIntonationStrength + 0.01}); err == nil {
		t.Fatal("intonation strength above the maximum was accepted")
	}
}

func TestPredictProsodyDoesNotRenderAudio(t *testing.T) {
	preview, err := PredictProsody(Config{
		Reading:         "あいう",
		MoraDurationMS:  100,
		PauseDurationMS: 180,
		ApplyPitch:      false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if preview.Reading != "あいう" || len(preview.Morae) != 3 {
		t.Fatalf("unexpected preview reading/morae: %#v", preview)
	}
	if !reflect.DeepEqual(preview.MoraDurationsMS, []float64{100, 100, 100}) {
		t.Fatalf("durations = %#v", preview.MoraDurationsMS)
	}
	if !reflect.DeepEqual(preview.MoraPositionsMS, []float64{50, 150, 250}) {
		t.Fatalf("positions = %#v", preview.MoraPositionsMS)
	}
	if !reflect.DeepEqual(preview.PitchPoints, []float64{0, 0, 0}) {
		t.Fatalf("pitch points = %#v", preview.PitchPoints)
	}
}

func TestMoraTimingsDistributeConsecutiveTrailingPauses(t *testing.T) {
	morae := []frontend.Mora{{Text: "a"}, {Pause: true}, {Pause: true}}
	p := &plan.Plan{DurationMS: 300, Units: []plan.Unit{{Position: 0, NoteStartMS: 0, DurationMS: 100}}}
	got := moraTimings(morae, p)
	if got[1].DurationMS != 100 || got[2].StartMS != 200 || got[2].DurationMS != 100 {
		t.Fatalf("timings=%+v", got)
	}
}

func TestExternalPitchFactorsDoNotImplicitlyEnableWaveformPitchProcessing(t *testing.T) {
	if applyPitchEnabled(Config{PitchFactors: []float64{1.02}}) {
		t.Fatal("external pitch targets implicitly enabled waveform pitch processing")
	}
	if !applyPitchEnabled(Config{PitchFactors: []float64{1.02}, ApplyPitch: true}) {
		t.Fatal("explicit ApplyPitch did not enable waveform pitch processing")
	}
	if !applyPitchEnabled(Config{ProsodyPitchOnly: true}) {
		t.Fatal("ProsodyPitchOnly did not enable pitch processing")
	}
}

func TestPitchProcessingSwitchControlsModelFrameContour(t *testing.T) {
	model := &prosody.Model{FramePitch: &prosody.FramePitchModel{}}
	capabilities := &plugin.Capabilities{FramePitch: true}
	if shouldPredictFrameContour(Config{RendererCapabilities: capabilities}, model) {
		t.Fatal("model frame contour was enabled while pitch processing was off")
	}
	if !shouldPredictFrameContour(Config{ApplyPitch: true, RendererCapabilities: capabilities}, model) {
		t.Fatal("model frame contour was disabled while pitch processing was on")
	}
	if got := effectiveIntonationStrength(Config{IntonationStrength: 0.5}); got != 0 {
		t.Fatalf("disabled pitch processing kept intonation strength %.2f", got)
	}
	if got := effectiveIntonationStrength(Config{ApplyPitch: true, IntonationStrength: 0.5}); got != 0.5 {
		t.Fatalf("enabled pitch processing changed intonation strength to %.2f", got)
	}
}

func TestFaithfulGPUSupportsFramePitch(t *testing.T) {
	if !rendererSupportsFramePitch("openutau-classic-worldline-faithful-gpu", nil) {
		t.Fatal("GPU faithful renderer rejected a frame pitch contour")
	}
}

func TestWaveformRendererSupportsFramePitch(t *testing.T) {
	if !rendererSupportsFramePitch("waveform", nil) {
		t.Fatal("waveform renderer rejected a frame pitch contour")
	}
}

func TestAlignRuntimeProsodyFeaturesAcceptsAlternatePronunciations(t *testing.T) {
	morae := []frontend.Mora{
		{Text: "\u3044", Vowel: "i"},
		{Text: "\u304b", Vowel: "a"},
		{Text: "\u308a", Vowel: "i"},
	}
	analysis := &openjtalk.Analysis{
		Morae: []string{"\u304a", "\u3053", "\u308a"},
		Features: []prosody.FeatureFrame{
			{"source": 0},
			{"source": 1},
			{"source": 2},
		},
	}
	aligned, err := alignRuntimeProsodyFeatures(morae, analysis)
	if err != nil {
		t.Fatal(err)
	}
	for index, source := range []float64{0, 1, 2} {
		if aligned[index]["source"] != source {
			t.Fatalf("aligned[%d] = %#v, want source %.0f", index, aligned[index], source)
		}
	}
}

func TestAlignRuntimeProsodyFeaturesAcceptsLongVowelNotation(t *testing.T) {
	morae := []frontend.Mora{{Text: "\u305b", Vowel: "e"}, {Text: "\u3044", Vowel: "i"}, {Text: "\u3044", Vowel: "i"}}
	analysis := &openjtalk.Analysis{
		Morae:    []string{"\u305b", "\u30fc", "\u3044"},
		Features: []prosody.FeatureFrame{{"source": 0}, {"source": 1}, {"source": 2}},
	}
	aligned, err := alignRuntimeProsodyFeatures(morae, analysis)
	if err != nil {
		t.Fatalf("long vowel notation was rejected: %v", err)
	}
	if aligned[1]["source"] != 1 {
		t.Fatalf("aligned long vowel feature = %#v", aligned[1])
	}
}

func TestAlignRuntimeProsodyFeaturesFillsMissingGoMora(t *testing.T) {
	morae := []frontend.Mora{{Text: "こ"}, {Text: "れ"}, {Text: "い"}}
	analysis := &openjtalk.Analysis{
		Morae:    []string{"こ", "れ"},
		Features: []prosody.FeatureFrame{{"source": 0}, {"source": 1}},
	}
	aligned, err := alignRuntimeProsodyFeatures(morae, analysis)
	if err != nil {
		t.Fatal(err)
	}
	if len(aligned) != len(morae) || aligned[2]["source"] != 1 {
		t.Fatalf("aligned features = %#v", aligned)
	}
}

func TestAlignRuntimeProsodyFeaturesSkipsExtraOpenJTalkMorae(t *testing.T) {
	morae := []frontend.Mora{{Text: "\u3053"}, {Text: "\u3044"}}
	analysis := &openjtalk.Analysis{
		Morae: []string{"\u3053", "\u304f", "\u3089", "\u3044"},
		Features: []prosody.FeatureFrame{
			{"source": 0},
			{"source": 1},
			{"source": 2},
			{"source": 3},
		},
	}
	aligned, err := alignRuntimeProsodyFeatures(morae, analysis)
	if err != nil {
		t.Fatal(err)
	}
	if len(aligned) != 2 || aligned[0]["source"] != 0 || aligned[1]["source"] != 3 {
		t.Fatalf("aligned features = %#v", aligned)
	}
}

func TestAlignRuntimeProsodyFeaturesAcceptsAlternatePronunciation(t *testing.T) {
	morae := []frontend.Mora{{Text: "\u3044"}, {Text: "\u304b"}, {Text: "\u308a"}}
	analysis := &openjtalk.Analysis{
		Morae: []string{"\u304a", "\u3053", "\u308a"},
		Features: []prosody.FeatureFrame{
			{"source": 0},
			{"source": 1},
			{"source": 2},
		},
	}
	aligned, err := alignRuntimeProsodyFeatures(morae, analysis)
	if err != nil {
		t.Fatal(err)
	}
	if len(aligned) != 3 || aligned[0]["source"] != 0 || aligned[2]["source"] != 2 {
		t.Fatalf("aligned features = %#v", aligned)
	}
}

func TestValidateConfigRejectsNonFiniteValues(t *testing.T) {
	for _, cfg := range []Config{
		{MoraDurationMS: math.NaN()},
		{PauseDurationMS: math.Inf(1)},
		{ReleaseMS: math.Inf(-1)},
		{PitchFactors: []float64{math.NaN()}},
	} {
		if err := validateConfig(cfg); err == nil {
			t.Fatalf("accepted invalid config: %#v", cfg)
		}
	}
}

func TestConvertToReadingUsesBuiltInTokenizer(t *testing.T) {
	reading, err := ConvertToReading("こんにちは。", nil, openjtalk.Config{})
	if err != nil || reading == "" {
		t.Fatalf("kana text failed: %v", err)
	}
}

func TestConvertToReadingReportsOpenJTalkFallback(t *testing.T) {
	// 数字は内蔵フロントエンドではトークナイズできない。フォールバック先を存在しないヘルパーに
	// 向けることで、このチェックアウトに同梱のOpen JTalkヘルパーの有無にかかわらず、
	// 合成エラーを決定的にする。
	_, err := ConvertToReading("2024年です。", nil, openjtalk.Config{
		HelperPath: filepath.Join(t.TempDir(), "missing-helper"),
	})
	if err == nil {
		t.Fatal("text the tokenizer cannot read was silently accepted")
	}
	if !strings.Contains(err.Error(), "convert text to reading") || !strings.Contains(err.Error(), "Open JTalk fallback") {
		t.Fatalf("combined fallback error missing context: %v", err)
	}
}

func TestVoicebankCacheInvalidatedByClearCaches(t *testing.T) {
	bankDir := t.TempDir()
	if err := os.MkdirAll(bankDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bankDir, "oto.ini"), []byte("a.wav=あ,0,0,0,0,0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	first, err := loadVoicebankCached(bankDir)
	if err != nil {
		t.Fatal(err)
	}
	second, err := loadVoicebankCached(bankDir)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatal("voicebank was loaded more than once")
	}
	ClearCaches()
	third, err := loadVoicebankCached(bankDir)
	if err != nil {
		t.Fatal(err)
	}
	if first == third {
		t.Fatal("voicebank cache survived ClearCaches")
	}
}
