package render

import (
	"fmt"
	"math"

	"utautts/internal/audio"
	"utautts/internal/pitch"
	"utautts/internal/plan"
)

const (
	openUtauPitchVowelGuardMS = 12.0
	openUtauPitchFadeMS       = 10.0
	openUtauPitchAlignMS      = 18.0
	openUtauPitchSearchMS     = 8.0
	openUtauPitchMinCorr      = 0.20
	openUtauPitchMinRMSRatio  = 0.65
	openUtauPitchMaxRMSRatio  = 1.60
)

// renderWaveformOpenUtauPitch keeps the ordinary waveform renderer as the
// authoritative signal. The OpenUtau-compatible branch is only exposed inside
// stable vowel regions; consonants, unvoiced frames, attacks, and handoffs stay
// byte-for-byte derived from the waveform branch.
func renderWaveformOpenUtauPitch(synthesisPlan *plan.Plan, cfg Config) (*audio.PCM, error) {
	return renderWaveformOpenUtauPitchMode(synthesisPlan, cfg, false, false)
}
func renderWaveformOpenUtauPitchLocal(synthesisPlan *plan.Plan, cfg Config) (*audio.PCM, error) {
	return renderWaveformOpenUtauPitchMode(synthesisPlan, cfg, true, false)
}

func renderWaveformOpenUtauPitchLocalDual(synthesisPlan *plan.Plan, cfg Config) (*audio.PCM, error) {
	return renderWaveformOpenUtauPitchMode(synthesisPlan, cfg, true, true)
}

func renderWaveformOpenUtauPitchLocalDualSmooth(synthesisPlan *plan.Plan, cfg Config) (*audio.PCM, error) {
	// The model already applies a 20 ms Gaussian. An additional 55 ms gives
	// roughly 60 ms total sigma while the slew constraint removes the sharp
	// corners most correlated with the reported fast tremor.
	cfg.PitchCurve = smoothAndLimitPitchCurve(cfg.PitchCurve, 55, 4)
	return renderWaveformOpenUtauPitchMode(synthesisPlan, cfg, true, true)
}

func smoothAndLimitPitchCurve(curve *PitchCurve, sigmaMS, maxCentsPer10MS float64) *PitchCurve {
	if curve == nil || curve.FrameMS <= 0 || len(curve.Cents) == 0 {
		return curve
	}
	result := &PitchCurve{FrameMS: curve.FrameMS, Cents: append([]float64(nil), curve.Cents...)}
	if sigmaMS > 0 {
		sigma := sigmaMS / curve.FrameMS
		radius := max(1, int(math.Ceil(3*sigma)))
		weights := make([]float64, 2*radius+1)
		for offset := -radius; offset <= radius; offset++ {
			weights[offset+radius] = math.Exp(-0.5 * math.Pow(float64(offset)/sigma, 2))
		}
		smoothed := make([]float64, len(result.Cents))
		for position := range result.Cents {
			sum, weightSum := 0.0, 0.0
			for offset := -radius; offset <= radius; offset++ {
				source := max(0, min(len(result.Cents)-1, position+offset))
				weight := weights[offset+radius]
				sum += result.Cents[source] * weight
				weightSum += weight
			}
			smoothed[position] = sum / weightSum
		}
		result.Cents = smoothed
	}
	maximumStep := maxCentsPer10MS * curve.FrameMS / 10
	if maximumStep > 0 {
		for position := 1; position < len(result.Cents); position++ {
			result.Cents[position] = math.Max(result.Cents[position-1]-maximumStep, math.Min(result.Cents[position-1]+maximumStep, result.Cents[position]))
		}
		for position := len(result.Cents) - 2; position >= 0; position-- {
			result.Cents[position] = math.Max(result.Cents[position+1]-maximumStep, math.Min(result.Cents[position+1]+maximumStep, result.Cents[position]))
		}
	}
	return result
}

// ConstrainPitchCurve applies the same conservative smoothing and slew limit
// used by the experimental pitch renderer to externally edited contours.
// Keeping this in render makes manual GUI/CLI edits obey the same artifact
// guardrails as learned contours.
func ConstrainPitchCurve(curve *PitchCurve, sigmaMS, maxCentsPer10MS float64) *PitchCurve {
	return smoothAndLimitPitchCurve(curve, sigmaMS, maxCentsPer10MS)
}

