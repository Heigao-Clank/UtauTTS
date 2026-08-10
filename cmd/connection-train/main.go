package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"utautts/internal/connection"
	"utautts/internal/dataset"
)

type pathList []string

func (paths *pathList) String() string { return strings.Join(*paths, ",") }
func (paths *pathList) Set(value string) error {
	*paths = append(*paths, value)
	return nil
}

type splitReport struct {
	Version                int      `json:"version"`
	Datasets               []string `json:"datasets"`
	ValidationVoicebank    string   `json:"validation_voicebank,omitempty"`
	ValidationFraction     float64  `json:"validation_fraction,omitempty"`
	TrainingRecords        int      `json:"training_records_before_filter"`
	ValidationRecords      int      `json:"validation_records_before_filter"`
	TrainingValidRecords   int      `json:"training_valid_records"`
	ValidationValidRecords int      `json:"validation_valid_records"`
	TrainingVoicebanks     []string `json:"training_voicebanks"`
	ValidationVoicebanks   []string `json:"validation_voicebanks"`
	TrainingGroups         int      `json:"training_groups"`
	ValidationGroups       int      `json:"validation_groups"`
}

func main() {
	var paths pathList
	var output, reportPath, validationVoicebank, modelType string
	var validationFraction, rate, l2 float64
	var epochs, hiddenUnits int
	var seed uint64
	flag.Var(&paths, "dataset", "connection JSONL path (repeatable)")
	flag.StringVar(&output, "out", "out/connection/model.json", "output model JSON")
	flag.StringVar(&reportPath, "split-report", "", "split report JSON (default: <out>.split.json)")
	flag.StringVar(&validationVoicebank, "validation-voicebank", "", "hold out this entire voicebank")
	flag.Float64Var(&validationFraction, "validation-fraction", 0.2, "group-level validation fraction when no voicebank is specified")
	flag.IntVar(&epochs, "epochs", 400, "training epochs")
	flag.StringVar(&modelType, "model", "logistic", "model type: logistic or mlp")
	flag.IntVar(&hiddenUnits, "hidden", 32, "MLP hidden units")
	flag.Float64Var(&rate, "learning-rate", 0.08, "gradient descent learning rate")
	flag.Float64Var(&l2, "l2", 0.001, "L2 regularization")
	flag.Uint64Var(&seed, "seed", 1, "deterministic split seed")
	flag.Parse()
	if len(paths) == 0 {
		flag.Usage()
		log.Fatal("at least one --dataset is required")
	}
	if validationVoicebank != "" {
		validationFraction = 0
	}
	if validationFraction < 0 || validationFraction >= 1 {
		log.Fatal("--validation-fraction must be in [0, 1)")
	}

	var examples []connection.Example
	for _, path := range paths {
		records, err := dataset.LoadConnections(path)
		if err != nil {
			log.Fatal(err)
		}
		for _, record := range records {
			examples = append(examples, connection.Example{
				Voicebank: record.Voicebank, GroupID: record.GroupID,
				Label: record.Label, Features: record.Features, Weight: record.Weight,
			})
		}
	}
	training, validation := connection.SplitExamples(examples, connection.SplitConfig{
		ValidationVoicebank: validationVoicebank,
		ValidationFraction:  validationFraction,
		Seed:                seed,
	})
	if len(validation) == 0 {
		log.Fatal("validation split is empty; check --validation-voicebank or --validation-fraction")
	}
	model, err := connection.TrainModel(training, validation, connection.TrainConfig{
		Epochs: epochs, LearningRate: rate, L2: l2, Model: modelType, HiddenUnits: hiddenUnits, Seed: int64(seed),
	})
	if err != nil {
		log.Fatal(err)
	}
	if err := connection.SaveLearnedModel(output, model); err != nil {
		log.Fatal(err)
	}
	if reportPath == "" {
		reportPath = output + ".split.json"
	}
	report := buildSplitReport(paths, validationVoicebank, validationFraction, training, validation)
	if err := writeJSON(reportPath, report); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("train=%d validation=%d positive=%d negative=%d accuracy=%.3f balanced=%.3f auc=%.3f log-loss=%.3f\n",
		model.Training.Records, model.Metrics.Records, model.Metrics.Positive, model.Metrics.Negative,
		model.Metrics.Accuracy, model.Metrics.BalancedAccuracy, model.Metrics.AUC, model.Metrics.LogLoss)
}

func buildSplitReport(paths []string, heldOut string, fraction float64, training, validation []connection.Example) splitReport {
	return splitReport{
		Version: 1, Datasets: paths, ValidationVoicebank: heldOut, ValidationFraction: fraction,
		TrainingRecords: len(training), ValidationRecords: len(validation),
		TrainingValidRecords: validCount(training), ValidationValidRecords: validCount(validation),
		TrainingVoicebanks: exampleVoicebanks(training), ValidationVoicebanks: exampleVoicebanks(validation),
		TrainingGroups: groupCount(training), ValidationGroups: groupCount(validation),
	}
}

func validCount(examples []connection.Example) int {
	count := 0
	for _, example := range examples {
		if example.Features.Valid() {
			count++
		}
	}
	return count
}

func exampleVoicebanks(examples []connection.Example) []string {
	set := map[string]bool{}
	for _, example := range examples {
		set[example.Voicebank] = true
	}
	result := make([]string, 0, len(set))
	for name := range set {
		result = append(result, name)
	}
	sort.Strings(result)
	return result
}

func groupCount(examples []connection.Example) int {
	set := map[string]bool{}
	for _, example := range examples {
		set[example.Voicebank+"\x00"+example.GroupID] = true
	}
	return len(set)
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
