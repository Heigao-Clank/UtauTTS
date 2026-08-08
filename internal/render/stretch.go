package render

import "math"

func stretchPreservingPrefix(source []float64, targetFrames, prefixFrames, sampleRate int) []float64 {
	if targetFrames <= 0 || len(source) == 0 {
		return nil
	}
	if targetFrames == len(source) {
		return append([]float64(nil), source...)
	}
	prefixFrames = min(prefixFrames, len(source), targetFrames)
	if prefixFrames < 0 {
		prefixFrames = 0
	}
	result := make([]float64, targetFrames)
	copy(result, source[:prefixFrames])
	remainingTarget := targetFrames - prefixFrames
	if remainingTarget == 0 {
		return result
	}
	remainingSource := source[prefixFrames:]
	if len(remainingSource) < 2 {
		remainingSource = source
	}
	stretched := wsola(remainingSource, remainingTarget, sampleRate)
	copy(result[prefixFrames:], stretched)

	// Smooth only the protected/unprotected boundary; the protected samples
	// themselves remain unchanged before this short transition.
	crossfade := min(msToFrames(3, sampleRate), prefixFrames, remainingTarget)
	for i := 0; i < crossfade; i++ {
		position := prefixFrames + i
		before := source[min(prefixFrames+i, len(source)-1)]
		alpha := float64(i+1) / float64(crossfade+1)
		result[position] = before*(1-alpha) + result[position]*alpha
	}
	return result
}

func retimeWithCompressedPrefix(source []float64, targetFrames, sourcePrefixFrames, targetPrefixFrames, sampleRate int) []float64 {
	if targetFrames <= 0 || len(source) == 0 {
		return nil
	}
	sourcePrefixFrames = max(0, min(sourcePrefixFrames, len(source)))
	targetPrefixFrames = max(0, min(targetPrefixFrames, targetFrames))
	if sourcePrefixFrames == targetPrefixFrames {
		return stretchPreservingPrefix(source, targetFrames, sourcePrefixFrames, sampleRate)
	}

	tailFrames := targetFrames - targetPrefixFrames
	crossfade := min(msToFrames(4, sampleRate), targetPrefixFrames, tailFrames, sourcePrefixFrames, len(source)-sourcePrefixFrames)
	if crossfade < 2 {
		result := make([]float64, targetFrames)
		copy(result[:targetPrefixFrames], retimeRegion(source[:sourcePrefixFrames], targetPrefixFrames, sampleRate))
		copy(result[targetPrefixFrames:], retimeRegion(source[sourcePrefixFrames:], tailFrames, sampleRate))
		return result
	}

	// Both regions include samples on the other side of the landmark. Blending
	// this shared source neighborhood avoids a phase discontinuity at the join.
	prefix := retimeRegion(source[:sourcePrefixFrames+crossfade], targetPrefixFrames+crossfade, sampleRate)
	tail := retimeRegion(source[sourcePrefixFrames-crossfade:], tailFrames+crossfade, sampleRate)
	overlap := crossfade * 2
	prefixOnly := targetPrefixFrames - crossfade
	result := make([]float64, targetFrames)
	copy(result[:prefixOnly], prefix[:prefixOnly])
	for i := 0; i < overlap; i++ {
		alpha := 0.5 - 0.5*math.Cos(math.Pi*float64(i+1)/float64(overlap+1))
		result[prefixOnly+i] = prefix[prefixOnly+i]*(1-alpha) + tail[i]*alpha
	}
	copy(result[targetPrefixFrames+crossfade:], tail[overlap:])
	declickJoin(result, targetPrefixFrames, msToFrames(2, sampleRate))
	return result
}

func retimeRegion(source []float64, targetFrames, sampleRate int) []float64 {
	if targetFrames <= 0 {
		return nil
	}
	if len(source) == 0 {
		return make([]float64, targetFrames)
	}
	if len(source) == targetFrames {
		return append([]float64(nil), source...)
	}
	return wsola(source, targetFrames, sampleRate)
}

