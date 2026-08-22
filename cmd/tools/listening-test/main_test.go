package main

import (
	"os"
	"path/filepath"
	"testing"

	"utautts/internal/plan"
)

func TestSelectionChangesKeepsPrivateCandidateDetails(t *testing.T) {
	left := &plan.Plan{Units: []plan.Unit{{Position: 2, Mora: "き", Alias: "n き", Source: "a.wav", OtoLine: 3, TargetScore: 114}}}
	right := &plan.Plan{Units: []plan.Unit{{Position: 2, Mora: "き", Alias: "n キ", Source: "b.wav", OtoLine: 4, TargetScore: 114, JoinProbability: 0.7}}}
	changes := selectionChanges(left, right)
	if len(changes) != 1 || changes[0].A.Alias != "n き" || changes[0].B.JoinProbability != 0.7 {
		t.Fatalf("unexpected changes: %+v", changes)
	}
}

func TestLoadMoraDurations(t *testing.T) {
	path := filepath.Join(t.TempDir(), "durations.json")
	data := []byte(`{"version":1,"cases":[{"id":"a","mora_durations_ms":[100,80,180],"pause_duration_ms":180}]}`)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := loadMoraDurations(path)
	if err != nil {
		t.Fatal(err)
	}
	if got["a"].MoraDurationsMS[1] != 80 || got["a"].PauseDurationMS != 180 {
		t.Fatalf("unexpected durations: %+v", got["a"])
	}
}

func TestLoadMoraDurationsRejectsNonFiniteValue(t *testing.T) {
	path := filepath.Join(t.TempDir(), "durations.json")
	data := []byte(`{"version":1,"cases":[{"id":"a","mora_durations_ms":[1e999]}]}`)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadMoraDurations(path); err == nil {
		t.Fatal("expected invalid duration corpus to fail")
	}
}

func TestMissingCaseInputOnlyRequiresConfiguredSources(t *testing.T) {
	if got := missingCaseInput(caseInput{name: "unused", present: false}); got != "" {
		t.Fatalf("unconfigured source reported as missing: %q", got)
	}
	if got := missingCaseInput(caseInput{name: "durations", path: "durations.json", present: false}); got != "durations" {
		t.Fatalf("missing source = %q", got)
	}
}
