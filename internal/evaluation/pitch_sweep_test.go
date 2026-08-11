package evaluation

import (
	"math"
	"testing"

	"utautts/internal/frontend"
)

func TestDeterministicPitchSweep(t *testing.T) {
	morae := []frontend.Mora{{Text: "あ"}, {Pause: true}, {Text: "い"}, {Text: "う"}, {Text: "え"}, {Text: "お"}}
	cases := DeterministicPitchSweep(morae)
	if len(cases) != 9 {
		t.Fatalf("got %d cases", len(cases))
	}
	for _, item := range cases {
		if len(item.Cents) != len(morae) || len(item.PitchFactors) != len(morae) {
			t.Fatalf("%s has wrong contour length", item.ID)
		}
		if item.Cents[1] != 0 || item.PitchFactors[1] != 1 {
			t.Fatalf("%s changed pause slot", item.ID)
		}
	}
	plus100 := cases[3]
	if plus100.ID != "plus100" || math.Abs(plus100.PitchFactors[0]-math.Pow(2, 1.0/12)) > 1e-12 {
		t.Fatalf("unexpected +100 case: %+v", plus100)
	}
	hill := cases[7]
	if hill.ID != "hill100" || math.Abs(hill.Cents[0]) > 1e-9 || math.Abs(hill.Cents[5]) > 1e-9 || hill.Cents[3] < 99 {
		t.Fatalf("unexpected hill: %+v", hill.Cents)
	}
}

func TestDeterministicFramePitchSweepHasAbruptStep(t *testing.T) {
	cases := DeterministicFramePitchSweep(100, 10)
	if len(cases) != 9 || len(cases[0].Cents) != 11 {
		t.Fatalf("unexpected frame sweep dimensions: %+v", cases)
	}
	step := cases[8].Cents
	if step[4] != 0 || step[5] != 100 {
		t.Fatalf("step was smoothed: %v", step)
	}
	hill := cases[7].Cents
	if math.Abs(hill[5]-100) > 1e-9 {
		t.Fatalf("hill peak = %f", hill[5])
	}
}
