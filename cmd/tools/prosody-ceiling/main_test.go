package main

import (
	"testing"

	"utautts/internal/prosody"
)

func TestBuildAssetsPreservesReferenceDurationAndFillsUnvoicedPitch(t *testing.T) {
	records := []prosody.Record{{
		Version: 1, ID: "case-1", Text: "赤。", Reading: "アカ。", AudioPath: "a.wav",
		Tokens: []prosody.Target{
			{Mora: "あ", DurationMS: 110, PitchRatio: 1.1},
			{Mora: "か", DurationMS: 90},
			{Pause: true, DurationMS: 180},
		},
	}}
	corpus, durations, pitches, references := buildAssets(records, "test", "input.jsonl", 0, 1)
	if len(corpus.Cases) != 1 || corpus.Cases[0].Reading != "アカ。" {
		t.Fatalf("unexpected corpus: %+v", corpus)
	}
	if got := durations.Cases[0]; got.MoraDurationsMS[1] != 90 || got.PauseDurationMS != 180 {
		t.Fatalf("unexpected durations: %+v", got)
	}
	if got := pitches.Cases[0].PitchFactors; got[0] != 1.1 || got[1] != 1.1 || got[2] != 1 {
		t.Fatalf("unexpected pitch factors: %#v", got)
	}
	if len(references.Skipped) != 0 {
		t.Fatalf("unexpected skips: %#v", references.Skipped)
	}
}

func TestBuildAssetsSkipsMisalignedRecord(t *testing.T) {
	record := prosody.Record{Version: 1, ID: "bad", Text: "赤", Reading: "アカ", Tokens: []prosody.Target{{Mora: "あ", DurationMS: 100}}}
	corpus, _, _, references := buildAssets([]prosody.Record{record}, "test", "input.jsonl", 0, 1)
	if len(corpus.Cases) != 0 || len(references.Skipped) != 1 {
		t.Fatalf("cases=%d skipped=%#v", len(corpus.Cases), references.Skipped)
	}
}
