package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"

	"utautts/internal/evaluation"
	"utautts/internal/frontend"
	"utautts/internal/voicebank"
)

type caseReport struct {
	VCVSelectedPositions     int     `json:"vcv_selected_positions"`
	CVSelectedPositions      int     `json:"cv_selected_positions"`
	ID                       string  `json:"id"`
	Text                     string  `json:"text"`
	Positions                int     `json:"positions"`
	MultiCandidatePositions  int     `json:"multi_candidate_positions"`
	PitchChoicePositions     int     `json:"pitch_choice_positions"`
	WidePitchPositions       int     `json:"wide_pitch_positions"`
	WideWithinGroupPositions int     `json:"wide_pitch_within_group_positions"`
	CrossGroupPositions      int     `json:"cross_group_positions"`
	MaximumPitchSpanCents    float64 `json:"maximum_pitch_span_cents"`
}

type aggregate struct {
	VCVSelectedPositions     int     `json:"vcv_selected_positions"`
	CVSelectedPositions      int     `json:"cv_selected_positions"`
	Utterances               int     `json:"utterances"`
	Positions                int     `json:"positions"`
	MultiCandidatePositions  int     `json:"multi_candidate_positions"`
	PitchChoicePositions     int     `json:"pitch_choice_positions"`
	WidePitchPositions       int     `json:"wide_pitch_positions"`
	WideWithinGroupPositions int     `json:"wide_pitch_within_group_positions"`
	CrossGroupPositions      int     `json:"cross_group_positions"`
	MaximumPitchSpanCents    float64 `json:"maximum_pitch_span_cents"`
}

type report struct {
	Version        int                         `json:"version"`
	Voicebank      string                      `json:"voicebank"`
	Root           string                      `json:"root"`
	Corpus         string                      `json:"corpus"`
	Tone           string                      `json:"tone"`
	EntryCount     int                         `json:"entry_count"`
	AliasCounts    map[voicebank.AliasKind]int `json:"alias_counts"`
	VCVContexts    map[string]int              `json:"vcv_contexts"`
	HasInitialVCV  bool                        `json:"has_initial_vcv"`
	HasNContextVCV bool                        `json:"has_n_context_vcv"`
	PrefixTones    []string                    `json:"prefix_tones,omitempty"`
	Aggregate      aggregate                   `json:"aggregate"`
	Cases          []caseReport                `json:"cases"`
	Failures       []string                    `json:"failures,omitempty"`
}

func main() {
	var voicebankPath, corpusPath, tone, outputPath string
	flag.StringVar(&voicebankPath, "voicebank", "", "path to a UTAU voicebank directory")
	flag.StringVar(&corpusPath, "corpus", "", "versioned evaluation corpus JSON")
	flag.StringVar(&tone, "tone", "C4", "voicebank tone used with prefix.map")
	flag.StringVar(&outputPath, "out", "", "output capability report JSON")
	flag.Parse()
	if voicebankPath == "" || corpusPath == "" || outputPath == "" {
		flag.Usage()
		log.Fatal("--voicebank, --corpus and --out are required")
	}
	bank, err := voicebank.Load(voicebankPath)
	if err != nil {
		log.Fatal(err)
	}
	corpus, err := evaluation.LoadCorpus(corpusPath)
	if err != nil {
		log.Fatal(err)
	}
	result := report{
		Version: 1, Voicebank: bank.Name, Root: bank.Root, Corpus: corpus.Name,
		Tone: tone, EntryCount: bank.EntryCount(),
	}
	capabilities := bank.AliasCapabilities()
	result.AliasCounts = capabilities.Counts
	result.VCVContexts = capabilities.VCVContexts
	result.HasInitialVCV = capabilities.HasInitialVCV
	result.HasNContextVCV = capabilities.HasNContextVCV
	for prefixTone := range bank.PrefixMap {
		result.PrefixTones = append(result.PrefixTones, prefixTone)
	}
	sort.Strings(result.PrefixTones)
	for _, item := range corpus.Cases {
		reading, err := frontend.ToKana(item.Text)
		if err != nil {
			result.Failures = append(result.Failures, fmt.Sprintf("%s: reading: %v", item.ID, err))
			continue
		}
		morae, err := frontend.ParseKana(reading)
		if err != nil {
			result.Failures = append(result.Failures, fmt.Sprintf("%s: morae: %v", item.ID, err))
			continue
		}
		audit, err := bank.AuditLattice(morae, tone, nil)
		if err != nil {
			result.Failures = append(result.Failures, fmt.Sprintf("%s: lattice: %v", item.ID, err))
			continue
		}
		current := caseReport{
			ID: item.ID, Text: item.Text, Positions: len(audit.Positions),
			MultiCandidatePositions:  audit.MultiCandidatePositions,
			VCVSelectedPositions:     audit.VCVSelectedPositions,
			CVSelectedPositions:      audit.CVSelectedPositions,
			PitchChoicePositions:     audit.PitchChoicePositions,
			WidePitchPositions:       audit.WidePitchPositions,
			WideWithinGroupPositions: audit.WidePitchWithinGroup,
		}
		for _, position := range audit.Positions {
			if position.SourceGroupCount > 1 {
				current.CrossGroupPositions++
			}
			current.MaximumPitchSpanCents = max(current.MaximumPitchSpanCents, position.PitchSpanCents)
		}
		result.Cases = append(result.Cases, current)
		result.Aggregate.Utterances++
		result.Aggregate.Positions += current.Positions
		result.Aggregate.MultiCandidatePositions += current.MultiCandidatePositions
		result.Aggregate.VCVSelectedPositions += current.VCVSelectedPositions
		result.Aggregate.CVSelectedPositions += current.CVSelectedPositions
		result.Aggregate.PitchChoicePositions += current.PitchChoicePositions
		result.Aggregate.WidePitchPositions += current.WidePitchPositions
		result.Aggregate.WideWithinGroupPositions += current.WideWithinGroupPositions
		result.Aggregate.CrossGroupPositions += current.CrossGroupPositions
		result.Aggregate.MaximumPitchSpanCents = max(result.Aggregate.MaximumPitchSpanCents, current.MaximumPitchSpanCents)
		fmt.Printf("%s positions=%d vcv=%d cv=%d multi=%d pitch=%d wide=%d wide-within-group=%d cross-group=%d\n", item.ID, current.Positions, current.VCVSelectedPositions, current.CVSelectedPositions, current.MultiCandidatePositions, current.PitchChoicePositions, current.WidePitchPositions, current.WideWithinGroupPositions, current.CrossGroupPositions)
	}
	if len(result.Cases) == 0 {
		log.Fatal("no corpus case could be audited")
	}
	if err := writeJSON(outputPath, result); err != nil {
		log.Fatal(err)
	}
}

func writeJSON(path string, value any) error {
	if directory := filepath.Dir(path); directory != "." {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			return err
		}
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}
