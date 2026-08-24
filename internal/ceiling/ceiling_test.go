package ceiling

import (
	"math"
	"path/filepath"
	"testing"

	"utautts/internal/audio"
	"utautts/internal/oto"
	"utautts/internal/voicebank"
)

func TestDiscoverFindsOrderedVCVSequence(t *testing.T) {
	bank := &voicebank.Bank{Entries: map[string][]oto.Entry{
		"a あ": {{Alias: "a あ", Filename: "recording.wav", OtoPath: "oto.ini", Line: 1, Offset: 0, Preutterance: 40}},
		"a い": {{Alias: "a い", Filename: "recording.wav", OtoPath: "oto.ini", Line: 2, Offset: 200, Preutterance: 40}},
		"a う": {{Alias: "a う", Filename: "recording.wav", OtoPath: "oto.ini", Line: 3, Offset: 400, Preutterance: 40}},
		"え":   {{Alias: "え", Filename: "recording.wav", OtoPath: "oto.ini", Line: 4, Offset: 600, Preutterance: 40}},
	}}
	sequences := Discover(bank, 3, 8)
	if len(sequences) != 1 || len(sequences[0].Entries) != 3 {
		t.Fatalf("sequences = %+v", sequences)
	}
	for index, entry := range sequences[0].Entries {
		if entry.Line != index+1 {
			t.Fatalf("entry %d line = %d", index, entry.Line)
		}
	}
}

func TestGeneratePreservesTransitionsAndSeparatesConditions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "continuous.wav")
	data := make([]int16, 600)
	for index := range data {
		data[index] = int16(12000 * math.Sin(2*math.Pi*float64(index)/50))
	}
	if err := audio.WriteWav(path, &audio.PCM{SampleRate: 1000, Channels: 1, Data: data}); err != nil {
		t.Fatal(err)
	}
	entries := []oto.Entry{
		{Alias: "a あ", Filename: path, OtoPath: "oto.ini", Line: 1, Offset: 0, Fixed: 80, Blank: -200, Preutterance: 40, Overlap: 20},
		{Alias: "a い", Filename: path, OtoPath: "oto.ini", Line: 2, Offset: 200, Fixed: 80, Blank: -200, Preutterance: 40, Overlap: 20},
		{Alias: "a う", Filename: path, OtoPath: "oto.ini", Line: 3, Offset: 400, Fixed: 80, Blank: -200, Preutterance: 40, Overlap: 20},
	}
	result, err := Generate(Sequence{Source: path, OtoPath: "oto.ini", Entries: entries}, Config{MoraDurationMS: 100, MinimumVowelMS: 20})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Original.Data) != 600 || len(result.ReconstructedOriginal.Data) != 600 {
		t.Fatalf("original lengths = %d, %d", len(result.Original.Data), len(result.ReconstructedOriginal.Data))
	}
	if len(result.Current.Data) != 340 || len(result.Anchored.Data) != 300 || len(result.ContinuousAnchored.Data) != 300 {
		t.Fatalf("short lengths = %d, %d, %d", len(result.Current.Data), len(result.Anchored.Data), len(result.ContinuousAnchored.Data))
	}
	if len(result.Intervals) != 3 || result.Intervals[0].SourcePrefixMS != 40 || result.Intervals[0].SourceSuffixMS != 20 {
		t.Fatalf("intervals = %+v", result.Intervals)
	}
	if result.Intervals[0].TargetPrefixMS != 53 || result.Intervals[0].TargetSuffixMS != 27 || result.Intervals[0].TargetStableVowelMS != 20 {
		t.Fatalf("target allocation = %+v", result.Intervals[0])
	}
}

func TestAnchoredRetimeDoesNotInsertSilentVowelWithoutSource(t *testing.T) {
	values := make([]float64, 200)
	for index := range values {
		values[index] = 0.25
	}
	entries := []oto.Entry{
		{Alias: "a あ", Fixed: 140, Preutterance: 40},
		{Alias: "a い", Fixed: 140, Preutterance: 40, Overlap: -20},
	}
	result, intervals := anchoredRetime(values, entries, []int{40, 140}, Config{MoraDurationMS: 100, MinimumVowelMS: 25}, 1000)
	if len(result) != 200 {
		t.Fatalf("frames = %d, want 200", len(result))
	}
	if intervals[0].SourceStableVowelMS != 0 || intervals[0].TargetStableVowelMS != 0 {
		t.Fatalf("unexpected vowel allocation: %+v", intervals[0])
	}
	for index, value := range result {
		if value == 0 {
			t.Fatalf("silence inserted at %d", index)
		}
	}
}
