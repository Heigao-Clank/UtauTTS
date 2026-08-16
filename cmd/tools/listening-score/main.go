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
)

type pathList []string

func (paths *pathList) String() string         { return strings.Join(*paths, ",") }
func (paths *pathList) Set(value string) error { *paths = append(*paths, value); return nil }

type systemInfo struct {
	Renderer         string `json:"renderer"`
	JoinModel        bool   `json:"join_model"`
	JoinModelPath    string `json:"join_model_path,omitempty"`
	ProsodyModel     bool   `json:"prosody_model,omitempty"`
	ProsodyPath      string `json:"prosody_model_path,omitempty"`
	ProsodyPitchOnly bool   `json:"prosody_pitch_only,omitempty"`
	PitchContourPath string `json:"pitch_contour_path,omitempty"`
}
type answerTrial struct {
	ID      int        `json:"id"`
	CaseID  string     `json:"case_id,omitempty"`
	Text    string     `json:"text,omitempty"`
	A       systemInfo `json:"a"`
	B       systemInfo `json:"b"`
	XAnswer string     `json:"x_answer"`
}
type answerKey struct {
	Version int           `json:"version"`
	Mode    string        `json:"mode"`
	Trials  []answerTrial `json:"trials"`
}
type response struct {
	Version int              `json:"version"`
	Mode    string           `json:"mode"`
	Answers []responseAnswer `json:"answers"`
}
type responseAnswer struct {
	ID     int    `json:"id"`
	Answer string `json:"answer"`
}

type scoreReport struct {
	Version         int                 `json:"version"`
	Mode            string              `json:"mode"`
	Participants    int                 `json:"participants"`
	Answered        int                 `json:"answered"`
	Missing         int                 `json:"missing"`
	Preferences     map[string]int      `json:"preferences,omitempty"`
	Ties            int                 `json:"ties,omitempty"`
	Correct         int                 `json:"correct,omitempty"`
	Incorrect       int                 `json:"incorrect,omitempty"`
	Unsure          int                 `json:"unsure,omitempty"`
	Accuracy        float64             `json:"accuracy,omitempty"`
	AccuracyCI95    *interval           `json:"accuracy_ci95,omitempty"`
	PreferenceRates map[string]float64  `json:"preference_rates,omitempty"`
	PreferenceCI95  map[string]interval `json:"preference_ci95,omitempty"`
	Cases           []caseResult        `json:"cases"`
}

type caseResult struct {
	TrialID    int    `json:"trial_id"`
	CaseID     string `json:"case_id,omitempty"`
	Text       string `json:"text,omitempty"`
	Answer     string `json:"answer"`
	Preference string `json:"preference"`
}

type interval struct {
	Low  float64 `json:"low"`
	High float64 `json:"high"`
}

func main() {
	var keyPath, outputPath string
	var responses pathList
	flag.StringVar(&keyPath, "key", "", "private answer-key.json")
	flag.Var(&responses, "response", "listening-results.json (repeatable)")
	flag.StringVar(&outputPath, "out", "", "optional score JSON")
	flag.Parse()
	if keyPath == "" || len(responses) == 0 {
		flag.Usage()
		log.Fatal("--key and at least one --response are required")
	}
	var key answerKey
	if err := readJSON(keyPath, &key); err != nil {
		log.Fatal(err)
	}
	lookup := map[int]answerTrial{}
	for _, trial := range key.Trials {
		lookup[trial.ID] = trial
	}
	report := scoreReport{Version: 2, Mode: key.Mode, Participants: len(responses), Preferences: map[string]int{}}
	if key.Mode == "ab" {
		for _, trial := range key.Trials {
			report.Preferences[systemName(trial.A)] += 0
			report.Preferences[systemName(trial.B)] += 0
		}
	}
	for _, path := range responses {
		var submitted response
		if err := readJSON(path, &submitted); err != nil {
			log.Fatal(err)
		}
		if submitted.Mode != key.Mode {
			log.Fatalf("%s mode %q does not match key mode %q", path, submitted.Mode, key.Mode)
		}
		seen := map[int]bool{}
		for _, answer := range submitted.Answers {
			trial, ok := lookup[answer.ID]
			if !ok || seen[answer.ID] {
				continue
			}
			seen[answer.ID] = true
			if answer.Answer == "" {
				continue
			}
			report.Answered++
			if key.Mode == "abx" {
				outcome := "incorrect"
				switch answer.Answer {
				case "unsure":
					report.Unsure++
					outcome = "unsure"
				case trial.XAnswer:
					report.Correct++
					outcome = "correct"
				default:
					report.Incorrect++
				}
				report.Cases = append(report.Cases, caseResult{TrialID: trial.ID, CaseID: trial.CaseID, Text: trial.Text, Answer: answer.Answer, Preference: outcome})
				continue
			}
			preference := "tie"
			switch answer.Answer {
			case "tie":
				report.Ties++
			case "A":
				preference = systemName(trial.A)
				report.Preferences[preference]++
			case "B":
				preference = systemName(trial.B)
				report.Preferences[preference]++
			}
			report.Cases = append(report.Cases, caseResult{TrialID: trial.ID, CaseID: trial.CaseID, Text: trial.Text, Answer: answer.Answer, Preference: preference})
		}
		report.Missing += len(key.Trials) - len(seen)
	}
	if denominator := report.Correct + report.Incorrect; denominator > 0 {
		report.Accuracy = float64(report.Correct) / float64(denominator)
		value := wilson95(report.Correct, denominator)
		report.AccuracyCI95 = &value
	}
	if key.Mode == "ab" {
		decisive := report.Answered - report.Ties
		if decisive > 0 {
			report.PreferenceRates = map[string]float64{}
			report.PreferenceCI95 = map[string]interval{}
			for name, count := range report.Preferences {
				report.PreferenceRates[name] = float64(count) / float64(decisive)
				report.PreferenceCI95[name] = wilson95(count, decisive)
			}
		}
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	data = append(data, '\n')
	if outputPath != "" {
		if directory := filepath.Dir(outputPath); directory != "." {
			if err := os.MkdirAll(directory, 0o755); err != nil {
				log.Fatal(err)
			}
		}
		if err := os.WriteFile(outputPath, data, 0o644); err != nil {
			log.Fatal(err)
		}
	}
	fmt.Print(string(data))
}

func systemName(system systemInfo) string {
	name := system.Renderer
	if system.JoinModel {
		name += "+learned"
		if system.JoinModelPath != "" {
			name += ":" + filepath.Base(system.JoinModelPath)
		}
	} else {
		name += "+handcrafted"
	}
	if system.ProsodyModel {
		name += "+prosody"
		if system.ProsodyPath != "" {
			name += ":" + filepath.Base(system.ProsodyPath)
		}
		if system.ProsodyPitchOnly {
			name += ":pitch-only"
		}
	}
	if system.PitchContourPath != "" {
		name += "+pitch-contour:" + filepath.Base(system.PitchContourPath)
	}
	return name
}

func wilson95(successes, total int) interval {
	if total <= 0 {
		return interval{}
	}
	z := 1.959963984540054
	n := float64(total)
	p := float64(successes) / n
	denominator := 1 + z*z/n
	center := (p + z*z/(2*n)) / denominator
	margin := z * math.Sqrt(p*(1-p)/n+z*z/(4*n*n)) / denominator
	return interval{Low: math.Max(0, center-margin), High: math.Min(1, center+margin)}
}

func readJSON(path string, value any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, value)
}
