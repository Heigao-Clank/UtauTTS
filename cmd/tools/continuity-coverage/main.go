package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"utautts/internal/evaluation"
	"utautts/internal/frontend"
	"utautts/internal/voicebank"
)

type caseReport struct {
	ID    string                     `json:"id"`
	Text  string                     `json:"text"`
	Audit *voicebank.ContinuityAudit `json:"audit"`
}

type pathAggregate struct {
	ContinuousBoundaries int     `json:"continuous_boundaries"`
	BoundaryCoverage     float64 `json:"boundary_coverage"`
	SafeBoundaries       int     `json:"safe_boundaries"`
	SafeCoverage         float64 `json:"safe_coverage"`
	Runs2                int     `json:"runs_2"`
	Runs3                int     `json:"runs_3"`
	Runs4Plus            int     `json:"runs_4_plus"`
}

type aggregate struct {
	Utterances int           `json:"utterances"`
	Positions  int           `json:"positions"`
	Boundaries int           `json:"boundaries"`
	Current    pathAggregate `json:"current"`
	Maximum    pathAggregate `json:"maximum"`
}

type report struct {
	Version      int          `json:"version"`
	Voicebank    string       `json:"voicebank"`
	Root         string       `json:"root"`
	Corpus       string       `json:"corpus"`
	Tone         string       `json:"tone"`
	Color        string       `json:"color,omitempty"`
	AliasPolicy  string       `json:"alias_policy"`
	TargetMoraMS float64      `json:"target_mora_ms"`
	SafeRatio    float64      `json:"safe_ratio"`
	Aggregate    aggregate    `json:"aggregate"`
	Cases        []caseReport `json:"cases"`
	Failures     []string     `json:"failures,omitempty"`
}

func main() {
	var voicebankPath, corpusPath, outputPath, tone, color, aliasPolicy string
	var targetMoraMS, safeRatio float64
	flag.StringVar(&voicebankPath, "voicebank", "", "path to a UTAU voicebank directory")
	flag.StringVar(&corpusPath, "corpus", "", "versioned evaluation corpus JSON")
	flag.StringVar(&outputPath, "out", "", "output coverage report JSON")
	flag.StringVar(&tone, "tone", "C4", "voicebank tone used with prefix.map")
	flag.StringVar(&color, "color", "", "voicebank subbank/color")
	flag.StringVar(&aliasPolicy, "alias-policy", "auto", "alias policy: auto, cv-only, vcv-prefer or cvvc-prefer")
	flag.Float64Var(&targetMoraMS, "mora-ms", 140, "reference target duration per mora")
	flag.Float64Var(&safeRatio, "safe-ratio", 4, "tentative maximum source-to-target compression ratio")
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
		Tone: tone, Color: color, AliasPolicy: aliasPolicy,
		TargetMoraMS: targetMoraMS, SafeRatio: safeRatio,
	}
	cfg := voicebank.ResolveConfig{Tone: tone, Color: color, AliasPolicy: voicebank.AliasPolicy(aliasPolicy)}
	for _, item := range corpus.Cases {
		reading := item.Reading
		if reading == "" {
			reading, err = frontend.ToKana(item.Text)
		}
		if err != nil {
			result.Failures = append(result.Failures, fmt.Sprintf("%s: reading: %v", item.ID, err))
			continue
		}
		morae, parseErr := frontend.ParseKana(reading)
		if parseErr != nil {
			result.Failures = append(result.Failures, fmt.Sprintf("%s: morae: %v", item.ID, parseErr))
			continue
		}
		audit, auditErr := bank.AuditContinuity(morae, cfg, targetMoraMS, safeRatio)
		if auditErr != nil {
			result.Failures = append(result.Failures, fmt.Sprintf("%s: continuity: %v", item.ID, auditErr))
			continue
		}
		result.Cases = append(result.Cases, caseReport{ID: item.ID, Text: item.Text, Audit: audit})
		addAudit(&result.Aggregate, audit)
		fmt.Printf("%s boundaries=%d current=%d (%.1f%%) maximum=%d (%.1f%%)\n",
			item.ID, audit.Boundaries,
			audit.Current.ContinuousBoundaries, audit.Current.BoundaryCoverage*100,
			audit.Maximum.ContinuousBoundaries, audit.Maximum.BoundaryCoverage*100)
	}
	if result.Aggregate.Utterances == 0 {
		log.Fatal("no corpus case could be audited")
	}
	finishAggregate(&result.Aggregate)
	if err := writeJSON(outputPath, result); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("total boundaries=%d current=%d (%.1f%%) maximum=%d (%.1f%%)\n",
		result.Aggregate.Boundaries,
		result.Aggregate.Current.ContinuousBoundaries, result.Aggregate.Current.BoundaryCoverage*100,
		result.Aggregate.Maximum.ContinuousBoundaries, result.Aggregate.Maximum.BoundaryCoverage*100)
}

func addAudit(total *aggregate, audit *voicebank.ContinuityAudit) {
	total.Utterances++
	total.Positions += audit.Positions
	total.Boundaries += audit.Boundaries
	addPath(&total.Current, audit.Current)
	addPath(&total.Maximum, audit.Maximum)
}

func addPath(total *pathAggregate, audit voicebank.ContinuityPathAudit) {
	total.ContinuousBoundaries += audit.ContinuousBoundaries
	total.SafeBoundaries += audit.SafeBoundaries
	total.Runs2 += audit.Runs2
	total.Runs3 += audit.Runs3
	total.Runs4Plus += audit.Runs4Plus
}

func finishAggregate(total *aggregate) {
	if total.Boundaries == 0 {
		return
	}
	total.Current.BoundaryCoverage = float64(total.Current.ContinuousBoundaries) / float64(total.Boundaries)
	total.Current.SafeCoverage = float64(total.Current.SafeBoundaries) / float64(total.Boundaries)
	total.Maximum.BoundaryCoverage = float64(total.Maximum.ContinuousBoundaries) / float64(total.Boundaries)
	total.Maximum.SafeCoverage = float64(total.Maximum.SafeBoundaries) / float64(total.Boundaries)
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
