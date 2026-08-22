package main

import (
	"testing"

	"utautts/internal/audio"
	"utautts/internal/plan"
)

func TestWholeRunPlanUsesOneContinuousUnit(t *testing.T) {
	base := &plan.Plan{Voicebank: "bank", Reading: "あいう"}
	pcm := &audio.PCM{SampleRate: 1000, Channels: 1, Data: make([]int16, 420)}
	got := wholeRunPlan(base, "continuous.wav", pcm)
	if got.DurationMS != 420 || got.Reading != base.Reading || len(got.Units) != 1 {
		t.Fatalf("plan = %#v", got)
	}
	unit := got.Units[0]
	if unit.Source != "continuous.wav" || unit.DurationMS != 420 || unit.PitchFactor != 1 || unit.EnergyFactor != 1 {
		t.Fatalf("unit = %#v", unit)
	}
}

func TestPCMDurationMSUsesFrames(t *testing.T) {
	pcm := &audio.PCM{SampleRate: 1000, Channels: 2, Data: make([]int16, 500)}
	if got := pcmDurationMS(pcm); got != 250 {
		t.Fatalf("duration = %v", got)
	}
}
