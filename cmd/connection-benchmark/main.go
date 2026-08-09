package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"utautts/internal/evaluation"
	"utautts/internal/plan"
	"utautts/internal/tts"
	"utautts/internal/voicebank"
)

type textList []string

func (values *textList) String() string         { return strings.Join(*values, " | ") }
func (values *textList) Set(value string) error { *values = append(*values, value); return nil }

type caseReport struct {
	Text         string             `json:"text"`
	Units        int                `json:"units"`
	ChangedUnits int                `json:"changed_units"`
	Handcrafted  *evaluation.Report `json:"handcrafted"`
	Learned      *evaluation.Report `json:"learned"`
}

type aggregate struct {
	Utterances            int     `json:"utterances"`
	ChangedUtterances     int     `json:"changed_utterances"`
	Units                 int     `json:"units"`
	ChangedUnits          int     `json:"changed_units"`
	ConnectedBoundaries   int     `json:"connected_boundaries"`
	HandcraftedMeanClick  float64 `json:"handcrafted_mean_click"`
	LearnedMeanClick      float64 `json:"learned_mean_click"`
	HandcraftedSpectrumDB float64 `json:"handcrafted_mean_spectrum_delta_db"`
	LearnedSpectrumDB     float64 `json:"learned_mean_spectrum_delta_db"`
	HandcraftedRMSDB      float64 `json:"handcrafted_mean_rms_delta_db"`
	LearnedRMSDB          float64 `json:"learned_mean_rms_delta_db"`
}

type benchmarkReport struct {
	Version   int             `json:"version"`
	Voicebank string          `json:"voicebank"`
	JoinModel string          `json:"join_model"`
	JoinScale float64         `json:"join_scale"`
	Aggregate aggregate       `json:"aggregate"`
	Cases     []caseReport    `json:"cases"`
	Failures  []failureReport `json:"failures,omitempty"`
}

type failureReport struct {
	Text  string `json:"text"`
	Stage string `json:"stage"`
	Error string `json:"error"`
}

