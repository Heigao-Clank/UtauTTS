package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"

	"utautts/internal/audio"
	"utautts/internal/tts"
	"utautts/internal/voicebank"
)

func main() {
	var (
		voicebankPath       string
		otoPath             string
		reading             string
		text                string
		tone                string
		outPath             string
		planPath            string
		moraMS              float64
		pauseMS             float64
		releaseMS           float64
		prosodyPath         string
		prosodyPitchOnly    bool
		pitchContourPath    string
		pitchContourCase    string
		intonationStrength  float64
		renderer            string
		worldlinePath       string
		worldlineBridgePath string
		selectionMode       string
		joinModelPath       string
		joinScoreScale      float64
	)
	flag.StringVar(&voicebankPath, "voicebank", "", "path to a UTAU voicebank directory")
	flag.StringVar(&otoPath, "oto", "", "deprecated alias for --voicebank")
	flag.StringVar(&reading, "kana", "", "kana reading to synthesize")
	flag.StringVar(&text, "text", "", "Japanese text to synthesize")
	flag.StringVar(&tone, "tone", "C4", "voicebank tone used with prefix.map")
	flag.StringVar(&outPath, "out", "", "output WAV path")
	flag.StringVar(&planPath, "plan-out", "", "optional synthesis plan JSON path")
	flag.Float64Var(&moraMS, "mora-ms", 140, "base mora duration in milliseconds")
	flag.Float64Var(&pauseMS, "pause-ms", 180, "punctuation pause in milliseconds")
	flag.Float64Var(&releaseMS, "release-ms", 20, "unit release envelope in milliseconds")
	flag.StringVar(&prosodyPath, "prosody", "", "optional learned prosody model JSON")
	flag.BoolVar(&prosodyPitchOnly, "prosody-pitch-only", false, "apply only learned pitch and keep fixed duration/energy")
	flag.StringVar(&pitchContourPath, "pitch-contours", "", "optional per-case pitch contour JSON")
	flag.StringVar(&pitchContourCase, "pitch-case", "", "case ID in --pitch-contours")
	flag.Float64Var(&intonationStrength, "intonation-strength", 0, "source-pitch stabilization and phrase contour strength (0..1)")
	flag.StringVar(&renderer, "renderer", "waveform", "renderer backend: waveform, waveform-long, worldline, worldline-hybrid, worldline-hybrid-cv, or worldline-hybrid-cv-balanced")
	flag.StringVar(&worldlinePath, "worldline", "", "path to OpenUtau worldline library (default: next to executable)")
	flag.StringVar(&worldlineBridgePath, "worldline-bridge", "", "path to utautts-worldline-bridge executable")
	flag.StringVar(&selectionMode, "selection", string(voicebank.SelectionViterbi), "unit selection: viterbi, greedy, or target-only")
	flag.StringVar(&joinModelPath, "join-model", "", "optional learned join-cost model JSON")
	flag.Float64Var(&joinScoreScale, "join-scale", 0, "learned logit score scale (default: model or 4)")
	flag.Parse()

	if voicebankPath == "" {
		voicebankPath = otoPath
	}
	if voicebankPath == "" || (reading == "" && text == "") || outPath == "" {
		flag.Usage()
		log.Fatal("--voicebank, --out, and either --text or --kana are required")
	}

	pitchFactors, err := loadPitchFactors(pitchContourPath, pitchContourCase)
	if err != nil {
		log.Fatal(err)
	}
	result, err := tts.Synthesize(tts.Config{
		VoicebankPath:       voicebankPath,
		Text:                text,
		Reading:             reading,
		Tone:                tone,
		MoraDurationMS:      moraMS,
		PauseDurationMS:     pauseMS,
		ReleaseMS:           releaseMS,
		ProsodyModelPath:    prosodyPath,
		ProsodyPitchOnly:    prosodyPitchOnly,
		PitchFactors:        pitchFactors,
		IntonationStrength:  intonationStrength,
		Renderer:            renderer,
		WorldlinePath:       worldlinePath,
		WorldlineBridgePath: worldlineBridgePath,
		SelectionMode:       voicebank.SelectionMode(selectionMode),
		JoinModelPath:       joinModelPath,
		JoinScoreScale:      joinScoreScale,
	})
	if err != nil {
		log.Fatal(err)
	}
	if err := audio.WriteWav(outPath, result.Audio); err != nil {
		log.Fatal(err)
	}
	if planPath != "" {
		data, err := json.MarshalIndent(result.Plan, "", "  ")
		if err != nil {
			log.Fatal(err)
		}
		data = append(data, '\n')
		if err := os.WriteFile(planPath, data, 0o644); err != nil {
			log.Fatal(err)
		}
	}

	duration := float64(len(result.Audio.Data)) / float64(result.Audio.SampleRate)
	fmt.Printf("wrote %s (%.2fs, %d Hz, %d units)\n", outPath, duration, result.Audio.SampleRate, len(result.Plan.Units))
}

func loadPitchFactors(path, caseID string) ([]float64, error) {
	if path == "" {
		return nil, nil
	}
	if caseID == "" {
		return nil, fmt.Errorf("--pitch-case is required with --pitch-contours")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var corpus struct {
		Cases []struct {
			ID           string    `json:"id"`
			PitchFactors []float64 `json:"pitch_factors"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(data, &corpus); err != nil {
		return nil, err
	}
	for _, item := range corpus.Cases {
		if item.ID == caseID {
			return item.PitchFactors, nil
		}
	}
	return nil, fmt.Errorf("pitch contour case %q not found in %s", caseID, path)
}