// restoreRawHighBand computes raw + lowpass(processed-raw). Frequencies above
// the cutoff therefore come only from raw, while the pitch-bearing low band
// comes from the continuously processed phrase. The centered FIR is zero phase
// because rendering is offline; identical inputs reconstruct exactly.
func restoreRawHighBand(raw, processed *audio.PCM, cutoffHz float64, taps int) *audio.PCM {
	if raw == nil || processed == nil || raw.SampleRate <= 0 || raw.SampleRate != processed.SampleRate ||
		raw.Channels != 1 || processed.Channels != 1 || len(raw.Data) != len(processed.Data) || cutoffHz <= 0 {
		return processed
	}
	if taps < 3 {
		taps = 3
	}
	if taps%2 == 0 {
		taps++
	}
	cutoff := math.Min(cutoffHz, 0.45*float64(raw.SampleRate)) / float64(raw.SampleRate)
	half := taps / 2
	kernel := make([]float64, taps)
	sum := 0.0
	for index := range kernel {
		offset := index - half
		value := 2 * cutoff
		if offset != 0 {
			value = math.Sin(2*math.Pi*cutoff*float64(offset)) / (math.Pi * float64(offset))
		}
		window := 0.42 - 0.5*math.Cos(2*math.Pi*float64(index)/float64(taps-1)) +
			0.08*math.Cos(4*math.Pi*float64(index)/float64(taps-1))
		kernel[index] = value * window
		sum += kernel[index]
	}
	for index := range kernel {
		kernel[index] /= sum
	}
	lowRaw := make([]float64, len(raw.Data))
	lowProcessed := make([]float64, len(raw.Data))
	for position := range raw.Data {
		// The windowed-sinc kernel is symmetric. Visit the left/right tap
		// pair in the same order as the original full traversal, preserving
		// its floating-point result while nearly halving the inner-loop work.
		for offset := half; offset > 0; offset-- {
			weight := kernel[half-offset]
			left := position - offset
			right := position + offset
			switch {
			case left >= 0 && right < len(raw.Data):
				lowRaw[position] += weight * (float64(raw.Data[left]) + float64(raw.Data[right]))
				lowProcessed[position] += weight * (float64(processed.Data[left]) + float64(processed.Data[right]))
			case left >= 0:
				lowRaw[position] += weight * float64(raw.Data[left])
				lowProcessed[position] += weight * float64(processed.Data[left])
			case right < len(raw.Data):
				lowRaw[position] += weight * float64(raw.Data[right])
				lowProcessed[position] += weight * float64(processed.Data[right])
			}
		}
		weight := kernel[half]
		lowRaw[position] += weight * float64(raw.Data[position])
		lowProcessed[position] += weight * float64(processed.Data[position])
	}
	rawEnergy, processedEnergy := 0.0, 0.0
	for index := range lowRaw {
		rawEnergy += lowRaw[index] * lowRaw[index]
		processedEnergy += lowProcessed[index] * lowProcessed[index]
	}
	gain := 1.0
	if rawEnergy > 1 && processedEnergy > 1 {
		gain = math.Max(0.8, math.Min(1.25, math.Sqrt(rawEnergy/processedEnergy)))
	}
	result := append([]int16(nil), raw.Data...)
	for position := range result {
		value := float64(raw.Data[position]) - lowRaw[position] + gain*lowProcessed[position]
		value = math.Max(-32768, math.Min(32767, value))
		result[position] = int16(math.Round(value))
	}
	return &audio.PCM{SampleRate: raw.SampleRate, Channels: 1, Data: result}
}

