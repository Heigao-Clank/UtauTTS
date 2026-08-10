package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"utautts/internal/audio"
	"utautts/internal/evaluation"
	"utautts/internal/plan"
	"utautts/internal/tts"
	"utautts/internal/voicebank"
)

const reportVersion = 2

type comparisonReport struct {
	Version   int              `json:"version"`
	Text      string           `json:"text,omitempty"`
	Reading   string           `json:"reading,omitempty"`
	Voicebank string           `json:"voicebank"`
	Renderer  string           `json:"renderer"`
	Cases     []comparisonCase `json:"cases"`
}

type comparisonCase struct {
	Name                   string                  `json:"name"`
	Mode                   voicebank.SelectionMode `json:"mode"`
	JoinCostMode           string                  `json:"join_cost_mode"`
	SynthesisElapsedMS     float64                 `json:"synthesis_elapsed_ms"`
	ChangedUnits           int                     `json:"changed_units_from_target_only"`
	ChangedFromHandcrafted int                     `json:"changed_units_from_handcrafted_viterbi"`
	WAVPath                string                  `json:"wav_path"`
	PlanPath               string                  `json:"plan_path"`
	EvaluationPath         string                  `json:"evaluation_path"`
	Evaluation             *evaluation.Report      `json:"evaluation"`
}

func main() {
	var cfg tts.Config
	var outputDirectory, joinModelPath string
	flag.StringVar(&cfg.VoicebankPath, "voicebank", "", "path to a UTAU voicebank directory")
	flag.StringVar(&cfg.Text, "text", "", "Japanese text to synthesize")
	flag.StringVar(&cfg.Reading, "kana", "", "kana reading to synthesize")
	flag.StringVar(&cfg.Tone, "tone", "C4", "voicebank tone used with prefix.map")
	flag.StringVar(&outputDirectory, "out", "", "output directory")
	flag.StringVar(&cfg.Renderer, "renderer", "waveform", "renderer backend")
	flag.Float64Var(&cfg.MoraDurationMS, "mora-ms", 140, "base mora duration in milliseconds")
	flag.Float64Var(&cfg.PauseDurationMS, "pause-ms", 180, "punctuation pause in milliseconds")
	flag.Float64Var(&cfg.ReleaseMS, "release-ms", 20, "unit release envelope in milliseconds")
	flag.StringVar(&cfg.ProsodyModelPath, "prosody", "", "optional learned prosody model JSON")
	flag.BoolVar(&cfg.ApplyPitch, "apply-pitch", false, "experimental waveform pitch resampling")
	flag.Float64Var(&cfg.IntonationStrength, "intonation-strength", 0, "intonation strength (0..1)")
	flag.StringVar(&cfg.WorldlinePath, "worldline", "", "path to worldline library")
	flag.StringVar(&cfg.WorldlineBridgePath, "worldline-bridge", "", "path to worldline bridge")
	flag.StringVar(&joinModelPath, "join-model", "", "optional learned join-cost model JSON")
	flag.Float64Var(&cfg.JoinScoreScale, "join-scale", 0, "learned logit score scale (default: model or 4)")
	flag.Parse()
	if cfg.VoicebankPath == "" || (cfg.Text == "" && cfg.Reading == "") || outputDirectory == "" {
		flag.Usage()
		log.Fatal("--voicebank, --out, and either --text or --kana are required")
	}
	if err := os.MkdirAll(outputDirectory, 0o755); err != nil {
		log.Fatal(err)
	}

	type comparisonSpec struct {
		name, joinModelPath string
		mode                voicebank.SelectionMode
	}
	specs := []comparisonSpec{
		{name: "target-only", mode: voicebank.SelectionTargetOnly},
		{name: "greedy", mode: voicebank.SelectionGreedy},
		{name: "handcrafted-viterbi", mode: voicebank.SelectionViterbi},
	}
	if joinModelPath != "" {
		specs = append(specs, comparisonSpec{name: "learned-viterbi", mode: voicebank.SelectionViterbi, joinModelPath: joinModelPath})
	}
	report := comparisonReport{
		Version: reportVersion, Text: cfg.Text, Reading: cfg.Reading,
		Voicebank: cfg.VoicebankPath, Renderer: cfg.Renderer,
	}
	cfg.SelectionMode = voicebank.SelectionTargetOnly
	if _, err := tts.Synthesize(cfg); err != nil {
		log.Fatalf("warm-up synthesis: %v", err)
	}
	var baseline, handcrafted *plan.Plan
	for _, spec := range specs {
		cfg.SelectionMode = spec.mode
		cfg.JoinModelPath = spec.joinModelPath
		started := time.Now()
		result, err := tts.Synthesize(cfg)
		if err != nil {
			log.Fatalf("%s synthesis: %v", spec.name, err)
		}
		elapsedMS := float64(time.Since(started).Microseconds()) / 1000
		wavPath := filepath.Join(outputDirectory, spec.name+".wav")
		planPath := filepath.Join(outputDirectory, spec.name+"-plan.json")
		evaluationPath := filepath.Join(outputDirectory, spec.name+"-evaluation.json")
		if err := audio.WriteWav(wavPath, result.Audio); err != nil {
			log.Fatal(err)
		}
		if err := writeJSON(planPath, result.Plan); err != nil {
			log.Fatal(err)
		}
		metrics, err := evaluation.Analyze(result.Audio, result.Plan)
		if err != nil {
			log.Fatalf("%s evaluation: %v", spec.name, err)
		}
		if err := writeJSON(evaluationPath, metrics); err != nil {
			log.Fatal(err)
		}
		if baseline == nil {
			baseline = result.Plan
		}
		if spec.name == "handcrafted-viterbi" {
			handcrafted = result.Plan
		}
		caseReport := comparisonCase{
			Name: spec.name, Mode: spec.mode, JoinCostMode: result.Plan.JoinCostMode, SynthesisElapsedMS: elapsedMS,
			ChangedUnits:           countSelectionDifferences(baseline, result.Plan),
			ChangedFromHandcrafted: countSelectionDifferences(handcrafted, result.Plan),
			WAVPath:                filepath.Base(wavPath), PlanPath: filepath.Base(planPath),
			EvaluationPath: filepath.Base(evaluationPath), Evaluation: metrics,
		}
		report.Cases = append(report.Cases, caseReport)
		fmt.Printf("%-21s target-changed=%d handcrafted-changed=%d click=%.5f spectrum=%.3fdB elapsed=%.1fms\n",
			spec.name, caseReport.ChangedUnits, caseReport.ChangedFromHandcrafted, metrics.MeanClick, metrics.MeanSpectrumDB, elapsedMS)
	}
	if err := writeJSON(filepath.Join(outputDirectory, "comparison.json"), report); err != nil {
		log.Fatal(err)
	}
}

func countSelectionDifferences(baseline, candidate *plan.Plan) int {
	if baseline == nil || candidate == nil {
		return 0
	}
	count := 0
	length := min(len(baseline.Units), len(candidate.Units))
	for index := 0; index < length; index++ {
		left, right := baseline.Units[index], candidate.Units[index]
		if left.Source != right.Source || left.OtoLine != right.OtoLine || left.Alias != right.Alias {
			count++
		}
	}
	if len(baseline.Units) > length {
		count += len(baseline.Units) - length
	}
	if len(candidate.Units) > length {
		count += len(candidate.Units) - length
	}
	return count
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}
