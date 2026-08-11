package voicebank

import (
	"math"
	"path/filepath"
	"strings"

	"utautts/internal/connection"
	"utautts/internal/frontend"
	"utautts/internal/oto"
)

type LatticeAudit struct {
	Positions               []PositionAudit `json:"positions"`
	MultiCandidatePositions int             `json:"multi_candidate_positions"`
	SameTargetPositions     int             `json:"same_target_positions"`
	PitchCandidates         int             `json:"pitch_candidates"`
	VoicedCandidates        int             `json:"voiced_candidates"`
	PitchChoicePositions    int             `json:"pitch_choice_positions"`
	WidePitchPositions      int             `json:"wide_pitch_positions"`
	WidePitchWithinGroup    int             `json:"wide_pitch_within_group_positions"`
	HandcraftedChanges      int             `json:"handcrafted_changes_from_target_only"`
	LearnedChanges          int             `json:"learned_changes_from_target_only"`
	LearnedFromHandcrafted  int             `json:"learned_changes_from_handcrafted"`
}

type PositionAudit struct {
	Position             int              `json:"position"`
	Mora                 string           `json:"mora"`
	CandidateCount       int              `json:"candidate_count"`
	TargetRange          float64          `json:"target_range"`
	VoicedCandidateCount int              `json:"voiced_candidate_count"`
	PitchMinHz           float64          `json:"pitch_min_hz,omitempty"`
	PitchMaxHz           float64          `json:"pitch_max_hz,omitempty"`
	PitchSpanCents       float64          `json:"pitch_span_cents,omitempty"`
	WithinGroupPitchSpan float64          `json:"within_group_pitch_span_cents,omitempty"`
	SourceGroupCount     int              `json:"source_group_count"`
	TargetSelected       int              `json:"target_selected"`
	HandcraftedSelected  int              `json:"handcrafted_selected"`
	LearnedSelected      int              `json:"learned_selected,omitempty"`
	Candidates           []CandidateAudit `json:"candidates"`
}

type CandidateAudit struct {
	Index                  int     `json:"index"`
	Alias                  string  `json:"alias"`
	Source                 string  `json:"source"`
	OtoPath                string  `json:"oto_path"`
	OtoLine                int     `json:"oto_line"`
	TargetScore            float64 `json:"target_score"`
	SourceGroup            string  `json:"source_group"`
	SourceF0Hz             float64 `json:"source_f0_hz"`
	PitchValid             bool    `json:"pitch_valid"`
	BestHandcraftedJoin    float64 `json:"best_handcrafted_join"`
	BestLearnedJoin        float64 `json:"best_learned_join,omitempty"`
	BestLearnedProbability float64 `json:"best_learned_probability,omitempty"`
}

