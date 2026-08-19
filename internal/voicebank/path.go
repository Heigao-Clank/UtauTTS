package voicebank

import (
	"math"

	"utautts/internal/connection"
	"utautts/internal/oto"
)

type pathState struct {
	score           float64
	previous        int
	joinScore       float64
	joinProbability float64
}

// selectBestPathsは各フレーズを候補ラティスとして扱う。CandidateScoreは
// ターゲットコストの基準、joinScoreはペア間の連結スコア。両者を分離することで、
// 探索を変更せずにどちらかのヒューリスティックを学習モデルに置き換えられる。
func selectBestPaths(layers [][]Selection, mode SelectionMode, model *connection.LearnedModel, extractor *connection.Extractor) []Selection {
	result := make([]Selection, 0, len(layers))
	cache := extractor
	if cache == nil {
		cache = connection.NewExtractor()
	}
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
		phrase := layers[start:end]
		switch mode {
		case SelectionGreedy:
			result = append(result, selectGreedyPath(phrase, cache, model, true)...)
		case SelectionTargetOnly:
			result = append(result, selectGreedyPath(phrase, cache, nil, false)...)
		default:
			result = append(result, selectPhrasePath(phrase, cache, model)...)
		}
		start = end
	}
	return result
}

func selectPhrasePath(layers [][]Selection, cache *connection.Extractor, model *connection.LearnedModel) []Selection {
	states := make([][]pathState, len(layers))
	states[0] = make([]pathState, len(layers[0]))
	for candidateIndex, candidate := range layers[0] {
		states[0][candidateIndex] = pathState{score: candidate.TargetScore, previous: -1}
	}
	for layerIndex := 1; layerIndex < len(layers); layerIndex++ {
		states[layerIndex] = make([]pathState, len(layers[layerIndex]))
		for currentIndex, current := range layers[layerIndex] {
			best := pathState{score: math.Inf(-1), previous: -1}
			for previousIndex, previous := range layers[layerIndex-1] {
				join, probability := pairScore(previous.Entry, current.Entry, cache, model)
				score := states[layerIndex-1][previousIndex].score + current.TargetScore + join
				if score > best.score {
					best = pathState{score: score, previous: previousIndex, joinScore: join, joinProbability: probability}
				}
			}
			states[layerIndex][currentIndex] = best
		}
	}

	last := 0
	for candidateIndex := 1; candidateIndex < len(states[len(states)-1]); candidateIndex++ {
		if states[len(states)-1][candidateIndex].score > states[len(states)-1][last].score {
			last = candidateIndex
		}
	}
	path := make([]Selection, len(layers))
	for layerIndex := len(layers) - 1; layerIndex >= 0; layerIndex-- {
		path[layerIndex] = layers[layerIndex][last]
		path[layerIndex].JoinScore = states[layerIndex][last].joinScore
		path[layerIndex].JoinProbability = states[layerIndex][last].joinProbability
		path[layerIndex].PathScore = states[layerIndex][last].score
		last = states[layerIndex][last].previous
	}
	return path
}

func selectGreedyPath(layers [][]Selection, cache *connection.Extractor, model *connection.LearnedModel, useJoin bool) []Selection {
	path := make([]Selection, 0, len(layers))
	pathScore := 0.0
	for layerIndex, layer := range layers {
		bestIndex := 0
		bestJoin := 0.0
		bestProbability := 0.0
		bestLocal := math.Inf(-1)
		for candidateIndex, candidate := range layer {
			join, probability := 0.0, 0.0
			if useJoin && layerIndex > 0 {
				join, probability = pairScore(path[layerIndex-1].Entry, candidate.Entry, cache, model)
			}
			local := candidate.TargetScore + join
			if local > bestLocal {
				bestIndex, bestJoin, bestProbability, bestLocal = candidateIndex, join, probability, local
			}
		}
		selected := layer[bestIndex]
		selected.JoinScore = bestJoin
		selected.JoinProbability = bestProbability
		pathScore += bestLocal
		selected.PathScore = pathScore
		path = append(path, selected)
	}
	return path
}

func joinScore(previous, current oto.Entry, cache *connection.Extractor) float64 {
	return connection.HandcraftedScore(cache.Pair(previous, current))
}

func pairScore(previous, current oto.Entry, cache *connection.Extractor, model *connection.LearnedModel) (float64, float64) {
	features := cache.Pair(previous, current)
	if model != nil {
		return connection.LearnedScore(features, model)
	}
	return connection.HandcraftedScore(features), 0
}
