package prosody

import (
	"fmt"
	"math"

	"utautts/internal/audio"
	"utautts/internal/frontend"
	"utautts/internal/pitch"
)

type ExtractConfig struct {
	MinDurationMS float64
	MaxDurationMS float64
	HTSLabelPath  string
}

func ExtractRecord(id, text, audioPath string, cfg ExtractConfig) (Record, error) {
	if cfg.MinDurationMS <= 0 {
		cfg.MinDurationMS = 25
	}
	if cfg.MaxDurationMS <= 0 {
		cfg.MaxDurationMS = 800
	}
	reading, err := frontend.ToKana(text)
	if err != nil {
		return Record{}, err
	}
	morae, err := frontend.ParseKana(reading)
	if err != nil {
		return Record{}, err
	}
	if len(morae) == 0 {
		return Record{}, fmt.Errorf("empty mora sequence")
	}
	pcm, err := audio.ReadWav(audioPath)
	if err != nil {
		return Record{}, err
	}
	wave := monoFloats(pcm)
	start, end := voicedRange(wave, pcm.SampleRate)
	segments := make([]labeledSegment, len(morae))
	if cfg.HTSLabelPath != "" {
		segments, err = loadHTSMoraSegments(cfg.HTSLabelPath, pcm.SampleRate)
		if err != nil {
			return Record{}, fmt.Errorf("load HTS labels: %w", err)
		}
		if len(segments) != len(morae) {
			return Record{}, fmt.Errorf("HTS labels contain %d morae, want %d", len(segments), len(morae))
		}
		for i := range segments {
			if segments[i].pause != morae[i].Pause {
				return Record{}, fmt.Errorf("HTS label pause mismatch at mora %d", i)
			}
			segments[i].start = max(0, min(len(wave), segments[i].start))
			segments[i].end = max(segments[i].start, min(len(wave), segments[i].end))
		}
		start, end = segments[0].start, segments[len(segments)-1].end
	} else {
		voicedCount := 0
		for _, mora := range morae {
			if !mora.Pause {
				voicedCount++
			}
		}
		if end-start < voicedCount*msFrames(cfg.MinDurationMS, pcm.SampleRate) {
			return Record{}, fmt.Errorf("voiced range is too short for %d tokens", len(morae))
		}
		boundaries := alignBoundaries(wave, pcm.SampleRate, start, end, len(morae), cfg.MinDurationMS)
		for i := range segments {
			segments[i] = labeledSegment{start: boundaries[i], end: boundaries[i+1], pause: morae[i].Pause}
		}
	}
	targets := make([]Target, len(morae))
	for i, mora := range morae {
		segmentStart, segmentEnd := segments[i].start, segments[i].end
		duration := framesMS(segmentEnd-segmentStart, pcm.SampleRate)
		if duration > cfg.MaxDurationMS && !mora.Pause {
			return Record{}, fmt.Errorf("token %d duration %.1fms exceeds limit", i, duration)
		}
		segment := wave[segmentStart:segmentEnd]
		targets[i] = Target{
			Position:   i,
			Mora:       mora.Text,
			Vowel:      mora.Vowel,
			Pause:      mora.Pause,
			StartMS:    framesMS(segmentStart, pcm.SampleRate),
			EndMS:      framesMS(segmentEnd, pcm.SampleRate),
			DurationMS: duration,
			F0Hz:       estimateMedianF0(segment, pcm.SampleRate),
			Energy:     rms(segment),
		}
	}
	medianF0, medianEnergy := finalizeRatios(targets)
	return Record{
		Version:      DatasetVersion,
		ID:           id,
		Text:         text,
		Reading:      reading,
		AudioPath:    audioPath,
		SampleRate:   pcm.SampleRate,
		StartMS:      framesMS(start, pcm.SampleRate),
		EndMS:        framesMS(end, pcm.SampleRate),
		MedianF0Hz:   medianF0,
		MedianEnergy: medianEnergy,
		Tokens:       targets,
	}, nil
}

