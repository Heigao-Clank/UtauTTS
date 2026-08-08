package plan

import (
	"fmt"

	"utautts/internal/frontend"
	"utautts/internal/prosody"
	"utautts/internal/voicebank"
)

const Version = 2

type Config struct {
	MoraDurationMS  float64
	PauseDurationMS float64
	Predictions     []prosody.Prediction
}

type Plan struct {
	Version    int     `json:"version"`
	Voicebank  string  `json:"voicebank"`
	Text       string  `json:"text,omitempty"`
	Reading    string  `json:"reading"`
	DurationMS float64 `json:"duration_ms"`
	Units      []Unit  `json:"units"`
}

type Unit struct {
	Position       int     `json:"position"`
	Mora           string  `json:"mora"`
	Alias          string  `json:"alias"`
	Source         string  `json:"source"`
	OtoPath        string  `json:"oto_path"`
	OtoLine        int     `json:"oto_line"`
	NoteStartMS    float64 `json:"note_start_ms"`
	DurationMS     float64 `json:"duration_ms"`
	OffsetMS       float64 `json:"offset_ms"`
	ConsonantMS    float64 `json:"consonant_ms"`
	CutoffMS       float64 `json:"cutoff_ms"`
	PreutteranceMS float64 `json:"preutterance_ms"`
	OverlapMS      float64 `json:"overlap_ms"`
	PitchFactor    float64 `json:"pitch_factor"`
	EnergyFactor   float64 `json:"energy_factor"`
}

func Build(bank *voicebank.Bank, reading string, morae []frontend.Mora, selections []voicebank.Selection, cfg Config) (*Plan, error) {
	if cfg.MoraDurationMS <= 0 {
		cfg.MoraDurationMS = 140
	}
	if cfg.PauseDurationMS <= 0 {
		cfg.PauseDurationMS = 180
	}
	byPosition := make(map[int]voicebank.Selection, len(selections))
	for _, selection := range selections {
		byPosition[selection.Position] = selection
	}

	result := &Plan{Version: Version, Voicebank: bank.Root, Reading: reading}
	cursor := 0.0
	for position, mora := range morae {
		prediction := prosody.Prediction{PitchFactor: 1, EnergyFactor: 1}
		if position < len(cfg.Predictions) {
			prediction = cfg.Predictions[position]
			if prediction.PitchFactor <= 0 {
				prediction.PitchFactor = 1
			}
			if prediction.EnergyFactor <= 0 {
				prediction.EnergyFactor = 1
			}
		}
		if mora.Pause {
			duration := cfg.PauseDurationMS
			if prediction.DurationMS > 0 {
				duration = prediction.DurationMS
			}
			cursor += duration
			continue
		}
		selection, ok := byPosition[position]
		if !ok {
			return nil, fmt.Errorf("selection missing for mora %q at position %d", mora.Text, position)
		}
		duration := durationFor(mora, cfg.MoraDurationMS)
		if prediction.DurationMS > 0 {
			duration = prediction.DurationMS
		}
		entry := selection.Entry
		result.Units = append(result.Units, Unit{
			Position:       position,
			Mora:           mora.Text,
			Alias:          selection.Alias,
			Source:         entry.Filename,
			OtoPath:        entry.OtoPath,
			OtoLine:        entry.Line,
			NoteStartMS:    cursor,
			DurationMS:     duration,
			OffsetMS:       entry.Offset,
			ConsonantMS:    entry.Fixed,
			CutoffMS:       entry.Blank,
			PreutteranceMS: entry.Preutterance,
			OverlapMS:      entry.Overlap,
			PitchFactor:    prediction.PitchFactor,
			EnergyFactor:   prediction.EnergyFactor,
		})
		cursor += duration
	}
	result.DurationMS = cursor
	return result, nil
}

func durationFor(mora frontend.Mora, base float64) float64 {
	switch mora.Vowel {
	case "cl":
		return base * 0.65
	case "n":
		return base * 0.9
	}
	if mora.Text == "ー" {
		return base * 1.2
	}
	return base
}
