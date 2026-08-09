package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"utautts/internal/dataset"
	"utautts/internal/voicebank"
)

func main() {
	var voicebankPath, outputPath, reportPath string
	var config dataset.ConnectionConfig
	flag.StringVar(&voicebankPath, "voicebank", "", "path to a UTAU voicebank directory")
	flag.StringVar(&outputPath, "out", "", "output JSONL path")
	flag.StringVar(&reportPath, "report", "", "summary JSON path (default: <out>.report.json)")
	flag.IntVar(&config.NegativesPerPositive, "negatives", 3, "negative examples per positive pair")
	flag.IntVar(&config.Limit, "limit", 0, "maximum natural pairs (0 means all)")
	flag.Parse()
	if voicebankPath == "" || outputPath == "" {
		flag.Usage()
		log.Fatal("--voicebank and --out are required")
	}
	if config.NegativesPerPositive < 0 || config.Limit < 0 {
		log.Fatal("--negatives and --limit must be non-negative")
	}

	bank, err := voicebank.Load(voicebankPath)
	if err != nil {
		log.Fatal(err)
	}
	records, report := dataset.BuildConnections(bank, config)
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		log.Fatal(err)
	}
	if err := writeJSONL(outputPath, records); err != nil {
		log.Fatal(err)
	}
	if reportPath == "" {
		reportPath = outputPath + ".report.json"
	}
	if err := writeJSON(reportPath, report); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("voicebank=%s positive=%d negative=%d missing-negative=%d invalid-positive=%d invalid-negative=%d\n",
		report.Voicebank, report.PositiveRecords, report.NegativeRecords,
		report.PairsWithoutNegative, report.InvalidPositiveRecords, report.InvalidNegativeRecords)
}

func writeJSONL(path string, records []dataset.ConnectionRecord) (result error) {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := file.Close(); result == nil {
			result = closeErr
		}
	}()
	writer := bufio.NewWriter(file)
	encoder := json.NewEncoder(writer)
	encoder.SetEscapeHTML(false)
	for _, record := range records {
		if err := encoder.Encode(record); err != nil {
			return err
		}
	}
	return writer.Flush()
}

func writeJSON(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}