func renderWaveformOpenUtauPitchMode(synthesisPlan *plan.Plan, cfg Config, localSourcePitch, alignBothEnds bool) (*audio.PCM, error) {
	baseCfg := cfg
	baseCfg.Backend = "waveform"
	baseCfg.ApplyPitch = false
	baseCfg.IntonationStrength = 0
	baseCfg.PitchCurve = nil
	base, err := renderWaveform(synthesisPlan, baseCfg)
	if err != nil {
		return nil, err
	}
	// This is a hard invariant, not merely a short crossfade: a flat target
	// never starts the external bridge and returns the exact waveform PCM.
	if !planHasPitchShift(synthesisPlan, cfg) {
		return base, nil
	}

	var pitched *audio.PCM
	if localSourcePitch {
		pitched, err = renderOpenUtauClassicWorldlineLocalPitch(synthesisPlan, cfg)
	} else {
		pitched, err = renderOpenUtauClassicWorldline(synthesisPlan, cfg)
	}
	if err != nil {
		return nil, err
	}
	if base.Channels != 1 || pitched.Channels != 1 || base.SampleRate != pitched.SampleRate {
		return nil, fmt.Errorf("waveform/OpenUtau pitch format mismatch")
	}

	baseline := pcmFloats(base.Data)
	processed := pcmFloats(pitched.Data)
	result := append([]int16(nil), base.Data...)
	regions := openUtauPitchRegions(synthesisPlan.Units, cfg.ReleaseMS, base.SampleRate, baseline, processed)
	for _, region := range regions {
		start, end := region.start, region.end
		lag, correlation := bestBranchLag(baseline, processed, start, end, base.SampleRate, false)
		if alignBothEnds && region.fadeOut {
			// Keep one lag for the whole vowel (no sample drops/duplication), but
			// choose it from the worse of entry and exit correlations. Entry-only
			// alignment leaves an arbitrary phase mismatch at every protected
			// phone boundary, heard as a repeated fast tremor during fade-out.
			lag, correlation = bestConstantBranchLag(baseline, processed, start, end, base.SampleRate)
		}
		if correlation < openUtauPitchMinCorr {
			continue
		}
		aligned := make([]float64, end-start)
		for frame := start; frame < end; frame++ {
			source := max(0, min(len(processed)-1, frame+lag))
			aligned[frame-start] = processed[source]
		}
		// Recheck the exact samples that will be exposed. Alignment can move a
		// dropout by up to openUtauPitchSearchMS across a region boundary.
		if !processedRegionRetainsSignal(baseline[start:end], aligned, 0, len(aligned), base.SampleRate) {
			continue
		}
		aligned = matchLevelAndMean(aligned, baseline[start:end])
		fade := min(msToFrames(openUtauPitchFadeMS, base.SampleRate), (end-start)/3)
		for frame := start; frame < end; frame++ {
			weight := 1.0
			if fade > 0 && frame-start < fade {
				weight = smoothstep(float64(frame-start) / float64(fade))
			}
			if region.fadeOut && fade > 0 && end-1-frame < fade {
				weight = math.Min(weight, smoothstep(float64(end-1-frame)/float64(fade)))
			}
			if weight <= 0 {
				continue
			}
			mixed := float64(base.Data[frame])*(1-weight) + aligned[frame-start]*32768*weight
			mixed = math.Max(-32768, math.Min(32767, mixed))
			result[frame] = int16(math.Round(mixed))
		}
	}
	return &audio.PCM{SampleRate: base.SampleRate, Channels: 1, Data: result}, nil
}

type pitchRegion struct {
	start   int
	end     int
	fadeOut bool
}

