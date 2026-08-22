package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"strings"

	"utautts/internal/evaluation"
	"utautts/internal/frontend"
	"utautts/internal/prosody"
)

type durationCorpus struct {
	Version int            `json:"version"`
	Name    string         `json:"name"`
	Source  string         `json:"source"`
	Cases   []durationCase `json:"cases"`
}

type durationCase struct {
	ID              string    `json:"id"`
	MoraDurationsMS []float64 `json:"mora_durations_ms"`
	PauseDurationMS float64   `json:"pause_duration_ms,omitempty"`
}

type pitchCorpus struct {
	Version int         `json:"version"`
	Name    string      `json:"name"`
	Source  string      `json:"source"`
	Cases   []pitchCase `json:"cases"`
}

type pitchCase struct {
	ID           string    `json:"id"`
	PitchFactors []float64 `json:"pitch_factors"`
}

type referenceManifest struct {
	Version int             `json:"version"`
	Source  string          `json:"source"`
	Cases   []referenceCase `json:"cases"`
	Skipped []string        `json:"skipped,omitempty"`
}

type referenceCase struct {
	ID         string  `json:"id"`
	Text       string  `json:"text"`
	Reading    string  `json:"reading"`
	AudioPath  string  `json:"audio_path"`
	StartMS    float64 `json:"start_ms"`
	EndMS      float64 `json:"end_ms"`
	MedianF0Hz float64 `json:"median_f0_hz"`
}

func main() {
	var inputPath, outputDirectory, name string
	var offset, count int
	flag.StringVar(&inputPath, "input", "", "aligned prosody JSONL")
	flag.StringVar(&outputDirectory, "out", "", "output directory")
	flag.StringVar(&name, "name", "prosody-ceiling-v1", "evaluation corpus name")
	flag.IntVar(&offset, "offset", 0, "number of valid records to skip")
	flag.IntVar(&count, "count", 12, "number of valid records to export")
	flag.Parse()
	if inputPath == "" || outputDirectory == "" || count < 1 || offset < 0 {
		flag.Usage()
		log.Fatal("--input, --out and a positive --count are required")
	}
	records, err := prosody.LoadJSONL(inputPath)
	if err != nil {
		log.Fatal(err)
	}
	corpus, durations, pitches, references := buildAssets(records, name, inputPath, offset, count)
	if len(corpus.Cases) < count {
		log.Fatalf("only %d aligned records were available after offset %d; requested %d", len(corpus.Cases), offset, count)
	}
	if err := os.MkdirAll(outputDirectory, 0o755); err != nil {
		log.Fatal(err)
	}
	for filename, value := range map[string]any{
		"corpus.json":             corpus,
		"mora-durations.json":     durations,
		"mora-pitch-factors.json": pitches,
		"references.json":         references,
	} {
		if err := writeJSON(filepath.Join(outputDirectory, filename), value); err != nil {
			log.Fatal(err)
		}
	}
	fmt.Printf("wrote %d aligned ceiling cases to %s (%d skipped)\n", len(corpus.Cases), outputDirectory, len(references.Skipped))
}

func buildAssets(records []prosody.Record, name, source string, offset, count int) (*evaluation.Corpus, durationCorpus, pitchCorpus, referenceManifest) {
	corpus := &evaluation.Corpus{Version: 1, Name: name}
	durations := durationCorpus{Version: 1, Name: name + "-reference-duration", Source: source}
	pitches := pitchCorpus{Version: 1, Name: name + "-reference-mora-f0", Source: source}
	references := referenceManifest{Version: 1, Source: source}
	validSeen := 0
	for _, record := range records {
		morae, reason := alignedMorae(record)
		if reason != "" {
			references.Skipped = append(references.Skipped, record.ID+": "+reason)
			continue
		}
		if validSeen < offset {
			validSeen++
			continue
		}
		if len(corpus.Cases) >= count {
			break
		}
		durationValues := make([]float64, len(morae))
		pitchValues := make([]float64, len(morae))
		pauseDuration := 0.0
		for index, token := range record.Tokens {
			durationValues[index] = token.DurationMS
			pitchValues[index] = token.PitchRatio
			if token.Pause {
				pitchValues[index] = 1
				if token.DurationMS > 0 {
					pauseDuration = token.DurationMS
				}
			} else if pitchValues[index] <= 0 || math.IsNaN(pitchValues[index]) || math.IsInf(pitchValues[index], 0) {
				pitchValues[index] = nearestPitchRatio(record.Tokens, index)
			}
		}
		corpus.Cases = append(corpus.Cases, evaluation.CorpusCase{
			ID: record.ID, Text: record.Text, Reading: record.Reading,
			Tags: []string{"reference-duration", "reference-mora-f0"},
		})
		durations.Cases = append(durations.Cases, durationCase{ID: record.ID, MoraDurationsMS: durationValues, PauseDurationMS: pauseDuration})
		pitches.Cases = append(pitches.Cases, pitchCase{ID: record.ID, PitchFactors: pitchValues})
		references.Cases = append(references.Cases, referenceCase{
			ID: record.ID, Text: record.Text, Reading: record.Reading, AudioPath: record.AudioPath,
			StartMS: record.StartMS, EndMS: record.EndMS, MedianF0Hz: record.MedianF0Hz,
		})
	}
	return corpus, durations, pitches, references
}

func alignedMorae(record prosody.Record) ([]frontend.Mora, string) {
	if strings.TrimSpace(record.ID) == "" || strings.TrimSpace(record.Text) == "" || strings.TrimSpace(record.Reading) == "" {
		return nil, "missing id, text, or reading"
	}
	morae, err := frontend.ParseKana(record.Reading)
	if err != nil {
		return nil, err.Error()
	}
	if len(morae) != len(record.Tokens) {
		return nil, fmt.Sprintf("mora/token length mismatch %d/%d", len(morae), len(record.Tokens))
	}
	for index := range morae {
		token := record.Tokens[index]
		if token.DurationMS <= 0 || math.IsNaN(token.DurationMS) || math.IsInf(token.DurationMS, 0) {
			return nil, fmt.Sprintf("invalid duration at position %d", index)
		}
		if morae[index].Pause != token.Pause {
			return nil, fmt.Sprintf("pause mismatch at position %d", index)
		}
		if !token.Pause && normalizeMora(morae[index].Text) != normalizeMora(token.Mora) {
			return nil, fmt.Sprintf("mora mismatch at position %d: %s/%s", index, morae[index].Text, token.Mora)
		}
	}
	return morae, ""
}

func normalizeMora(value string) string {
	runes := []rune(strings.TrimSpace(value))
	for index, value := range runes {
		if value >= 'ァ' && value <= 'ヶ' {
			runes[index] = value - ('ァ' - 'ぁ')
		}
	}
	return string(runes)
}

func nearestPitchRatio(tokens []prosody.Target, index int) float64 {
	for distance := 1; distance < len(tokens); distance++ {
		for _, candidate := range []int{index - distance, index + distance} {
			if candidate >= 0 && candidate < len(tokens) && !tokens[candidate].Pause && tokens[candidate].PitchRatio > 0 {
				return tokens[candidate].PitchRatio
			}
		}
	}
	return 1
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}
