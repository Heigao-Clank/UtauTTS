package evaluation

import (
	"math"
	"testing"

	"utautts/internal/audio"
	"utautts/internal/plan"
)

func TestAnalyzeDetectsConnectedBoundaryClick(t *testing.T) {
	data := make([]int16, 4000)
	for i := range data {
		value := 0.25 * math.Sin(2*math.Pi*200*float64(i)/8000)
		if i >= 2000 {
			value += 0.5
		}
		data[i] = int16(value * 32767)
	}
	p := &plan.Plan{Units: []plan.Unit{
		{Position: 0, NoteStartMS: 0}, {Position: 1, NoteStartMS: 250},
	}}
	report, err := Analyze(&audio.PCM{SampleRate: 8000, Channels: 1, Data: data}, p)
	if err != nil {
		t.Fatal(err)
	}
	if report.ConnectedCount != 1 || report.MaxClick < 0.45 || report.MaxPeakClick < report.MaxClick || report.MeanDeltaRMS <= 0 || len(report.Boundaries) != 1 {
		t.Fatalf("report = %#v", report)
	}
}

func TestAnalyzeExcludesPhraseStartFromSummary(t *testing.T) {
	data := make([]int16, 4000)
	p := &plan.Plan{Units: []plan.Unit{
		{Position: 0, NoteStartMS: 0}, {Position: 1, NoteStartMS: 100}, {Position: 3, NoteStartMS: 300},
	}}
	report, err := Analyze(&audio.PCM{SampleRate: 8000, Channels: 1, Data: data}, p)
	if err != nil {
		t.Fatal(err)
	}
	if report.ConnectedCount != 1 || report.Boundaries[1].Connected {
		t.Fatalf("report = %#v", report)
	}
}

func TestAnalyzeMeasuresRenderedHandoffRange(t *testing.T) {
	data := make([]int16, 4000)
	for i := 1680; i < len(data); i++ { // 210 ms at 8 kHz
		data[i] = 16000
	}
	p := &plan.Plan{Units: []plan.Unit{
		{Position: 0, NoteStartMS: 0},
		{
			Position: 1, NoteStartMS: 250, PreutteranceMS: 100, OverlapMS: 40,
			TimingScale: 1, EffectivePreutteranceMS: 60, EffectiveOverlapMS: 20,
		},
	}}
	report, err := Analyze(&audio.PCM{SampleRate: 8000, Channels: 1, Data: data}, p)
	if err != nil {
		t.Fatal(err)
	}
	metric := report.Boundaries[0]
	if metric.HandoffStartMS != 190 || metric.HandoffEndMS != 230 || metric.TimeMS != 210 {
		t.Fatalf("handoff metric = %#v", metric)
	}
	if metric.Click < 0.45 {
		t.Fatalf("click at actual handoff was not detected: %#v", metric)
	}
}
