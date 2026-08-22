package voicebank

import (
	"math"
	"path/filepath"
	"sort"

	"utautts/internal/frontend"
)

type ContinuityAudit struct {
	TargetMoraMS float64             `json:"target_mora_ms"`
	SafeRatio    float64             `json:"safe_ratio"`
	Positions    int                 `json:"positions"`
	Boundaries   int                 `json:"boundaries"`
	Current      ContinuityPathAudit `json:"current"`
	Maximum      ContinuityPathAudit `json:"maximum"`
}

type ContinuityPathAudit struct {
	ContinuousBoundaries int             `json:"continuous_boundaries"`
	BoundaryCoverage     float64         `json:"boundary_coverage"`
	SafeBoundaries       int             `json:"safe_boundaries"`
	SafeCoverage         float64         `json:"safe_coverage"`
	MedianCompression    float64         `json:"median_compression,omitempty"`
	P90Compression       float64         `json:"p90_compression,omitempty"`
	Runs                 []ContinuityRun `json:"runs,omitempty"`
	Runs2                int             `json:"runs_2"`
	Runs3                int             `json:"runs_3"`
	Runs4Plus            int             `json:"runs_4_plus"`
}

type ContinuityRun struct {
	StartPosition  int     `json:"start_position"`
	Units          int     `json:"units"`
	Source         string  `json:"source"`
	MaxCompression float64 `json:"max_compression"`
}

func (b *Bank) AuditContinuity(morae []frontend.Mora, cfg ResolveConfig, targetMoraMS, safeRatio float64) (*ContinuityAudit, error) {
	if targetMoraMS <= 0 {
		targetMoraMS = 140
	}
	if safeRatio <= 0 {
		safeRatio = 4
	}
	current, err := b.ResolveWithConfig(morae, cfg)
	if err != nil {
		return nil, err
	}
	policy := cfg.AliasPolicy
	if policy == "" {
		policy = AliasPolicyAuto
	}
	layers, err := b.candidateLayersWithPolicyMode(morae, cfg.Tone, cfg.Color, policy, cfg.AcousticMode)
	if err != nil {
		return nil, err
	}
	maximum := selectMaximumContinuity(layers, cfg.AcousticMode)
	boundaries := countBoundaries(current)
	return &ContinuityAudit{
		TargetMoraMS: targetMoraMS,
		SafeRatio:    safeRatio,
		Positions:    len(current),
		Boundaries:   boundaries,
		Current:      summarizeContinuity(current, boundaries, targetMoraMS, safeRatio, b.Root),
		Maximum:      summarizeContinuity(maximum, boundaries, targetMoraMS, safeRatio, b.Root),
	}, nil
}

type continuityState struct {
	edges    int
	quality  float64
	previous int
}

func selectMaximumContinuity(layers [][]Selection, acousticMode string) []Selection {
	result := make([]Selection, 0, len(layers))
	for start := 0; start < len(layers); {
		for start < len(layers) && len(layers[start]) == 0 {
			start++
		}
		if start == len(layers) {
			break
		}
		end := start + 1
		for end < len(layers) && len(layers[end]) > 0 {
			end++
		}
		result = append(result, selectMaximumContinuityPhrase(layers[start:end], acousticMode)...)
		start = end
	}
	return result
}

func selectMaximumContinuityPhrase(layers [][]Selection, acousticMode string) []Selection {
	states := make([][]continuityState, len(layers))
	states[0] = make([]continuityState, len(layers[0]))
	for index, candidate := range layers[0] {
		states[0][index] = continuityState{quality: localCandidateScore(candidate, acousticMode), previous: -1}
	}
	for layerIndex := 1; layerIndex < len(layers); layerIndex++ {
		states[layerIndex] = make([]continuityState, len(layers[layerIndex]))
		for currentIndex, current := range layers[layerIndex] {
			best := continuityState{edges: -1, quality: math.Inf(-1), previous: -1}
			for previousIndex, previous := range layers[layerIndex-1] {
				candidate := continuityState{
					edges:    states[layerIndex-1][previousIndex].edges,
					quality:  states[layerIndex-1][previousIndex].quality + localCandidateScore(current, acousticMode),
					previous: previousIndex,
				}
				if continuousAnchorBoundary(previous, current) {
					candidate.edges++
				}
				if betterContinuityState(candidate, best) {
					best = candidate
				}
			}
			states[layerIndex][currentIndex] = best
		}
	}
	last := 0
	for index := 1; index < len(states[len(states)-1]); index++ {
		if betterContinuityState(states[len(states)-1][index], states[len(states)-1][last]) {
			last = index
		}
	}
	path := make([]Selection, len(layers))
	for layerIndex := len(layers) - 1; layerIndex >= 0; layerIndex-- {
		path[layerIndex] = layers[layerIndex][last]
		last = states[layerIndex][last].previous
	}
	return path
}

