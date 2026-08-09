package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

type pathList []string

func (paths *pathList) String() string         { return strings.Join(*paths, ",") }
func (paths *pathList) Set(value string) error { *paths = append(*paths, value); return nil }

type systemInfo struct {
	Renderer  string `json:"renderer"`
	JoinModel bool   `json:"join_model"`
}
type answerTrial struct {
	ID      int        `json:"id"`
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
	Version      int            `json:"version"`
	Mode         string         `json:"mode"`
	Participants int            `json:"participants"`
	Answered     int            `json:"answered"`
	Missing      int            `json:"missing"`
	Preferences  map[string]int `json:"preferences,omitempty"`
	Ties         int            `json:"ties,omitempty"`
	Correct      int            `json:"correct,omitempty"`
	Incorrect    int            `json:"incorrect,omitempty"`
	Unsure       int            `json:"unsure,omitempty"`
	Accuracy     float64        `json:"accuracy,omitempty"`
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
	report := scoreReport{Version: 1, Mode: key.Mode, Participants: len(responses), Preferences: map[string]int{}}
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
				switch answer.Answer {
				case "unsure":
					report.Unsure++
				case trial.XAnswer:
					report.Correct++
				default:
					report.Incorrect++
				}
				continue
			}
			switch answer.Answer {
			case "tie":
				report.Ties++
			case "A":
				report.Preferences[systemName(trial.A)]++
			case "B":
				report.Preferences[systemName(trial.B)]++
			}
		}
		report.Missing += len(key.Trials) - len(seen)
	}
	if denominator := report.Correct + report.Incorrect; denominator > 0 {
		report.Accuracy = float64(report.Correct) / float64(denominator)
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
	}
	return name
}

func readJSON(path string, value any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, value)
}
