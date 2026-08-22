package voicebank

import (
	"path/filepath"
	"testing"

	"utautts/internal/frontend"
	"utautts/internal/oto"
)

func TestSummarizeContinuity(t *testing.T) {
	path := []Selection{
		continuitySelection(0, "recording.wav", 0),
		continuitySelection(1, "recording.wav", 500),
		continuitySelection(2, "recording.wav", 1000),
		continuitySelection(3, "other.wav", 0),
	}
	got := summarizeContinuity(path, 3, 140, 4, ".")
	if got.ContinuousBoundaries != 2 || got.SafeBoundaries != 2 {
		t.Fatalf("boundaries = %d, safe = %d", got.ContinuousBoundaries, got.SafeBoundaries)
	}
	if len(got.Runs) != 1 || got.Runs[0].Units != 3 || got.Runs3 != 1 {
		t.Fatalf("runs = %#v", got.Runs)
	}
	if got.MedianCompression < 3.57 || got.MedianCompression > 3.58 {
		t.Fatalf("median compression = %f", got.MedianCompression)
	}
}

func TestSelectMaximumContinuityPrefersRunBeforeQuality(t *testing.T) {
	layers := [][]Selection{
		{
			continuitySelectionWithScore(0, "joined.wav", 0, 100),
			continuitySelectionWithScore(0, "isolated-a.wav", 0, 200),
		},
		{
			continuitySelectionWithScore(1, "joined.wav", 500, 100),
			continuitySelectionWithScore(1, "isolated-b.wav", 0, 200),
		},
	}
	got := selectMaximumContinuity(layers, "")
	if len(got) != 2 || filepath.Base(got[0].Entry.Filename) != "joined.wav" || filepath.Base(got[1].Entry.Filename) != "joined.wav" {
		t.Fatalf("path = %#v", got)
	}
}

func TestContinuousAnchorBoundaryRejectsCompositeAndReverseAnchor(t *testing.T) {
	left := continuitySelection(0, "recording.wav", 500)
	right := continuitySelection(1, "recording.wav", 400)
	if continuousAnchorBoundary(left, right) {
		t.Fatal("reverse anchor was accepted")
	}
	right = continuitySelection(1, "recording.wav", 1000)
	right.Composite = true
	if continuousAnchorBoundary(left, right) {
		t.Fatal("composite unit was accepted")
	}
}

func continuitySelection(position int, filename string, anchor float64) Selection {
	return continuitySelectionWithScore(position, filename, anchor, 100)
}

func continuitySelectionWithScore(position int, filename string, anchor, score float64) Selection {
	full := filepath.Join("bank", filename)
	return Selection{
		Position: position,
		Mora:     frontend.Mora{Text: "あ"},
		Alias:    "a あ",
		Kind:     AliasVCV,
		Entry: oto.Entry{
			Filename:     full,
			OtoPath:      filepath.Join("bank", "oto.ini"),
			Offset:       anchor - 100,
			Preutterance: 100,
		},
		TargetScore: score,
	}
}
