package tts

import (
	"testing"

	"utautts/internal/frontend"
	"utautts/internal/plan"
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