func monoFloats(pcm *audio.PCM) []float64 {
	frames := len(pcm.Data) / pcm.Channels
	result := make([]float64, frames)
	for frame := 0; frame < frames; frame++ {
		sum := 0.0
		for channel := 0; channel < pcm.Channels; channel++ {
			sum += float64(pcm.Data[frame*pcm.Channels+channel]) / 32768
		}
		result[frame] = sum / float64(pcm.Channels)
	}
	return result
}

func voicedRange(wave []float64, sampleRate int) (int, int) {
	frame := max(1, msFrames(10, sampleRate))
	values := make([]float64, 0, len(wave)/frame+1)
	peak := 0.0
	for start := 0; start < len(wave); start += frame {
		end := min(len(wave), start+frame)
		value := rms(wave[start:end])
		values = append(values, value)
		peak = max(peak, value)
	}
	if peak == 0 {
		return 0, len(wave)
	}
	noiseCount := max(1, min(len(values)/10, 10))
	noise := median(append(append([]float64(nil), values[:noiseCount]...), values[len(values)-noiseCount:]...))
	threshold := max(noise*3, peak*0.035)
	first, last := 0, len(values)-1
	for first < len(values) && values[first] < threshold {
		first++
	}
	for last > first && values[last] < threshold {
		last--
	}
	padding := msFrames(20, sampleRate)
	start := max(0, first*frame-padding)
	end := min(len(wave), (last+1)*frame+padding)
	return start, end
}

func alignBoundaries(wave []float64, sampleRate, start, end, count int, minDurationMS float64) []int {
	boundaries := make([]int, count+1)
	boundaries[0], boundaries[count] = start, end
	minimum := max(1, msFrames(minDurationMS, sampleRate))
	average := float64(end-start) / float64(count)
	search := min(msFrames(100, sampleRate), int(average*0.55))
	step := max(1, msFrames(2.5, sampleRate))
	window := max(4, msFrames(10, sampleRate))
	globalRMS := rms(wave[start:end]) + 1e-9

	for index := 1; index < count; index++ {
		expected := start + int(math.Round(float64(index)*average))
		low := max(boundaries[index-1]+minimum, expected-search)
		high := min(end-(count-index)*minimum, expected+search)
		best, bestScore := max(low, min(expected, high)), math.Inf(-1)
		for candidate := low; candidate <= high; candidate += step {
			leftStart := max(start, candidate-window)
			rightEnd := min(end, candidate+window)
			if candidate-leftStart < 4 || rightEnd-candidate < 4 {
				continue
			}
			localRMS := rms(wave[leftStart:rightEnd]) / globalRMS
			correlation := normalizedCorrelation(wave[leftStart:candidate], wave[candidate:rightEnd])
			timingPenalty := math.Abs(float64(candidate-expected)) / float64(max(1, search))
			score := 0.65*(1-math.Min(localRMS, 1.5)) + 0.35*(1-correlation) - 0.25*timingPenalty
			if score > bestScore {
				best, bestScore = candidate, score
			}
		}
		boundaries[index] = best
	}
	return boundaries
}

func normalizedCorrelation(left, right []float64) float64 {
	length := min(len(left), len(right))
	if length == 0 {
		return 0
	}
	left = left[len(left)-length:]
	right = right[:length]
	numerator, leftEnergy, rightEnergy := 0.0, 0.0, 0.0
	for i := 0; i < length; i++ {
		numerator += left[i] * right[i]
		leftEnergy += left[i] * left[i]
		rightEnergy += right[i] * right[i]
	}
	return numerator / (math.Sqrt(leftEnergy*rightEnergy) + 1e-12)
}

func estimateMedianF0(segment []float64, sampleRate int) float64 {
	return pitch.EstimateMedian(segment, sampleRate)
}

func rms(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sum := 0.0
	for _, value := range values {
		sum += value * value
	}
	return math.Sqrt(sum / float64(len(values)))
}

func msFrames(ms float64, sampleRate int) int {
	return int(math.Round(ms * float64(sampleRate) / 1000))
}

func framesMS(frames, sampleRate int) float64 {
	return float64(frames) * 1000 / float64(sampleRate)
}
