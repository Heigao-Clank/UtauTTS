package prosody

import (
	"math"
	"testing"

	"utautts/internal/frontend"
)

func TestManualPitchCurveInterpolatesMoraCenters(t *testing.T) {
	file := &ManualPitchFile{Version: 1, Mode: "offset", Points: []ManualPitchPoint{
		{Position: 0, Mora: "あ", Cents: 0},
		{Position: 2, Mora: "う", Cents: 100},
	}}
	morae := []frontend.Mora{{Text: "あ"}, {Text: "い"}, {Text: "う"}}
	timings := []MoraTiming{{StartMS: 0, DurationMS: 100}, {StartMS: 100, DurationMS: 100}, {StartMS: 200, DurationMS: 100}}
	curve, err := file.Curve(morae, timings, 300)
	if err != nil {
		t.Fatal(err)
	}
	if curve.FrameMS != 10 || len(curve.Cents) != 32 {
		t.Fatalf("curve metadata = %+v", curve)
	}
	if math.Abs(curve.Cents[5]) > 0.01 || math.Abs(curve.Cents[15]) > 0.01 || math.Abs(curve.Cents[20]-50) > 0.01 || math.Abs(curve.Cents[25]-100) > 0.01 {
		t.Fatalf("curve values = %.2f, %.2f, %.2f, %.2f", curve.Cents[5], curve.Cents[15], curve.Cents[20], curve.Cents[25])
	}
}

func TestManualPitchCurveKeepsUnspecifiedMoraeAtZero(t *testing.T) {
	file := &ManualPitchFile{Version: 1, Points: []ManualPitchPoint{{Position: 1, Cents: 80}}}
	morae := []frontend.Mora{{Text: "あ"}, {Text: "い"}, {Text: "う"}}
	timings := []MoraTiming{{StartMS: 0, DurationMS: 100}, {StartMS: 100, DurationMS: 100}, {StartMS: 200, DurationMS: 100}}
	curve, err := file.Curve(morae, timings, 300)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(curve.Cents[5]) > 0.01 || math.Abs(curve.Cents[15]-80) > 0.01 || math.Abs(curve.Cents[25]) > 0.01 {
		t.Fatalf("sparse curve centers = %.2f, %.2f, %.2f", curve.Cents[5], curve.Cents[15], curve.Cents[25])
	}
}

func TestManualPitchCurveRejectsPauseAndMoraMismatch(t *testing.T) {
	morae := []frontend.Mora{{Text: "あ"}, {Pause: true}}
	timings := []MoraTiming{{StartMS: 0, DurationMS: 100}, {StartMS: 100, DurationMS: 100}}
	for _, point := range []ManualPitchPoint{{Position: 1, Cents: 10}, {Position: 0, Mora: "い", Cents: 10}} {
		_, err := (&ManualPitchFile{Version: 1, Points: []ManualPitchPoint{point}}).Curve(morae, timings, 200)
		if err == nil {
			t.Fatalf("accepted invalid point %+v", point)
		}
	}
}
