package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"

	"utautts/internal/dataset"
)

type annotationFile struct {
	Version     int          `json:"version"`
	Annotations []annotation `json:"annotations"`
}

type annotation struct {
	RecordID string  `json:"record_id"`
	Label    int     `json:"label"`
	Weight   float64 `json:"weight,omitempty"`
}

func main() {
	var datasetPath, annotationsPath, outputPath string
	var defaultWeight float64
	flag.StringVar(&datasetPath, "dataset", "", "source connection JSONL")
	flag.StringVar(&annotationsPath, "annotations", "", "human annotation JSON")
	flag.StringVar(&outputPath, "out", "", "output connection JSONL")
	flag.Float64Var(&defaultWeight, "weight", 3, "default human-label training weight")
	flag.Parse()
	if datasetPath == "" || annotationsPath == "" || outputPath == "" || defaultWeight <= 0 {
		flag.Usage()
		log.Fatal("--dataset, --annotations, --out, and positive --weight are required")
	}
	records, err := dataset.LoadConnections(datasetPath)
	if err != nil {
		log.Fatal(err)
	}
	data, err := os.ReadFile(annotationsPath)
	if err != nil {
		log.Fatal(err)
	}
	var input annotationFile
	if err := json.Unmarshal(data, &input); err != nil {
		log.Fatal(err)
	}
	if input.Version != 1 {
		log.Fatalf("unsupported annotation version %d", input.Version)
	}
	annotations := map[string]annotation{}
	for _, item := range input.Annotations {
		if item.RecordID == "" || (item.Label != 0 && item.Label != 1) {
			log.Fatal("each annotation needs a record_id and label 0 or 1")
		}
		if _, exists := annotations[item.RecordID]; exists {
			log.Fatalf("duplicate annotation for %s", item.RecordID)
		}
		annotations[item.RecordID] = item
	}
	matched := 0
	for index := range records {
		records[index].SchemaVersion = dataset.SchemaVersion
		item, ok := annotations[records[index].RecordID]
		if !ok {
			continue
		}
		records[index].Label = item.Label
		records[index].LabelSource = "human"
		records[index].Weight = item.Weight
		if records[index].Weight <= 0 {
			records[index].Weight = defaultWeight
		}
		matched++
	}
	if matched != len(annotations) {
		log.Fatalf("matched %d of %d annotations", matched, len(annotations))
	}
	if err := dataset.SaveConnections(outputPath, records); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("wrote %d records with %d human labels to %s\n", len(records), matched, outputPath)
}
