package voicebank

import (
	"path/filepath"

	"utautts/internal/connection"
	"utautts/internal/frontend"
	"utautts/internal/oto"
)

type LatticeAudit struct {
	Positions               []PositionAudit `json:"positions"`
	MultiCandidatePositions int             `json:"multi_candidate_positions"`
	SameTargetPositions     int             `json:"same_target_positions"`
	HandcraftedChanges      int             `json:"handcrafted_changes_from_target_only"`
	LearnedChanges          int             `json:"learned_changes_from_target_only"`
	LearnedFromHandcrafted  int             `json:"learned_changes_from_handcrafted"`
}

type PositionAudit struct {
	Position            int              `json:"position"`
	Mora                string           `json:"mora"`
	CandidateCount      int              `json:"candidate_count"`
	TargetRange         float64          `json:"target_range"`
	TargetSelected      int              `json:"target_selected"`
	HandcraftedSelected int              `json:"handcrafted_selected"`
	LearnedSelected     int              `json:"learned_selected,omitempty"`
	Candidates          []CandidateAudit `json:"candidates"`
}

type CandidateAudit struct {
	Index                  int     `json:"index"`
	Alias                  string  `json:"alias"`
	Source                 string  `json:"source"`
	OtoPath                string  `json:"oto_path"`
	OtoLine                int     `json:"oto_line"`
	TargetScore            float64 `json:"target_score"`
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
		for candidateIndex, candidate := range layer {
			minimum = min(minimum, candidate.TargetScore)
			maximum = max(maximum, candidate.TargetScore)
			audit := CandidateAudit{
				Index: candidateIndex, Alias: candidate.Alias,
				Source:  relativePath(b.Root, candidate.Entry.Filename),
				OtoPath: relativePath(b.Root, candidate.Entry.OtoPath), OtoLine: candidate.Entry.Line,
				TargetScore: candidate.TargetScore,
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
