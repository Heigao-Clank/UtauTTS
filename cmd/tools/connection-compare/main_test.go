package main

import (
	"testing"

	"utautts/internal/plan"
)

func TestCountSelectionDifferences(t *testing.T) {
	baseline := &plan.Plan{Units: []plan.Unit{
		{Alias: "a", Source: "one.wav", OtoLine: 1},
		{Alias: "b", Source: "two.wav", OtoLine: 2},
	}}
	candidate := &plan.Plan{Units: []plan.Unit{
		{Alias: "a", Source: "one.wav", OtoLine: 1},
		{Alias: "b", Source: "other.wav", OtoLine: 2},
		{Alias: "c", Source: "three.wav", OtoLine: 3},
	}}
	if got := countSelectionDifferences(baseline, candidate); got != 2 {
		t.Fatalf("differences = %d, want 2", got)
	}
}