// openUtauPitchRegions builds guarded vowel-only regions. Every phone boundary
// remains waveform: even a periodic, energetic processed gap can contain the
// wrong voiced phone and was heard as missing speech in results17.
func openUtauPitchRegions(units []plan.Unit, releaseMS float64, sampleRate int, baseline, processed []float64) []pitchRegion {
	length := min(len(baseline), len(processed))
	if sampleRate <= 0 || length == 0 {
		return nil
	}
	var regions []pitchRegion
	lastUnit := -1
	for unitIndex, unit := range units {
		if unit.Silent {
			continue
		}
		start, end, ok := stableVowelFrames(units, unitIndex, releaseMS, sampleRate, length, length)
		if !ok || pitch.EstimateMedian(baseline[start:end], sampleRate) <= 0 {
			continue
		}
		// The gap guard below cannot protect a vowel that the processed branch
		// itself has attenuated or dropped. Reject the whole candidate instead
		// of patching individual windows: extra raw/processed switches inside a
		// vowel recreate the pitch wobble this renderer is meant to avoid.
		if !processedRegionRetainsSignal(baseline, processed, start, end, sampleRate) {
			continue
		}
		lastUnit = unitIndex
		candidate := pitchRegion{start: start, end: end, fadeOut: true}
		if len(regions) > 0 {
			previous := &regions[len(regions)-1]
			// Merge only genuinely overlapping vowel interiors. Never bridge a
			// positive gap across a phone boundary using the processed branch.
			if candidate.start <= previous.end {
				previous.end = max(previous.end, candidate.end)
				continue
			}
		}
		regions = append(regions, candidate)
	}
	if len(regions) > 0 && lastUnit >= 0 && noLaterAudibleUnit(units, lastUnit) {
		last := &regions[len(regions)-1]
		if processedReleaseRetainsSignal(baseline, processed, last.end, length, sampleRate) {
			// Do not switch back to the stretched raw vowel during a healthy
			// release. The processed branch already contains its own fade.
			last.end = length
			last.fadeOut = false
		}
	}
	return regions
}

func processedRegionRetainsSignal(baseline, processed []float64, start, end, sampleRate int) bool {
	start = max(0, start)
	end = min(len(baseline), len(processed), end)
	if sampleRate <= 0 || end <= start {
		return false
	}
	// A whole-region RMS hides a short missing phone. Scan densely and require
	// every window in the candidate vowel to preserve enough of the waveform
	// branch's energy. Being conservative is intentional: a false rejection
	// only removes pitch movement, while a false acceptance removes speech.
	window := min(end-start, max(16, msToFrames(12, sampleRate)))
	hop := max(1, msToFrames(3, sampleRate))
	check := func(frameStart int) bool {
		frameEnd := min(end, frameStart+window)
		frameStart = max(start, frameEnd-window)
		baseRMS := frameRMS(baseline[frameStart:frameEnd])
		branchRMS := frameRMS(processed[frameStart:frameEnd])
		return baseRMS >= 0.003 && branchRMS >= baseRMS*openUtauPitchMinRMSRatio && branchRMS <= baseRMS*openUtauPitchMaxRMSRatio
	}
	for frameStart := start; frameStart+window <= end; frameStart += hop {
		if !check(frameStart) {
			return false
		}
	}
	return check(end - window)
}

func processedReleaseRetainsSignal(baseline, processed []float64, start, end, sampleRate int) bool {
	start = max(0, start)
	end = min(len(baseline), len(processed), end)
	if sampleRate <= 0 || end <= start {
		return true
	}
	window := min(end-start, max(16, msToFrames(12, sampleRate)))
	hop := max(1, msToFrames(3, sampleRate))
	check := func(frameStart int) bool {
		frameEnd := min(end, frameStart+window)
		frameStart = max(start, frameEnd-window)
		baseRMS := frameRMS(baseline[frameStart:frameEnd])
		// Silence at the natural end needs no retention check.
		branchRMS := frameRMS(processed[frameStart:frameEnd])
		return baseRMS < 0.003 || (branchRMS >= baseRMS*openUtauPitchMinRMSRatio && branchRMS <= baseRMS*openUtauPitchMaxRMSRatio)
	}
	for frameStart := start; frameStart+window <= end; frameStart += hop {
		if !check(frameStart) {
			return false
		}
	}
	return check(end - window)
}

func noLaterAudibleUnit(units []plan.Unit, index int) bool {
	for next := index + 1; next < len(units); next++ {
		if !units[next].Silent {
			return false
		}
	}
	return true
}

func planHasPitchShift(synthesisPlan *plan.Plan, cfg Config) bool {
	if pitchCurveHasShift(cfg.PitchCurve) || cfg.IntonationStrength > 0 {
		return true
	}
	for _, unit := range synthesisPlan.Units {
		factor := unit.PitchFactor
		if factor <= 0 {
			factor = 1
		}
		if math.Abs(factor-1) > 0.0001 {
			return true
		}
	}
	return false
}

