package render

import (
	"math"
	"reflect"
	"testing"

	"utautts/internal/audio"
	"utautts/internal/plan"
)

func TestRenderIsDeterministicAndUsesAbsolutePlacement(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/unit.wav"
	data := make([]int16, 300)
	for i := range data {
		data[i] = 10000
	}
	if err := audio.WriteWav(path, &audio.PCM{SampleRate: 1000, Channels: 1, Data: data}); err != nil {
		t.Fatal(err)
	}
	p := &plan.Plan{DurationMS: 200, Units: []plan.Unit{{
		Alias: "あ", Source: path, NoteStartMS: 100, DurationMS: 100,
		PreutteranceMS: 100, OverlapMS: 0,
	}}}
	first, err := Render(p, Config{ReleaseMS: 20})
	if err != nil {
		t.Fatal(err)
	}
	second, err := Render(p, Config{ReleaseMS: 20})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatal("render is not deterministic")
	}
	if len(first.Data) != 220 {
		t.Fatalf("frames = %d, want 220", len(first.Data))
	}
	if first.Data[0] != 0 || first.Data[100] == 0 || first.Data[len(first.Data)-1] != 0 {
		t.Fatalf("unexpected envelope: first=%d middle=%d last=%d", first.Data[0], first.Data[100], first.Data[len(first.Data)-1])
	}
}

func TestNormalizeTimingCompressesLongVCVAndKeepsVowelTail(t *testing.T) {
	unit := plan.Unit{DurationMS: 140, PreutteranceMS: 360, OverlapMS: 120, ConsonantMS: 439}
	got := normalizeTiming(unit, 20)
	if math.Abs(got.preutteranceMS-105) > 0.001 {
		t.Fatalf("preutterance = %.3f, want 105", got.preutteranceMS)
	}
	if math.Abs(got.overlapMS-35) > 0.001 {
		t.Fatalf("overlap = %.3f, want 35", got.overlapMS)
	}
	if got.consonantMS >= got.preutteranceMS+unit.DurationMS+20-(20+49) {
		t.Fatalf("consonant %.3f leaves no guaranteed vowel tail", got.consonantMS)
	}
}

func TestNormalizeTimingLeavesOrdinaryBankAlone(t *testing.T) {
	unit := plan.Unit{DurationMS: 140, PreutteranceMS: 60, OverlapMS: 20, ConsonantMS: 100}
	got := normalizeTiming(unit, 20)
	if got.preutteranceMS != 60 || got.overlapMS != 20 || got.consonantMS != 100 || got.scale != 1 {
		t.Fatalf("ordinary timing changed: %#v", got)
	}
}

func TestRetimeCompressedPrefixRetainsVowelTail(t *testing.T) {
	source := make([]float64, 600)
	for i := 439; i < len(source); i++ {
		source[i] = 1
	}
	got := retimeWithCompressedPrefix(source, 265, 439, 128, 1000)
	if len(got) != 265 {
		t.Fatalf("length = %d", len(got))
	}
	voiced := 0
	for _, value := range got[128:] {
		if value > 0.5 {
			voiced++
		}
	}
	if voiced < 100 {
		t.Fatalf("vowel tail only has %d frames", voiced)
	}
	maxDelta := 0.0
	maxDeltaAt := 0
	for i := 1; i < len(got); i++ {
		if delta := math.Abs(got[i] - got[i-1]); delta > maxDelta {
			maxDelta, maxDeltaAt = delta, i
		}
	}
	if maxDelta > 0.25 {
		t.Fatalf("compressed-prefix join clicks: max delta %.3f at %d", maxDelta, maxDeltaAt)
	}
}

func TestRenderLongVCVUsesWeightedCrossfade(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/vcv.wav"
	data := make([]int16, 800)
	for i := range data {
		data[i] = 10000
	}
	if err := audio.WriteWav(path, &audio.PCM{SampleRate: 1000, Channels: 1, Data: data}); err != nil {
		t.Fatal(err)
	}
	p := &plan.Plan{DurationMS: 280, Units: []plan.Unit{
		{Alias: "a", Source: path, NoteStartMS: 0, DurationMS: 140, PreutteranceMS: 360, OverlapMS: 120, ConsonantMS: 439},
		{Alias: "b", Source: path, NoteStartMS: 140, DurationMS: 140, PreutteranceMS: 360, OverlapMS: 120, ConsonantMS: 439},
	}}
	pcm, err := Render(p, Config{ReleaseMS: 20})
	if err != nil {
		t.Fatal(err)
	}
	peak := int16(0)
	for _, value := range pcm.Data {
		if value > peak {
			peak = value
		}
	}
	if peak > 10010 {
		t.Fatalf("overlap was additively mixed: peak=%d", peak)
	}
	if p.Units[1].EffectivePreutteranceMS != 105 || p.Units[1].EffectiveOverlapMS != 35 {
		t.Fatalf("effective timing not recorded: %#v", p.Units[1])
	}
}

func TestStretchPreservesPrefixAndLength(t *testing.T) {
	source := make([]float64, 200)
	for i := range source {
		source[i] = float64(i) / 200
	}
	got := stretchPreservingPrefix(source, 350, 50, 1000)
	if len(got) != 350 {
		t.Fatalf("length = %d", len(got))
	}
	if !reflect.DeepEqual(got[:50], source[:50]) {
		t.Fatal("protected prefix changed")
	}
}
