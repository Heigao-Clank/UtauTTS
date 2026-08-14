package plan

import (
	"fmt"
	"math"

	"utautts/internal/frontend"
	"utautts/internal/prosody"
	"utautts/internal/voicebank"
)

const Version = 9

type Config struct {
	MoraDurationMS   float64
	PauseDurationMS  float64
	Predictions      []prosody.Prediction
	SelectionMode    voicebank.SelectionMode
	JoinCostMode     string
	JoinModelVersion int
	JoinScoreScale   float64
}

type Plan struct {
	Version                 int                      `json:"version"`
	Voicebank               string                   `json:"voicebank"`
	Text                    string                   `json:"text,omitempty"`
	Reading                 string                   `json:"reading"`
	SelectionMode           string                   `json:"selection_mode"`
	JoinCostMode            string                   `json:"join_cost_mode"`
	JoinModelVersion        int                      `json:"join_model_version,omitempty"`
	JoinScoreScale          float64                  `json:"join_score_scale,omitempty"`
	BoundaryBridgeMS        float64                  `json:"boundary_bridge_ms,omitempty"`
	BoundaryBridgeThreshold float64                  `json:"boundary_bridge_threshold,omitempty"`
	BoundaryBridges         []BoundaryBridge         `json:"boundary_bridges,omitempty"`
	BoundaryRepairDecisions []BoundaryRepairDecision `json:"boundary_repair_decisions,omitempty"`
	DurationMS              float64                  `json:"duration_ms"`
	Units                   []Unit                   `json:"units"`
}

// BoundaryBridge records an optional short transition repair applied by a
// renderer. It does not add a mora or change the linguistic unit sequence.
type BoundaryBridge struct {
	UnitIndex   int     `json:"unit_index"`
	Position    int     `json:"position"`
	StartMS     float64 `json:"start_ms"`
	EndMS       float64 `json:"end_ms"`
	DurationMS  float64 `json:"duration_ms"`
	LagMS       float64 `json:"lag_ms,omitempty"`
	JoinScore   float64 `json:"join_score"`
	Correlation float64 `json:"correlation,omitempty"`
	Source      string  `json:"source"`
	Kind        string  `json:"kind"`
}

// BoundaryRepairDecision records normal-versus-repair candidate selection,
// including decisions that deliberately keep the unmodified connection.
type BoundaryRepairDecision struct {
	UnitIndex        int     `json:"unit_index"`
	Position         int     `json:"position"`
	CandidateCount   int     `json:"candidate_count"`
	SelectedKind     string  `json:"selected_kind"`
	Applied          bool    `json:"applied"`
	DurationMS       float64 `json:"duration_ms,omitempty"`
	LagMS            float64 `json:"lag_ms,omitempty"`
	JoinScore        float64 `json:"join_score"`
	Correlation      float64 `json:"correlation,omitempty"`
	BaselinePeak     float64 `json:"baseline_peak_delta"`
	SelectedPeak     float64 `json:"selected_peak_delta"`
	BaselineDeltaRMS float64 `json:"baseline_delta_rms"`
	SelectedDeltaRMS float64 `json:"selected_delta_rms"`
}

type Unit struct {
	Position                int     `json:"position"`
	Mora                    string  `json:"mora"`
	Alias                   string  `json:"alias"`
	Source                  string  `json:"source"`
	Silent                  bool    `json:"silent,omitempty"`
	LongUnitGroup           int     `json:"long_unit_group,omitempty"`
	LongUnitSize            int     `json:"long_unit_size,omitempty"`
	OtoPath                 string  `json:"oto_path"`
	OtoLine                 int     `json:"oto_line"`
	NoteStartMS             float64 `json:"note_start_ms"`
	DurationMS              float64 `json:"duration_ms"`
	OffsetMS                float64 `json:"offset_ms"`
	ConsonantMS             float64 `json:"consonant_ms"`
	CutoffMS                float64 `json:"cutoff_ms"`
	PreutteranceMS          float64 `json:"preutterance_ms"`
	OverlapMS               float64 `json:"overlap_ms"`
	PitchFactor             float64 `json:"pitch_factor"`
	EnergyFactor            float64 `json:"energy_factor"`
	TimingScale             float64 `json:"timing_scale"`
	EffectivePreutteranceMS float64 `json:"effective_preutterance_ms"`
	EffectiveConsonantMS    float64 `json:"effective_consonant_ms"`
	EffectiveOverlapMS      float64 `json:"effective_overlap_ms"`
	SourceF0Hz              float64 `json:"source_f0_hz,omitempty"`
	TargetF0Hz              float64 `json:"target_f0_hz,omitempty"`
	IntonationFactor        float64 `json:"intonation_factor"`
	CandidateCount          int     `json:"candidate_count"`
	TargetScore             float64 `json:"target_score"`
	JoinScore               float64 `json:"join_score"`
	JoinProbability         float64 `json:"join_probability,omitempty"`
	PathScore               float64 `json:"path_score"`
}

func Build(bank *voicebank.Bank, reading string, morae []frontend.Mora, selections []voicebank.Selection, cfg Config) (*Plan, error) {
	if math.IsNaN(cfg.MoraDurationMS) || math.IsInf(cfg.MoraDurationMS, 0) {
		return nil, fmt.Errorf("mora duration must be finite, got %v", cfg.MoraDurationMS)
	}
	if math.IsNaN(cfg.PauseDurationMS) || math.IsInf(cfg.PauseDurationMS, 0) {
		return nil, fmt.Errorf("pause duration must be finite, got %v", cfg.PauseDurationMS)
	}
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

	selectionMode := cfg.SelectionMode
	if selectionMode == "" {
		selectionMode = voicebank.SelectionViterbi
	}
	joinCostMode := cfg.JoinCostMode
	if joinCostMode == "" {
		joinCostMode = "handcrafted"
	}
	result := &Plan{
		Version: Version, Voicebank: bank.Root, Reading: reading,
		SelectionMode: string(selectionMode), JoinCostMode: joinCostMode,
		JoinModelVersion: cfg.JoinModelVersion,
		JoinScoreScale:   cfg.JoinScoreScale,
	}
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
			} else if prediction.DurationFactor > 0 {
				duration *= prediction.DurationFactor
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
		} else if prediction.DurationFactor > 0 {
			duration *= prediction.DurationFactor
		}
		entry := selection.Entry
		result.Units = append(result.Units, Unit{
			Position:        position,
			Mora:            mora.Text,
			Alias:           selection.Alias,
			Source:          entry.Filename,
			Silent:          entry.Filename == "",
			OtoPath:         entry.OtoPath,
			OtoLine:         entry.Line,
			NoteStartMS:     cursor,
			DurationMS:      duration,
			OffsetMS:        entry.Offset,
			ConsonantMS:     entry.Fixed,
			CutoffMS:        entry.Blank,
			PreutteranceMS:  entry.Preutterance,
			OverlapMS:       entry.Overlap,
			PitchFactor:     prediction.PitchFactor,
			EnergyFactor:    prediction.EnergyFactor,
			CandidateCount:  selection.CandidateCount,
			TargetScore:     selection.TargetScore,
			JoinScore:       selection.JoinScore,
			JoinProbability: selection.JoinProbability,
			PathScore:       selection.PathScore,
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
