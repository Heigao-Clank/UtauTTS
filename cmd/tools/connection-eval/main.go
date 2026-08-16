package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"

	"utautts/internal/audio"
	"utautts/internal/evaluation"
	"utautts/internal/plan"
)

func main() {
	var wavPath, planPath, outputPath string
	flag.StringVar(&wavPath, "wav", "", "synthesized WAV to evaluate")
	flag.StringVar(&planPath, "plan", "", "synthesis plan JSON used for the WAV")
	flag.StringVar(&outputPath, "out", "", "optional report JSON (default: stdout)")
	flag.Parse()
	if wavPath == "" || planPath == "" {
		flag.Usage()
		log.Fatal("--wav and --plan are required")
	}
	pcm, err := audio.ReadWav(wavPath)
	if err != nil {
		log.Fatal(err)
	}
	data, err := os.ReadFile(planPath)
	if err != nil {
		log.Fatal(err)
	}
	var synthesisPlan plan.Plan
	if err := json.Unmarshal(data, &synthesisPlan); err != nil {
		log.Fatal(err)
	}
	report, err := evaluation.Analyze(pcm, &synthesisPlan)
	if err != nil {
		log.Fatal(err)
	}
	encoded, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	encoded = append(encoded, '\n')
	if outputPath != "" {
		if err := os.WriteFile(outputPath, encoded, 0o644); err != nil {
			log.Fatal(err)
		}
		fmt.Printf("wrote %s (%d connected boundaries; mean click %.5f; mean peak %.5f)\n", outputPath, report.ConnectedCount, report.MeanClick, report.MeanPeakClick)
		return
	}
	_, _ = os.Stdout.Write(encoded)
}
