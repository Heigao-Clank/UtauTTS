package main

import (
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
