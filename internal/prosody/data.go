package prosody

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"utautts/internal/frontend"
)

const DatasetVersion = 1

type Record struct {
	Version      int      `json:"version"`
	ID           string   `json:"id"`
	Text         string   `json:"text"`
	Reading      string   `json:"reading"`
	AudioPath    string   `json:"audio_path"`
	SampleRate   int      `json:"sample_rate"`
	StartMS      float64  `json:"start_ms"`
	EndMS        float64  `json:"end_ms"`
	MedianF0Hz   float64  `json:"median_f0_hz"`
	MedianEnergy float64  `json:"median_energy"`
	Tokens       []Target `json:"tokens"`
}

type Target struct {
	Position    int     `json:"position"`
	Mora        string  `json:"mora,omitempty"`
	Vowel       string  `json:"vowel,omitempty"`
	Pause       bool    `json:"pause,omitempty"`
	StartMS     float64 `json:"start_ms"`
	EndMS       float64 `json:"end_ms"`
	DurationMS  float64 `json:"duration_ms"`
	F0Hz        float64 `json:"f0_hz,omitempty"`
	PitchRatio  float64 `json:"pitch_ratio,omitempty"`
	Energy      float64 `json:"energy"`
	EnergyRatio float64 `json:"energy_ratio"`
}

type Prediction struct {
	DurationMS     float64
	DurationFactor float64
	PitchFactor    float64
	EnergyFactor   float64
}

func LoadJSONL(path string) ([]Record, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var records []Record
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	line := 0
	for scanner.Scan() {
		line++
		if len(scanner.Bytes()) == 0 {
			continue
		}
		var record Record
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			return nil, fmt.Errorf("%s:%d: %w", path, line, err)
		}
		if record.Version != DatasetVersion {
			return nil, fmt.Errorf("%s:%d: unsupported dataset version %d", path, line, record.Version)
		}
		records = append(records, record)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return records, nil
}

func WriteJSONLine(writer *bufio.Writer, record Record) error {
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	if _, err := writer.Write(data); err != nil {
		return err
	}
	return writer.WriteByte('\n')
}

func finalizeRatios(targets []Target) (float64, float64) {
	var f0Values, energyValues []float64
	for _, target := range targets {
		if !target.Pause && target.F0Hz > 0 {
			f0Values = append(f0Values, target.F0Hz)
		}
		if !target.Pause && target.Energy > 0 {
			energyValues = append(energyValues, target.Energy)
		}
	}
	medianF0 := median(f0Values)
	if medianF0 > 0 {
		for i := range targets {
			if targets[i].Pause || targets[i].F0Hz <= 0 {
				continue
			}
			for targets[i].F0Hz > medianF0*1.6 {
				targets[i].F0Hz /= 2
			}
			for targets[i].F0Hz < medianF0/1.6 {
				targets[i].F0Hz *= 2
			}
		}
		f0Values = f0Values[:0]
		for _, target := range targets {
			if !target.Pause && target.F0Hz > 0 {
				f0Values = append(f0Values, target.F0Hz)
			}
		}
		medianF0 = median(f0Values)
	}
	medianEnergy := median(energyValues)
	for i := range targets {
		if medianF0 > 0 && targets[i].F0Hz > 0 {
			targets[i].PitchRatio = targets[i].F0Hz / medianF0
		}
		if medianEnergy > 0 && targets[i].Energy > 0 {
			targets[i].EnergyRatio = targets[i].Energy / medianEnergy
		} else {
			targets[i].EnergyRatio = 1
		}
	}
	return medianF0, medianEnergy
}

func median(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	copyValues := append([]float64(nil), values...)
	sort.Float64s(copyValues)
	middle := len(copyValues) / 2
	if len(copyValues)%2 == 1 {
		return copyValues[middle]
	}
	return (copyValues[middle-1] + copyValues[middle]) / 2
}

func moraFromTarget(target Target) frontend.Mora {
	return frontend.Mora{Text: target.Mora, Consonant: frontend.ConsonantOf(target.Mora), Vowel: target.Vowel, Pause: target.Pause}
}
