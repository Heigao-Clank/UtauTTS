package voicebank

import (
	"errors"
	"math"
	"path/filepath"
	"testing"

	"utautts/internal/audio"
	"utautts/internal/frontend"
	"utautts/internal/oto"
)

func TestResolvePrefersVCVAndFallsBackToCV(t *testing.T) {
	bank := &Bank{Entries: map[string][]oto.Entry{
		"- こ": {{Alias: "- こ", Filename: "start.wav"}},
		"o ん": {{Alias: "o ん", Filename: "vcv.wav"}},
		"に":   {{Alias: "に", Filename: "cv.wav"}},
	}}
	morae, err := frontend.ParseKana("こんに")
	if err != nil {
		t.Fatal(err)
	}
	got, err := bank.Resolve(morae)
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Alias != "- こ" || got[1].Alias != "o ん" || got[2].Alias != "に" {
		t.Fatalf("selections = %#v", got)
	}
}

func TestResolveResetsContextAtPause(t *testing.T) {
	bank := &Bank{Entries: map[string][]oto.Entry{
		"- あ": {{Alias: "- あ"}},
		"- い": {{Alias: "- い"}},
	}}
	morae, err := frontend.ParseKana("あ、い")
	if err != nil {
		t.Fatal(err)
	}
	got, err := bank.Resolve(morae)
	if err != nil {
		t.Fatal(err)
	}
	if got[1].Alias != "- い" {
		t.Fatalf("second alias = %q", got[1].Alias)
	}
}

func TestResolveReturnsMissingAlias(t *testing.T) {
	bank := &Bank{Entries: map[string][]oto.Entry{}}
	morae, _ := frontend.ParseKana("あ")
	_, err := bank.Resolve(morae)
	var missing *MissingAliasError
	if !errors.As(err, &missing) || missing.Position != 0 {
		t.Fatalf("error = %v", err)
	}
}

func TestResolveLongMarkUsesPreviousVowel(t *testing.T) {
	bank := &Bank{Entries: map[string][]oto.Entry{
		"す":   {{Alias: "す"}},
		"u う": {{Alias: "u う"}},
	}}
	morae, _ := frontend.ParseKana("スー")
	got, err := bank.Resolve(morae)
	if err != nil {
		t.Fatal(err)
	}
	if got[1].Mora.Text != "ー" || got[1].Alias != "u う" {
		t.Fatalf("long vowel selection = %+v", got[1])
	}
}

func TestResolveScoresDuplicateEntriesByOtoConsistency(t *testing.T) {
	bank := &Bank{Entries: map[string][]oto.Entry{
		"- あ": {
			{Alias: "- あ", Filename: "broken.wav", Fixed: 20, Preutterance: 80, Overlap: 100},
			{Alias: "- あ", Filename: "usable.wav", Fixed: 100, Preutterance: 60, Overlap: 20},
		},
	}}
	morae, _ := frontend.ParseKana("あ")
	got, err := bank.Resolve(morae)
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Entry.Filename != "usable.wav" || got[0].Score == 0 {
		t.Fatalf("selection = %#v", got[0])
	}
}

func TestResolveCanRejectBrokenVCVForCVFallback(t *testing.T) {
	bank := &Bank{Entries: map[string][]oto.Entry{
		"- あ": {{Alias: "- あ", Fixed: 100, Preutterance: 60, Overlap: 20}},
		"a い": {{Alias: "a い", Filename: "broken.wav", Fixed: 0, Preutterance: 200, Overlap: 250}},
		"い":   {{Alias: "い", Filename: "cv.wav", Fixed: 100, Preutterance: 50, Overlap: 10}},
	}}
	morae, _ := frontend.ParseKana("あい")
	got, err := bank.Resolve(morae)
	if err != nil {
		t.Fatal(err)
	}
	if got[1].Alias != "い" {
		t.Fatalf("fallback = %#v", got[1])
	}
}

func TestResolveUsesPhrasePathInsteadOfGreedyDuplicateChoice(t *testing.T) {
	bank := &Bank{Entries: map[string][]oto.Entry{
		"- あ": {
			{Alias: "- あ", Filename: "isolated.wav", Fixed: 100, Preutterance: 60, Overlap: 20},
			{Alias: "- あ", Filename: "continuous.wav", Offset: 10, Fixed: 100, Preutterance: 60, Overlap: 20},
		},
		"a い": {{Alias: "a い", Filename: "continuous.wav", Offset: 200, Fixed: 100, Preutterance: 60, Overlap: 20}},
	}}
	morae, _ := frontend.ParseKana("あい")
	got, err := bank.Resolve(morae)
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Entry.Filename != "continuous.wav" {
		t.Fatalf("path did not retain source continuity: %#v", got)
	}
}

func TestJoinScorePrefersMatchingAcousticBoundary(t *testing.T) {
	dir := t.TempDir()
	previousPath := filepath.Join(dir, "previous.wav")
	matchingPath := filepath.Join(dir, "matching.wav")
	mismatchPath := filepath.Join(dir, "mismatch.wav")
	writeResolverTone(t, previousPath, 200)
	writeResolverTone(t, matchingPath, 200)
	writeResolverTone(t, mismatchPath, 400)
	entry := func(path string) oto.Entry {
		return oto.Entry{Filename: path, Preutterance: 30, Overlap: 10}
	}
	cache := boundaryFeatureCache{}
	matching := joinScore(entry(previousPath), entry(matchingPath), cache)
	mismatch := joinScore(entry(previousPath), entry(mismatchPath), cache)
	if matching <= mismatch {
		t.Fatalf("matching score %.3f <= mismatching score %.3f", matching, mismatch)
	}
}

func writeResolverTone(t *testing.T, path string, hz float64) {
	t.Helper()
	const sampleRate = 16000
	data := make([]int16, sampleRate/3)
	for i := range data {
		data[i] = int16(8000 * math.Sin(2*math.Pi*hz*float64(i)/sampleRate))
	}
	if err := audio.WriteWav(path, &audio.PCM{SampleRate: sampleRate, Channels: 1, Data: data}); err != nil {
		t.Fatal(err)
	}
}