func main() {
	var texts textList
	var cfg tts.Config
	var outputPath string
	flag.StringVar(&cfg.VoicebankPath, "voicebank", "", "path to a UTAU voicebank directory")
	flag.Var(&texts, "text", "Japanese text to benchmark (repeatable)")
	flag.StringVar(&cfg.JoinModelPath, "join-model", "", "learned join-cost model JSON")
	flag.Float64Var(&cfg.JoinScoreScale, "join-scale", 0, "learned logit score scale")
	flag.StringVar(&outputPath, "out", "", "output benchmark JSON")
	flag.StringVar(&cfg.Tone, "tone", "C4", "voicebank tone")
	flag.StringVar(&cfg.Renderer, "renderer", "waveform", "renderer backend")
	flag.Float64Var(&cfg.MoraDurationMS, "mora-ms", 140, "base mora duration")
	flag.Float64Var(&cfg.PauseDurationMS, "pause-ms", 180, "pause duration")
	flag.Float64Var(&cfg.ReleaseMS, "release-ms", 20, "release envelope")
	flag.Parse()
	if cfg.VoicebankPath == "" || cfg.JoinModelPath == "" || outputPath == "" || len(texts) == 0 {
		flag.Usage()
		log.Fatal("--voicebank, --join-model, --out, and at least one --text are required")
	}
	report := benchmarkReport{Version: 1, Voicebank: cfg.VoicebankPath, JoinModel: cfg.JoinModelPath, JoinScale: cfg.JoinScoreScale}
	for _, text := range texts {
		cfg.Text = text
		cfg.SelectionMode, cfg.JoinModelPath = voicebank.SelectionViterbi, ""
		handcrafted, err := tts.Synthesize(cfg)
		if err != nil {
			report.Failures = append(report.Failures, failureReport{Text: text, Stage: "handcrafted", Error: err.Error()})
			fmt.Printf("skipped %s: %v\n", text, err)
			continue
		}
		handMetrics, err := evaluation.Analyze(handcrafted.Audio, handcrafted.Plan)
		if err != nil {
			report.Failures = append(report.Failures, failureReport{Text: text, Stage: "handcrafted-evaluation", Error: err.Error()})
			continue
		}
		cfg.JoinModelPath = report.JoinModel
		learned, err := tts.Synthesize(cfg)
		if err != nil {
			report.Failures = append(report.Failures, failureReport{Text: text, Stage: "learned", Error: err.Error()})
			continue
		}
		learnedMetrics, err := evaluation.Analyze(learned.Audio, learned.Plan)
		if err != nil {
			report.Failures = append(report.Failures, failureReport{Text: text, Stage: "learned-evaluation", Error: err.Error()})
			continue
		}
		if report.JoinScale <= 0 {
			report.JoinScale = learned.Plan.JoinScoreScale
		}
		changed := selectionDifferences(handcrafted.Plan, learned.Plan)
		report.Cases = append(report.Cases, caseReport{
			Text: text, Units: len(learned.Plan.Units), ChangedUnits: changed,
			Handcrafted: handMetrics, Learned: learnedMetrics,
		})
		accumulate(&report.Aggregate, len(learned.Plan.Units), changed, handMetrics, learnedMetrics)
		fmt.Printf("changed=%d click %.5f -> %.5f spectrum %.3f -> %.3f %s\n",
			changed, handMetrics.MeanClick, learnedMetrics.MeanClick,
			handMetrics.MeanSpectrumDB, learnedMetrics.MeanSpectrumDB, text)
	}
	finishAggregate(&report.Aggregate)
	if len(report.Cases) == 0 {
		log.Fatal("no utterance could be evaluated")
	}
	if err := writeJSON(outputPath, report); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("aggregate utterances=%d changed=%d/%d click %.5f -> %.5f spectrum %.3f -> %.3f\n",
		report.Aggregate.Utterances, report.Aggregate.ChangedUnits, report.Aggregate.Units,
		report.Aggregate.HandcraftedMeanClick, report.Aggregate.LearnedMeanClick,
		report.Aggregate.HandcraftedSpectrumDB, report.Aggregate.LearnedSpectrumDB)
}

func accumulate(total *aggregate, units, changed int, handcrafted, learned *evaluation.Report) {
	total.Utterances++
	total.Units += units
	total.ChangedUnits += changed
	if changed > 0 {
		total.ChangedUtterances++
	}
	weight := float64(min(handcrafted.ConnectedCount, learned.ConnectedCount))
	total.ConnectedBoundaries += int(weight)
	total.HandcraftedMeanClick += handcrafted.MeanClick * weight
	total.LearnedMeanClick += learned.MeanClick * weight
	total.HandcraftedSpectrumDB += handcrafted.MeanSpectrumDB * weight
	total.LearnedSpectrumDB += learned.MeanSpectrumDB * weight
	total.HandcraftedRMSDB += handcrafted.MeanRMSDeltaDB * weight
	total.LearnedRMSDB += learned.MeanRMSDeltaDB * weight
}

func finishAggregate(total *aggregate) {
	if total.ConnectedBoundaries == 0 {
		return
	}
	denominator := float64(total.ConnectedBoundaries)
	total.HandcraftedMeanClick /= denominator
	total.LearnedMeanClick /= denominator
	total.HandcraftedSpectrumDB /= denominator
	total.LearnedSpectrumDB /= denominator
	total.HandcraftedRMSDB /= denominator
	total.LearnedRMSDB /= denominator
}

func selectionDifferences(left, right *plan.Plan) int {
	length, count := min(len(left.Units), len(right.Units)), 0
	for index := 0; index < length; index++ {
		if left.Units[index].Source != right.Units[index].Source || left.Units[index].OtoLine != right.Units[index].OtoLine || left.Units[index].Alias != right.Units[index].Alias {
			count++
		}
	}
	return count + max(len(left.Units), len(right.Units)) - length
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
