package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"utautts/internal/connection"
	"utautts/internal/frontend"
	"utautts/internal/voicebank"
)

type report struct {
	Version   int                     `json:"version"`
	Voicebank string                  `json:"voicebank"`
	Text      string                  `json:"text,omitempty"`
	Reading   string                  `json:"reading"`
	Audit     *voicebank.LatticeAudit `json:"audit"`
}

func main() {
	var voicebankPath, text, reading, tone, modelPath, outputPath string
	flag.StringVar(&voicebankPath, "voicebank", "", "path to a UTAU voicebank directory")
	flag.StringVar(&text, "text", "", "Japanese text to inspect")
	flag.StringVar(&reading, "kana", "", "kana reading to inspect")
	flag.StringVar(&tone, "tone", "C4", "voicebank tone used with prefix.map")
	flag.StringVar(&modelPath, "join-model", "", "optional learned join-cost model JSON")
	flag.StringVar(&outputPath, "out", "", "output audit JSON")
	flag.Parse()
	if voicebankPath == "" || outputPath == "" || (text == "" && reading == "") {
		flag.Usage()
		log.Fatal("--voicebank, --out, and either --text or --kana are required")
	}
	bank, err := voicebank.Load(voicebankPath)
	if err != nil {
		log.Fatal(err)
	}
	if reading == "" {
		reading, err = frontend.ToKana(text)
		if err != nil {
			log.Fatal(err)
		}
	}
	morae, err := frontend.ParseKana(reading)
	if err != nil {
		log.Fatal(err)
	}
	var model *connection.LearnedModel
	if modelPath != "" {
		model, err = connection.LoadLearnedModel(modelPath)
		if err != nil {
			log.Fatal(err)
		}
	}
	audit, err := bank.AuditLattice(morae, tone, model)
	if err != nil {
		log.Fatal(err)
	}
	result := report{Version: 1, Voicebank: bank.Name, Text: text, Reading: reading, Audit: audit}
	if err := writeJSON(outputPath, result); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("positions=%d multi=%d same-target=%d handcrafted-changed=%d learned-changed=%d learned-from-handcrafted=%d\n",
		len(audit.Positions), audit.MultiCandidatePositions, audit.SameTargetPositions,
		audit.HandcraftedChanges, audit.LearnedChanges, audit.LearnedFromHandcrafted)
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