func unitHasOpenUtauPitchShift(unit plan.Unit, curve *PitchCurve) bool {
	factor := unit.PitchFactor
	if factor <= 0 {
		factor = 1
	}
	intonation := unit.IntonationFactor
	if intonation <= 0 {
		intonation = 1
	}
	if math.Abs(factor*intonation-1) > 0.0001 {
		return true
	}
	start := math.Max(0, unit.NoteStartMS)
	end := math.Max(start, unit.NoteStartMS+unit.DurationMS)
	for timeMS := start; timeMS <= end; timeMS += math.Max(1, curveFrameMS(curve)) {
		if math.Abs(pitchCurveFactorAt(curve, timeMS)-1) > 0.0001 {
			return true
		}
	}
	return false
}

func curveFrameMS(curve *PitchCurve) float64 {
	if curve == nil || curve.FrameMS <= 0 {
		return 5
	}
	return curve.FrameMS
}

func stableVowelFrames(units []plan.Unit, unitIndex int, releaseMS float64, sampleRate, baseLength, processedLength int) (int, int, bool) {
	if unitIndex < 0 || unitIndex >= len(units) {
		return 0, 0, false
	}
	unit := units[unitIndex]
	timing := normalizeTiming(unit, releaseMS)
	unitStartMS := unit.NoteStartMS - timing.preutteranceMS
	startMS := unitStartMS + timing.consonantMS + openUtauPitchVowelGuardMS
	endMS := unit.NoteStartMS + unit.DurationMS - openUtauPitchVowelGuardMS
	if unitIndex+1 < len(units) {
		next := units[unitIndex+1]
		if !next.Silent {
			nextTiming := normalizeTiming(next, releaseMS)
			endMS = math.Min(endMS, next.NoteStartMS-nextTiming.preutteranceMS)
		}
	}
	start := max(0, msToFramesSigned(startMS, sampleRate))
	end := min(baseLength, processedLength, msToFramesSigned(endMS, sampleRate))
	minimum := msToFrames(2*openUtauPitchFadeMS+15, sampleRate)
	return start, end, end-start >= minimum
}

func bestBranchLag(baseline, processed []float64, start, end, sampleRate int, atEnd bool) (int, float64) {
	window := min(msToFrames(openUtauPitchAlignMS, sampleRate), (end-start)/3)
	if window < 8 {
		return 0, 0
	}
	targetStart := start
	if atEnd {
		targetStart = end - window
	}
	search := msToFrames(openUtauPitchSearchMS, sampleRate)
	bestLag, bestCorrelation := 0, math.Inf(-1)
	for lag := -search; lag <= search; lag++ {
		sourceStart := targetStart + lag
		if sourceStart < 0 || sourceStart+window > len(processed) || targetStart+window > len(baseline) {
			continue
		}
		correlation := normalizedCorrelation(baseline[targetStart:targetStart+window], processed[sourceStart:sourceStart+window])
		if correlation > bestCorrelation {
			bestLag, bestCorrelation = lag, correlation
		}
	}
	return bestLag, bestCorrelation
}

// bestConstantBranchLag deliberately chooses one lag for the whole vowel.
// Interpolating integer sample positions between independent entry/exit lags
// duplicates and drops samples inside the vowel, creating a zipper-like
// electronic tone. A constant offset preserves the resampler's waveform and F0.
func bestConstantBranchLag(baseline, processed []float64, start, end, sampleRate int) (int, float64) {
	window := min(msToFrames(openUtauPitchAlignMS, sampleRate), (end-start)/3)
	if window < 8 {
		return 0, 0
	}
	search := msToFrames(openUtauPitchSearchMS, sampleRate)
	bestLag, bestScore := 0, math.Inf(-1)
	for lag := -search; lag <= search; lag++ {
		entrySource := start + lag
		exitTarget := end - window
		exitSource := exitTarget + lag
		if entrySource < 0 || entrySource+window > len(processed) || exitSource < 0 || exitSource+window > len(processed) || end > len(baseline) {
			continue
		}
		entry := normalizedCorrelation(baseline[start:start+window], processed[entrySource:entrySource+window])
		exit := normalizedCorrelation(baseline[exitTarget:end], processed[exitSource:exitSource+window])
		score := math.Min(entry, exit)
		if score > bestScore {
			bestLag, bestScore = lag, score
		}
	}
	return bestLag, bestScore
}