func betterContinuityState(left, right continuityState) bool {
	return left.edges > right.edges || left.edges == right.edges && left.quality > right.quality
}

func summarizeContinuity(path []Selection, boundaries int, targetMoraMS, safeRatio float64, root string) ContinuityPathAudit {
	result := ContinuityPathAudit{}
	var ratios []float64
	for index := 1; index < len(path); index++ {
		if path[index].Position != path[index-1].Position+1 || !continuousAnchorBoundary(path[index-1], path[index]) {
			continue
		}
		ratio := anchorDistanceMS(path[index-1], path[index]) / targetMoraMS
		result.ContinuousBoundaries++
		ratios = append(ratios, ratio)
		if ratio <= safeRatio {
			result.SafeBoundaries++
		}
	}
	if boundaries > 0 {
		result.BoundaryCoverage = float64(result.ContinuousBoundaries) / float64(boundaries)
		result.SafeCoverage = float64(result.SafeBoundaries) / float64(boundaries)
	}
	if len(ratios) > 0 {
		sort.Float64s(ratios)
		result.MedianCompression = percentile(ratios, 0.5)
		result.P90Compression = percentile(ratios, 0.9)
	}
	for start := 0; start < len(path); {
		end := start + 1
		maxCompression := 0.0
		for end < len(path) && path[end].Position == path[end-1].Position+1 && continuousAnchorBoundary(path[end-1], path[end]) {
			maxCompression = max(maxCompression, anchorDistanceMS(path[end-1], path[end])/targetMoraMS)
			end++
		}
		if end-start >= 2 {
			run := ContinuityRun{
				StartPosition:  path[start].Position,
				Units:          end - start,
				Source:         relativePath(root, path[start].Entry.Filename),
				MaxCompression: maxCompression,
			}
			result.Runs = append(result.Runs, run)
			switch run.Units {
			case 2:
				result.Runs2++
			case 3:
				result.Runs3++
			default:
				result.Runs4Plus++
			}
		}
		start = end
	}
	return result
}

func countBoundaries(path []Selection) int {
	count := 0
	for index := 1; index < len(path); index++ {
		if path[index].Position == path[index-1].Position+1 {
			count++
		}
	}
	return count
}

func continuousAnchorBoundary(previous, current Selection) bool {
	if previous.Kind != AliasVCV || current.Kind != AliasVCV || previous.Composite || current.Composite {
		return false
	}
	if previous.Entry.Filename == "" || current.Entry.Filename == "" {
		return false
	}
	if !samePath(previous.Entry.Filename, current.Entry.Filename) || !samePath(previous.Entry.OtoPath, current.Entry.OtoPath) {
		return false
	}
	return anchorDistanceMS(previous, current) > 0
}

func anchorDistanceMS(previous, current Selection) float64 {
	return current.Entry.Offset + current.Entry.Preutterance - previous.Entry.Offset - previous.Entry.Preutterance
}

func samePath(left, right string) bool {
	return filepath.Clean(left) == filepath.Clean(right)
}

func percentile(sorted []float64, fraction float64) float64 {
	if len(sorted) == 1 {
		return sorted[0]
	}
	position := fraction * float64(len(sorted)-1)
	lower := int(math.Floor(position))
	upper := int(math.Ceil(position))
	if lower == upper {
		return sorted[lower]
	}
	weight := position - float64(lower)
	return sorted[lower]*(1-weight) + sorted[upper]*weight
}
