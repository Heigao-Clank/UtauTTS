package main

import (
	"flag"
	"fmt"
	"log"

	"utautts/internal/prosody"
)

func main() {
	var dataset, output string
	var epochs int
	var rate, l2 float64
	var seed int64
	flag.StringVar(&dataset, "dataset", "", "training JSONL from prosody-dataset")
	flag.StringVar(&output, "out", "out/prosody/model.json", "output model JSON")
	flag.IntVar(&epochs, "epochs", 30, "training epochs")
	flag.Float64Var(&rate, "learning-rate", 0.01, "Adam learning rate")
	flag.Float64Var(&l2, "l2", 0.00001, "L2 regularization")
	flag.Int64Var(&seed, "seed", 1, "deterministic random seed")
	flag.Parse()
	if dataset == "" {
		flag.Usage()
		log.Fatal("--dataset is required")
	}
	records, err := prosody.LoadJSONL(dataset)
	if err != nil {
		log.Fatal(err)
	}
	model, err := prosody.Train(records, prosody.TrainConfig{Epochs: epochs, LearningRate: rate, L2: l2, Seed: seed})
	if err != nil {
		log.Fatal(err)
	}
	if err := model.Save(output); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("wrote %s (train %d records/%d tokens, validation %d records; duration MAE %.1fms, pitch MAE %.1f cents, energy MAE %.3f)\n",
		output, model.Training.Records, model.Training.Tokens, model.Metrics.Records, model.Metrics.DurationMAEMS, model.Metrics.PitchMAECents, model.Metrics.EnergyRatioMAE)
}