func declickJoin(wave []float64, position, radius int) {
	if position <= 0 || position >= len(wave) || radius < 1 {
		return
	}
	left := max(0, position-radius)
	right := min(len(wave)-1, position+radius)
	if right-left < 3 {
		return
	}
	localDelta := 0.0
	count := 0
	for i := left + 1; i <= right; i++ {
		if i == position {
			continue
		}
		localDelta += math.Abs(wave[i] - wave[i-1])
		count++
	}
	localDelta /= float64(max(1, count))
	if math.Abs(wave[position]-wave[position-1]) <= math.Max(0.08, localDelta*4) {
		return
	}
	start, end := wave[left], wave[right]
	for i := left + 1; i < right; i++ {
		alpha := float64(i-left) / float64(right-left)
		wave[i] = start*(1-alpha) + end*alpha
	}
}

// wsola is a compact waveform-similarity overlap-add implementation. It is
// deterministic and preserves local periodicity better than linear resampling.
func wsola(source []float64, targetFrames, sampleRate int) []float64 {
	if targetFrames <= 0 || len(source) == 0 {
		return nil
	}
	if len(source) < 16 || targetFrames < 16 {
		return linearResample(source, targetFrames)
	}
	window := min(msToFrames(40, sampleRate), len(source), targetFrames)
	if window < 16 {
		return linearResample(source, targetFrames)
	}
	if window%2 == 1 {
		window--
	}
	synthesisHop := max(1, window/2)
	search := max(1, min(msToFrames(5, sampleRate), window/4))
	ratio := float64(len(source)) / float64(targetFrames)

	accumulator := make([]float64, targetFrames+window)
	weights := make([]float64, len(accumulator))
	previousSource := 0
	for outputPosition := 0; outputPosition < targetFrames; outputPosition += synthesisHop {
		expected := int(math.Round(float64(outputPosition) * ratio))
		maxStart := max(0, len(source)-window)
		expected = min(expected, maxStart)
		start := expected
		if outputPosition > 0 {
			start = bestMatch(source, previousSource+synthesisHop, expected, search, window/2, maxStart)
		}
		for i := 0; i < window && outputPosition+i < len(accumulator); i++ {
			weight := 0.5 - 0.5*math.Cos(2*math.Pi*float64(i+1)/float64(window+1))
			accumulator[outputPosition+i] += source[start+i] * weight
			weights[outputPosition+i] += weight
		}
		previousSource = start
	}

	result := make([]float64, targetFrames)
	for i := range result {
		if weights[i] > 1e-12 {
			result[i] = accumulator[i] / weights[i]
		}
	}
	return result
}

func bestMatch(source []float64, reference, expected, search, compare, maxStart int) int {
	reference = max(0, min(reference, maxStart))
	low := max(0, expected-search)
	high := min(maxStart, expected+search)
	best := expected
	bestScore := math.Inf(-1)
	for candidate := low; candidate <= high; candidate++ {
		length := min(compare, len(source)-reference, len(source)-candidate)
		if length < 4 {
			continue
		}
		numerator, leftEnergy, rightEnergy := 0.0, 0.0, 0.0
		for i := 0; i < length; i++ {
			left := source[reference+i]
			right := source[candidate+i]
			numerator += left * right
			leftEnergy += left * left
			rightEnergy += right * right
		}
		denominator := math.Sqrt(leftEnergy*rightEnergy) + 1e-12
		score := numerator / denominator
		if score > bestScore {
			bestScore = score
			best = candidate
		}
	}
	return best
}

func linearResample(source []float64, targetFrames int) []float64 {
	if targetFrames <= 0 || len(source) == 0 {
		return nil
	}
	if len(source) == 1 {
		result := make([]float64, targetFrames)
		for i := range result {
			result[i] = source[0]
		}
		return result
	}
	result := make([]float64, targetFrames)
	for i := range result {
		position := float64(i) * float64(len(source)-1) / float64(max(1, targetFrames-1))
		left := int(math.Floor(position))
		right := min(left+1, len(source)-1)
		fraction := position - float64(left)
		result[i] = source[left]*(1-fraction) + source[right]*fraction
	}
	return result
}
