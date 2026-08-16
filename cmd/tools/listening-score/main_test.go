package main

import (
	"math"
	"testing"
)

func TestWilson95(t *testing.T) {
	value := wilson95(5, 10)
	if math.Abs(value.Low-0.2366) > 0.001 || math.Abs(value.High-0.7634) > 0.001 {
		t.Fatalf("unexpected interval: %+v", value)
	}
}

func TestSystemNameIncludesModelIdentity(t *testing.T) {
	got := systemName(systemInfo{Renderer: "waveform", JoinModel: true, JoinModelPath: "models/join.json"})
	if got != "waveform+learned:join.json" {
		t.Fatalf("unexpected name %q", got)
	}
}
