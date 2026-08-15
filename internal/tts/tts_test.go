package tts

import (
	"math"
	"testing"

	"utautts/internal/frontend"
	"utautts/internal/plan"
	"utautts/internal/plugin"
	"utautts/internal/prosody"
	"utautts/internal/render"
)

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

func TestValidateRuntimeMoraAlignment(t *testing.T) {
	morae := []frontend.Mora{{Text: "きょ"}, {Text: "う"}, {Pause: true}, {Text: "は"}}
	if err := validateRuntimeMoraAlignment(morae, []string{"きょ", "う", "", "は"}); err != nil {
		t.Fatal(err)
	}
	if err := validateRuntimeMoraAlignment(morae, []string{"きょ", "お", "", "は"}); err == nil {
		t.Fatal("mismatched Open JTalk mora was accepted")
	}
	if err := validateRuntimeMoraAlignment(morae, []string{"きょ", "う", "は"}); err == nil {
		t.Fatal("mismatched Open JTalk frame count was accepted")
	}
}

func TestValidateRuntimeMoraAlignmentAcceptsOpenJTalkLongVowelNotation(t *testing.T) {
	morae := []frontend.Mora{
		{Text: "\u305b", Vowel: "e"},
		{Text: "\u3044", Vowel: "i"},
		{Text: "\u3044", Vowel: "i"},
	}
	if err := validateRuntimeMoraAlignment(morae, []string{"\u305b", "\u30fc", "\u3044"}); err != nil {
		t.Fatalf("long vowel notation was rejected: %v", err)
	}

	nonVowelMora := []frontend.Mora{{Text: "\u3093", Vowel: "n"}}
	if err := validateRuntimeMoraAlignment(nonVowelMora, []string{"\u30fc"}); err == nil {
		t.Fatal("long vowel notation was accepted for a non-vowel mora")
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
