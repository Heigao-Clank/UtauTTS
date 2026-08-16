package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"utautts/internal/evaluation"
	"utautts/internal/rendererplugin"
	"utautts/internal/tts"
)

type metrics struct {
	Connected  int     `json:"connected_boundaries"`
	MeanClick  float64 `json:"mean_click"`
	MeanPeak   float64 `json:"mean_peak_click"`
	MeanDelta  float64 `json:"mean_delta_rms"`
	MeanRMSDB  float64 `json:"mean_rms_delta_db"`
	MeanSpecDB float64 `json:"mean_spectrum_delta_db"`
	MeanF0     float64 `json:"mean_f0_delta_cents,omitempty"`
}

type caseReport struct {
	ID      string   `json:"id"`
	Text    string   `json:"text"`
	Repairs int      `json:"repairs"`
	Base    *metrics `json:"baseline,omitempty"`
	Repair  *metrics `json:"repair,omitempty"`
	Error   string   `json:"error,omitempty"`
}

type report struct {
	Version   int          `json:"version"`
	Voicebank string       `json:"voicebank"`
	Corpus    string       `json:"corpus"`
	BridgeMS  float64      `json:"boundary_bridge_ms"`
	Threshold float64      `json:"boundary_bridge_threshold"`
	Repairs   int          `json:"repairs"`
	Base      metrics      `json:"baseline"`
	Repair    metrics      `json:"repair"`
	Cases     []caseReport `json:"cases"`
}

func main() {
	var cfg tts.Config
	var corpusPath, outputPath, rendererID string
	var rendererDirectories []string
	flag.StringVar(&cfg.VoicebankPath, "voicebank", "", "path to a UTAU voicebank directory")
	flag.StringVar(&corpusPath, "corpus", "", "versioned evaluation corpus JSON")
	flag.StringVar(&outputPath, "out", "", "output benchmark JSON")
	flag.Float64Var(&cfg.BoundaryBridgeMS, "boundary-bridge-ms", 20, "maximum repair width")
	flag.Float64Var(&cfg.BoundaryBridgeThreshold, "boundary-bridge-threshold", 0, "repair join-score threshold")
	flag.StringVar(&rendererID, "renderer", "", "renderer plugin ID (default: highest manifest priority)")
	flag.Func("renderer-dir", "renderer plugin directory (repeatable)", func(value string) error { rendererDirectories = append(rendererDirectories, value); return nil })
	flag.Parse()
	renderers, err := rendererplugin.Discover(rendererDirectories)
	if err != nil {
		log.Printf("renderer plugin discovery warning: %v", err)
	}
	renderer, err := rendererplugin.Resolve(renderers, rendererID)
	if err != nil {
		log.Fatal(err)
	}
	rendererplugin.Apply(renderer, &cfg)
	if cfg.VoicebankPath == "" || corpusPath == "" || outputPath == "" {
		flag.Usage()
		log.Fatal("--voicebank, --corpus and --out are required")
	}
	corpus, err := evaluation.LoadCorpus(corpusPath)
	if err != nil {
		log.Fatal(err)
	}
	result := report{Version: 1, Voicebank: cfg.VoicebankPath, Corpus: corpus.Name, BridgeMS: cfg.BoundaryBridgeMS, Threshold: cfg.BoundaryBridgeThreshold}
	requestedMS := cfg.BoundaryBridgeMS
	for _, item := range corpus.Cases {
		current := caseReport{ID: item.ID, Text: item.Text}
		cfg.Text, cfg.BoundaryBridgeMS = item.Text, 0
		base, err := tts.Synthesize(cfg)
		if err != nil {
			current.Error = "baseline: " + err.Error()
			result.Cases = append(result.Cases, current)
			continue
		}
		baseEval, err := evaluation.Analyze(base.Audio, base.Plan)
		if err != nil {
			current.Error = "baseline evaluation: " + err.Error()
			result.Cases = append(result.Cases, current)
			continue
		}
		cfg.BoundaryBridgeMS = requestedMS
		repaired, err := tts.Synthesize(cfg)
		if err != nil {
			current.Error = "repair: " + err.Error()
			result.Cases = append(result.Cases, current)
			continue
		}
		repairEval, err := evaluation.Analyze(repaired.Audio, repaired.Plan)
		if err != nil {
			current.Error = "repair evaluation: " + err.Error()
			result.Cases = append(result.Cases, current)
			continue
		}
		baseMetrics, repairMetrics := fromReport(baseEval), fromReport(repairEval)
		current.Base, current.Repair, current.Repairs = &baseMetrics, &repairMetrics, len(repaired.Plan.BoundaryBridges)
		result.Repairs += current.Repairs
		accumulate(&result.Base, baseMetrics)
		accumulate(&result.Repair, repairMetrics)
		result.Cases = append(result.Cases, current)
		fmt.Printf("%s repairs=%d click=%.5f->%.5f spectrum=%.3f->%.3f\n", item.ID, current.Repairs, baseMetrics.MeanClick, repairMetrics.MeanClick, baseMetrics.MeanSpecDB, repairMetrics.MeanSpecDB)
	}
	finish(&result.Base)
	finish(&result.Repair)
	if result.Base.Connected == 0 || result.Repair.Connected == 0 {
		log.Fatal("no cases could be evaluated")
	}
	if err := writeJSON(outputPath, result); err != nil {
		log.Fatal(err)
	}
}

func fromReport(value *evaluation.Report) metrics {
	return metrics{Connected: value.ConnectedCount, MeanClick: value.MeanClick, MeanPeak: value.MeanPeakClick, MeanDelta: value.MeanDeltaRMS, MeanRMSDB: value.MeanRMSDeltaDB, MeanSpecDB: value.MeanSpectrumDB, MeanF0: value.MeanF0DeltaCents}
}

func accumulate(total *metrics, value metrics) {
	weight := float64(value.Connected)
	total.Connected += value.Connected
	total.MeanClick += value.MeanClick * weight
	total.MeanPeak += value.MeanPeak * weight
	total.MeanDelta += value.MeanDelta * weight
	total.MeanRMSDB += value.MeanRMSDB * weight
	total.MeanSpecDB += value.MeanSpecDB * weight
	total.MeanF0 += value.MeanF0 * weight
}

func finish(value *metrics) {
	if value.Connected == 0 {
		return
	}
	weight := float64(value.Connected)
	value.MeanClick /= weight
	value.MeanPeak /= weight
	value.MeanDelta /= weight
	value.MeanRMSDB /= weight
	value.MeanSpecDB /= weight
	value.MeanF0 /= weight
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
