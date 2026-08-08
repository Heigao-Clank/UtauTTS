package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"

	"utautts/internal/audio"
	"utautts/internal/tts"
)

func main() {
	var (
		voicebankPath string
		otoPath       string
		reading       string
		text          string
		tone          string
		outPath       string
		planPath      string
		moraMS        float64
		pauseMS       float64
		releaseMS     float64
		prosodyPath   string
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
	flag.Parse()

	if voicebankPath == "" {
		voicebankPath = otoPath
	}
	if voicebankPath == "" || (reading == "" && text == "") || outPath == "" {
		flag.Usage()
		log.Fatal("--voicebank, --out, and either --text or --kana are required")
	}

	result, err := tts.Synthesize(tts.Config{
		VoicebankPath:    voicebankPath,
		Text:             text,
		Reading:          reading,
		Tone:             tone,
		MoraDurationMS:   moraMS,
		PauseDurationMS:  pauseMS,
		ReleaseMS:        releaseMS,
		ProsodyModelPath: prosodyPath,
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