// AuditLattice reports every retained candidate and its best possible incoming
// edge. It is diagnostic only; synthesis still uses the exact Viterbi path.
func (b *Bank) AuditLattice(morae []frontend.Mora, tone string, model *connection.LearnedModel) (*LatticeAudit, error) {
	layers, err := b.candidateLayers(morae, tone)
	if err != nil {
		return nil, err
	}
	targetPath := selectBestPaths(layers, SelectionTargetOnly, nil)
	handcraftedPath := selectBestPaths(layers, SelectionViterbi, nil)
	var learnedPath []Selection
	if model != nil {
		learnedPath = selectBestPaths(layers, SelectionViterbi, model)
	}
	targetByPosition := selectionsByPosition(targetPath)
	handcraftedByPosition := selectionsByPosition(handcraftedPath)
	learnedByPosition := selectionsByPosition(learnedPath)
	cache := connection.NewExtractor()
	pitchCache := map[candidatePitchKey]candidatePitch{}
	result := &LatticeAudit{}
	for layerIndex, layer := range layers {
		if len(layer) == 0 {
			continue
		}
		position := PositionAudit{
			Position: layer[0].Position, Mora: layer[0].Mora.Text,
			CandidateCount: len(layer), TargetSelected: candidateIndex(layer, targetByPosition[layer[0].Position]),
			HandcraftedSelected: candidateIndex(layer, handcraftedByPosition[layer[0].Position]),
			LearnedSelected:     -1,
		}
		if model != nil {
			position.LearnedSelected = candidateIndex(layer, learnedByPosition[layer[0].Position])
		}
		minimum, maximum := layer[0].TargetScore, layer[0].TargetScore
		sourceGroups := map[string]bool{}
		groupPitches := map[string][]float64{}
		for candidateIndex, candidate := range layer {
			minimum = min(minimum, candidate.TargetScore)
			maximum = max(maximum, candidate.TargetScore)
			measuredPitch := measureCandidateF0(candidate.Entry, pitchCache)
			if candidate.Entry.Filename != "" {
				result.PitchCandidates++
			}
			if measuredPitch.Valid {
				result.VoicedCandidates++
				position.VoicedCandidateCount++
				if position.PitchMinHz == 0 || measuredPitch.Hz < position.PitchMinHz {
					position.PitchMinHz = measuredPitch.Hz
				}
				position.PitchMaxHz = max(position.PitchMaxHz, measuredPitch.Hz)
			}
			group := sourceGroup(b.Root, candidate.Entry)
			sourceGroups[group] = true
			if measuredPitch.Valid {
				groupPitches[group] = append(groupPitches[group], measuredPitch.Hz)
			}
			audit := CandidateAudit{
				Index: candidateIndex, Alias: candidate.Alias,
				Source:  relativePath(b.Root, candidate.Entry.Filename),
				OtoPath: relativePath(b.Root, candidate.Entry.OtoPath), OtoLine: candidate.Entry.Line,
				TargetScore: candidate.TargetScore, SourceGroup: group,
				SourceF0Hz: measuredPitch.Hz, PitchValid: measuredPitch.Valid,
			}
			if layerIndex > 0 && len(layers[layerIndex-1]) > 0 {
				audit.BestHandcraftedJoin = bestIncoming(layers[layerIndex-1], candidate.Entry, cache, nil).score
				if model != nil {
					best := bestIncoming(layers[layerIndex-1], candidate.Entry, cache, model)
					audit.BestLearnedJoin, audit.BestLearnedProbability = best.score, best.probability
				}
			}
			position.Candidates = append(position.Candidates, audit)
		}
		position.TargetRange = maximum - minimum
		position.SourceGroupCount = len(sourceGroups)
		if position.VoicedCandidateCount > 1 && position.PitchMinHz > 0 {
			position.PitchSpanCents = 1200 * math.Log2(position.PitchMaxHz/position.PitchMinHz)
			result.PitchChoicePositions++
			if position.PitchSpanCents >= 50 {
				result.WidePitchPositions++
			}
		}
		for _, values := range groupPitches {
			if len(values) < 2 {
				continue
			}
			minimum, maximum := values[0], values[0]
			for _, value := range values[1:] {
				minimum = min(minimum, value)
				maximum = max(maximum, value)
			}
			span := 1200 * math.Log2(maximum/minimum)
			position.WithinGroupPitchSpan = max(position.WithinGroupPitchSpan, span)
		}
		if position.WithinGroupPitchSpan >= 50 {
			result.WidePitchWithinGroup++
		}
		if len(layer) > 1 {
			result.MultiCandidatePositions++
			if position.TargetRange == 0 {
				result.SameTargetPositions++
			}
		}
		if position.HandcraftedSelected != position.TargetSelected {
			result.HandcraftedChanges++
		}
		if model != nil && position.LearnedSelected != position.TargetSelected {
			result.LearnedChanges++
		}
		if model != nil && position.LearnedSelected != position.HandcraftedSelected {
			result.LearnedFromHandcrafted++
		}
		result.Positions = append(result.Positions, position)
	}
	return result, nil
}

func sourceGroup(root string, entry oto.Entry) string {
	relative := relativePath(root, entry.OtoPath)
	directory := filepath.ToSlash(filepath.Dir(relative))
	if directory == "." || directory == "" {
		return "root"
	}
	return strings.Split(directory, "/")[0]
}

type incomingScore struct{ score, probability float64 }

func bestIncoming(previous []Selection, current oto.Entry, cache *connection.Extractor, model *connection.LearnedModel) incomingScore {
	best := incomingScore{score: -1e100}
	for _, candidate := range previous {
		score, probability := pairScore(candidate.Entry, current, cache, model)
		if score > best.score {
			best = incomingScore{score: score, probability: probability}
		}
	}
	return best
}

func selectionsByPosition(selections []Selection) map[int]Selection {
	result := make(map[int]Selection, len(selections))
	for _, selection := range selections {
		result[selection.Position] = selection
	}
	return result
}

func candidateIndex(layer []Selection, selected Selection) int {
	for index, candidate := range layer {
		if candidate.Entry == selected.Entry && candidate.Alias == selected.Alias {
			return index
		}
	}
	return -1
}

func relativePath(root, path string) string {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return filepath.ToSlash(relative)
}
