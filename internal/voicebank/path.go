package voicebank

import (
	"math"
	"path/filepath"

	"utautts/internal/acoustic"
	"utautts/internal/audio"
	"utautts/internal/oto"
)

type pathState struct {
	score     float64
	previous  int
	joinScore float64
}

// selectBestPaths treats every phrase as a candidate lattice. CandidateScore
// is the target cost baseline and joinScore is the pairwise concatenation
// score. Keeping those concerns separate makes it possible to replace either
// heuristic with a learned model without changing the search.
func selectBestPaths(layers [][]Selection) []Selection {
	result := make([]Selection, 0, len(layers))
	cache := boundaryFeatureCache{}
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
		result = append(result, selectPhrasePath(layers[start:end], cache)...)
		start = end
	}
	return result
}

func selectPhrasePath(layers [][]Selection, cache boundaryFeatureCache) []Selection {
	states := make([][]pathState, len(layers))
	states[0] = make([]pathState, len(layers[0]))
	for candidateIndex, candidate := range layers[0] {
		states[0][candidateIndex] = pathState{score: candidate.Score, previous: -1}
	}
	for layerIndex := 1; layerIndex < len(layers); layerIndex++ {
		states[layerIndex] = make([]pathState, len(layers[layerIndex]))
		for currentIndex, current := range layers[layerIndex] {
			best := pathState{score: math.Inf(-1), previous: -1}
			for previousIndex, previous := range layers[layerIndex-1] {
				join := joinScore(previous.Entry, current.Entry, cache)
				score := states[layerIndex-1][previousIndex].score + current.Score + join
				if score > best.score {
					best = pathState{score: score, previous: previousIndex, joinScore: join}
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
		path[layerIndex].Score += states[layerIndex][last].joinScore
		last = states[layerIndex][last].previous
	}
	return path
}

type boundaryPair struct {
	in  acoustic.Frame
	out acoustic.Frame
}

type boundaryFeatureCache map[oto.Entry]boundaryPair

func joinScore(previous, current oto.Entry, cache boundaryFeatureCache) float64 {
	previousFeatures := cache.features(previous)
	currentFeatures := cache.features(current)
	score := 0.0
	if sameSource(previous.Filename, current.Filename) && current.Offset > previous.Offset {
		// Units kept in their original recording order retain real coarticulation.
		score += 8
	}
	if !previousFeatures.out.Valid || !currentFeatures.in.Valid {
		return score
	}

	spectralDelta := acoustic.MeanSpectrumDelta(previousFeatures.out.SpectrumDB, currentFeatures.in.SpectrumDB)
	score -= math.Min(18, spectralDelta*0.8)
	score -= math.Min(6, math.Abs(previousFeatures.out.RMSDB-currentFeatures.in.RMSDB)*0.25)
	if previousFeatures.out.F0Hz > 0 && currentFeatures.in.F0Hz > 0 {
		cents := math.Abs(1200 * math.Log2(currentFeatures.in.F0Hz/previousFeatures.out.F0Hz))
		score -= math.Min(8, cents*0.015)
	} else if (previousFeatures.out.F0Hz > 0) != (currentFeatures.in.F0Hz > 0) {
		score -= 4
	}
	return score
}

func (cache boundaryFeatureCache) features(entry oto.Entry) boundaryPair {
	if value, ok := cache[entry]; ok {
		return value
	}
	value := measureBoundaryFeatures(entry)
	cache[entry] = value
	return value
}

func measureBoundaryFeatures(entry oto.Entry) boundaryPair {
	pcm, err := audio.ReadWav(entry.Filename)
	if err != nil || pcm.SampleRate <= 0 || pcm.Channels <= 0 {
		return boundaryPair{}
	}
	wave := acoustic.Mono(pcm)
	trimEndMS := float64(len(wave)) * 1000 / float64(pcm.SampleRate)
	if entry.Blank < 0 {
		trimEndMS = entry.Offset - entry.Blank
	} else {
		trimEndMS -= entry.Blank
	}
	incomingMS := entry.Offset + math.Max(0, entry.Preutterance-entry.Overlap)
	stableStartMS := entry.Offset + math.Max(entry.Fixed, entry.Preutterance)
	outgoingMS := math.Min(stableStartMS+60, trimEndMS-15)
	if outgoingMS < stableStartMS {
		outgoingMS = stableStartMS
	}
	return boundaryPair{
		in:  frameFeatures(wave, pcm.SampleRate, incomingMS),
		out: frameFeatures(wave, pcm.SampleRate, outgoingMS),
	}
}

func frameFeatures(wave []float64, sampleRate int, centerMS float64) acoustic.Frame {
	center := int(math.Round(centerMS * float64(sampleRate) / 1000))
	halfWindow := max(16, int(math.Round(0.015*float64(sampleRate))))
	start, end := center-halfWindow, center+halfWindow
	if start < 0 || end > len(wave) || end-start < 32 {
		return acoustic.Frame{}
	}
	return acoustic.AnalyzeFrame(wave[start:end], sampleRate, 10, true)
}

func sameSource(left, right string) bool {
	if left == "" || right == "" {
		return false
	}
	leftAbs, leftErr := filepath.Abs(left)
	rightAbs, rightErr := filepath.Abs(right)
	return leftErr == nil && rightErr == nil && filepath.Clean(leftAbs) == filepath.Clean(rightAbs)
}
