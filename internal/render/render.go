package render

import (
	"errors"
	"fmt"
	"math"
	"sort"

	"utautts/internal/audio"
	"utautts/internal/pitch"
	"utautts/internal/plan"
)

type Config struct {
	ReleaseMS           float64
	IntonationStrength  float64
	Backend             string
	WorldlinePath       string
	WorldlineBridgePath string
}

type sourceCache map[string]*audio.PCM

type effectiveTiming struct {
	preutteranceMS float64
	consonantMS    float64
	overlapMS      float64
	scale          float64
}

func Render(synthesisPlan *plan.Plan, cfg Config) (*audio.PCM, error) {
	switch cfg.Backend {
	case "", "waveform":
		return renderWaveform(synthesisPlan, cfg)
	case "waveform-long":
		return renderWaveformLong(synthesisPlan, cfg)
	case "worldline":
		return renderWorldline(synthesisPlan, cfg)
	case "worldline-hybrid":
		return renderWorldlineHybrid(synthesisPlan, cfg, cvRestoreNone)
	case "worldline-hybrid-cv":
		return renderWorldlineHybrid(synthesisPlan, cfg, cvRestoreFull)
	case "worldline-hybrid-cv-balanced":
		return renderWorldlineHybrid(synthesisPlan, cfg, cvRestoreBalanced)
	default:
		return nil, fmt.Errorf("unknown renderer backend %q", cfg.Backend)
	}
}

func renderWaveformLong(synthesisPlan *plan.Plan, cfg Config) (*audio.PCM, error) {
	if synthesisPlan == nil || len(synthesisPlan.Units) == 0 {
		return nil, errors.New("empty synthesis plan")
	}
	if cfg.ReleaseMS <= 0 {
		cfg.ReleaseMS = 20
	}
	collapsed := *synthesisPlan
	collapsed.Units = collapseLongUnits(synthesisPlan.Units)
	for index := range synthesisPlan.Units {
		timing := normalizeTiming(synthesisPlan.Units[index], cfg.ReleaseMS)
		unit := &synthesisPlan.Units[index]
		unit.TimingScale = timing.scale
		unit.EffectivePreutteranceMS = timing.preutteranceMS
		unit.EffectiveConsonantMS = timing.consonantMS
		unit.EffectiveOverlapMS = timing.overlapMS
		unit.IntonationFactor = 1
	}
	return renderWaveform(&collapsed, cfg)
}

func collapseLongUnits(units []plan.Unit) []plan.Unit {
	result := make([]plan.Unit, 0, len(units))
	groupID := 0
	for start := 0; start < len(units); {
		end := start + 1
		for end < len(units) && canContinueLongUnit(units[end-1], units[end]) {
			end++
		}
		if end-start < 2 {
			result = append(result, units[start])
			start = end
			continue
		}
		groupID++
		for index := start; index < end; index++ {
			units[index].LongUnitGroup = groupID
			units[index].LongUnitSize = end - start
		}
		combined := units[start]
		last := units[end-1]
		combined.Alias = combined.Alias + "…" + last.Alias
		combined.DurationMS = last.NoteStartMS + last.DurationMS - combined.NoteStartMS
		combined.LongUnitGroup = groupID
		combined.LongUnitSize = end - start
		if last.CutoffMS < 0 {
			absoluteEndMS := last.OffsetMS - last.CutoffMS
			combined.CutoffMS = -(absoluteEndMS - combined.OffsetMS)
		} else {
			combined.CutoffMS = last.CutoffMS
		}
		result = append(result, combined)
		start = end
	}
	return result
}

func canContinueLongUnit(previous, current plan.Unit) bool {
	if previous.Silent || current.Silent || previous.Source == "" || previous.Source != current.Source {
		return false
	}
	if previous.OtoPath == "" || previous.OtoPath != current.OtoPath || current.OtoLine != previous.OtoLine+1 {
		return false
	}
	if current.Position != previous.Position+1 || current.OffsetMS <= previous.OffsetMS {
		return false
	}
	return math.Abs(previous.PitchFactor-current.PitchFactor) < 1e-6 && math.Abs(previous.EnergyFactor-current.EnergyFactor) < 1e-6
}

func renderWaveform(synthesisPlan *plan.Plan, cfg Config) (*audio.PCM, error) {
	if synthesisPlan == nil || len(synthesisPlan.Units) == 0 {
		return nil, errors.New("empty synthesis plan")
	}
	if cfg.ReleaseMS <= 0 {
		cfg.ReleaseMS = 20
	}

	cache := sourceCache{}
	sampleRate := 0
	var mix []float64
	var mixWeights []float64
	timings := make([]effectiveTiming, len(synthesisPlan.Units))
	for i := range synthesisPlan.Units {
		unit := &synthesisPlan.Units[i]
		timings[i] = normalizeTiming(*unit, cfg.ReleaseMS)
		unit.TimingScale = timings[i].scale
		unit.EffectivePreutteranceMS = timings[i].preutteranceMS
		unit.EffectiveConsonantMS = timings[i].consonantMS
		unit.EffectiveOverlapMS = timings[i].preutteranceMS - fadeInDurationMS(timings[i])
		unit.IntonationFactor = 1
	}
	intonation := analyzeIntonation(synthesisPlan, timings, cache, cfg.IntonationStrength)
	for unitIndex := range synthesisPlan.Units {
		unit := &synthesisPlan.Units[unitIndex]
		timing := timings[unitIndex]
		if unit.Silent {
			continue
		}
		raw, err := cache.load(unit.Source)
		if err != nil {
			return nil, fmt.Errorf("read unit %q (%s): %w", unit.Alias, unit.Source, err)
		}
		mono := toMono(raw)
		if sampleRate == 0 {
			sampleRate = mono.SampleRate
		}
		if mono.SampleRate != sampleRate {
			mono = resampleRate(mono, sampleRate)
		}
		trimmed, err := audio.TrimPCM(mono, unit.OffsetMS, unit.ConsonantMS, unit.CutoffMS)
		if err != nil {
			return nil, fmt.Errorf("trim unit %q: %w", unit.Alias, err)
		}

		targetMS := math.Max(1, timing.preutteranceMS+unit.DurationMS+cfg.ReleaseMS)
		targetFrames := msToFrames(targetMS, sampleRate)
		sourceConsonantFrames := msToFrames(unit.ConsonantMS, sampleRate)
		effectiveConsonantFrames := msToFrames(timing.consonantMS, sampleRate)
		wave := pcmFloats(trimmed.Data)
		appliedPitch := unit.PitchFactor * intonation[unitIndex]
		wave = resampleForPitch(wave, appliedPitch)
		if appliedPitch > 0 {
			sourceConsonantFrames = int(math.Round(float64(sourceConsonantFrames) / appliedPitch))
		}
		wave = retimeWithCompressedPrefix(wave, targetFrames, sourceConsonantFrames, effectiveConsonantFrames, sampleRate)
		if unit.EnergyFactor > 0 {
			for i := range wave {
				wave[i] *= unit.EnergyFactor
			}
		}

		startFrame := msToFramesSigned(unit.NoteStartMS-timing.preutteranceMS, sampleRate)
		sourceStart := 0
		if startFrame < 0 {
			sourceStart = -startFrame
			startFrame = 0
		}
		if sourceStart >= len(wave) {
			continue
		}
		endFrame := startFrame + len(wave) - sourceStart
		if endFrame > len(mix) {
			mix = append(mix, make([]float64, endFrame-len(mix))...)
			mixWeights = append(mixWeights, make([]float64, endFrame-len(mixWeights))...)
		}

		fadeInMS := fadeInDurationMS(timing)
		fadeInFrames := msToFrames(fadeInMS, sampleRate)
		fadeOutFrames := msToFrames(cfg.ReleaseMS, sampleRate)
		for sourceFrame := sourceStart; sourceFrame < len(wave); sourceFrame++ {
			gain := envelope(sourceFrame, len(wave), fadeInFrames, fadeOutFrames)
			position := startFrame + sourceFrame - sourceStart
			gain *= handoffGain(position, unitIndex, synthesisPlan, timings, sampleRate)
			mix[position] += wave[sourceFrame] * gain
			mixWeights[position] += gain
		}
	}
	if sampleRate == 0 || len(mix) == 0 {
		return nil, errors.New("render produced no samples")
	}

	minimumFrames := msToFrames(synthesisPlan.DurationMS+cfg.ReleaseMS, sampleRate)
	if len(mix) < minimumFrames {
		padding := minimumFrames - len(mix)
		mix = append(mix, make([]float64, padding)...)
		mixWeights = append(mixWeights, make([]float64, padding)...)
	}
	for i := range mix {
		if mixWeights[i] > 1 {
			mix[i] /= mixWeights[i]
		}
	}
	preventClipping(mix, 0.98)
	return &audio.PCM{SampleRate: sampleRate, Channels: 1, Data: floatPCM(mix)}, nil
}

func analyzeIntonation(synthesisPlan *plan.Plan, timings []effectiveTiming, cache sourceCache, strength float64) []float64 {
	factors := make([]float64, len(synthesisPlan.Units))
	for i := range factors {
		factors[i] = 1
	}
	strength = math.Max(0, math.Min(1, strength))
	if strength == 0 {
		return factors
	}

	pitches := make([]float64, len(synthesisPlan.Units))
	var voiced []float64
	for i, unit := range synthesisPlan.Units {
		if unit.Silent {
			continue
		}
		raw, err := cache.load(unit.Source)
		if err != nil {
			continue
		}
		mono := toMono(raw)
		trimmed, err := audio.TrimPCM(mono, unit.OffsetMS, unit.ConsonantMS, unit.CutoffMS)
		if err != nil {
			continue
		}
		wave := pcmFloats(trimmed.Data)
		start := min(len(wave), msToFrames(unit.ConsonantMS, mono.SampleRate))
		end := min(len(wave), start+msToFrames(180, mono.SampleRate))
		if end-start < msToFrames(30, mono.SampleRate) {
			continue
		}
		pitches[i] = pitch.EstimateMedian(wave[start:end], mono.SampleRate)
		if pitches[i] > 0 {
			voiced = append(voiced, pitches[i])
		}
	}
	reference := medianFloat(voiced)
	if reference <= 0 {
		return factors
	}
	for i := range pitches {
		if pitches[i] <= 0 {
			continue
		}
		for pitches[i] > reference*1.6 {
			pitches[i] /= 2
		}
		for pitches[i] < reference/1.6 {
			pitches[i] *= 2
		}
	}

	for start := 0; start < len(synthesisPlan.Units); {
		end := start + 1
		for end < len(synthesisPlan.Units) && synthesisPlan.Units[end].Position == synthesisPlan.Units[end-1].Position+1 {
			end++
		}
		for i := start; i < end; i++ {
			position := 0.0
			if end-start > 1 {
				position = float64(i-start) / float64(end-start-1)
			}
			semitones := 0.3 - 0.8*position
			if i == start {
				semitones -= 0.35
			}
			if i == start+1 {
				semitones += 0.25
			}
			target := reference * math.Pow(2, semitones/12)
			unit := &synthesisPlan.Units[i]
			unit.SourceF0Hz = pitches[i]
			if pitches[i] > 0 {
				effectiveStrength := strength
				if timings[i].scale < 1 {
					effectiveStrength *= math.Max(0.25, timings[i].scale)
				}
				factor := math.Pow(target/pitches[i], effectiveStrength)
				factors[i] = math.Max(0.92, math.Min(1.08, factor))
				pitchFactor := unit.PitchFactor
				if pitchFactor <= 0 {
					pitchFactor = 1
				}
				unit.TargetF0Hz = pitches[i] * factors[i] * pitchFactor
			}
			unit.IntonationFactor = factors[i]
		}
		start = end
	}
	return factors
}

func medianFloat(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	values = append([]float64(nil), values...)
	sort.Float64s(values)
	middle := len(values) / 2
	if len(values)%2 == 1 {
		return values[middle]
	}
	return (values[middle-1] + values[middle]) / 2
}

func handoffGain(globalFrame, unitIndex int, synthesisPlan *plan.Plan, timings []effectiveTiming, sampleRate int) float64 {
	if unitIndex+1 >= len(synthesisPlan.Units) {
		return 1
	}
	unit := synthesisPlan.Units[unitIndex]
	next := synthesisPlan.Units[unitIndex+1]
	if next.Position != unit.Position+1 {
		return 1
	}
	nextTiming := timings[unitIndex+1]
	start := msToFramesSigned(next.NoteStartMS-nextTiming.preutteranceMS, sampleRate)
	end := start + msToFrames(fadeInDurationMS(nextTiming), sampleRate)
	if globalFrame <= start {
		return 1
	}
	if globalFrame >= end || end <= start {
		return 0
	}
	progress := float64(globalFrame-start) / float64(end-start)
	return 1 - smoothstep(progress)
}

func fadeInDurationMS(timing effectiveTiming) float64 {
	// Some CV banks use zero preutterance, or set overlap equal to
	// preutterance. Without a floor, the previous unit is cut off exactly when
	// the next unit still has zero envelope gain, creating an audible click.
	return math.Max(6, timing.preutteranceMS-timing.overlapMS)
}

func normalizeTiming(unit plan.Unit, releaseMS float64) effectiveTiming {
	preutterance := math.Max(0, unit.PreutteranceMS)
	overlap := unit.OverlapMS
	consonant := math.Max(0, unit.ConsonantMS)
	scale := 1.0

	if preutterance > math.Max(120, unit.DurationMS*1.5) {
		effectivePreutterance := math.Max(80, unit.DurationMS*0.75)
		scale = effectivePreutterance / preutterance
		preutterance = effectivePreutterance
		if overlap > 0 {
			overlap *= scale
		}
		consonant *= scale
	}
	overlap = math.Min(overlap, preutterance)

	if scale < 1 {
		targetMS := preutterance + unit.DurationMS + releaseMS
		minimumTailMS := releaseMS + math.Max(40, unit.DurationMS*0.35)
		consonant = math.Min(consonant, math.Max(0, targetMS-minimumTailMS))
	}
	return effectiveTiming{preutterance, consonant, overlap, scale}
}

func resampleForPitch(source []float64, factor float64) []float64 {
	if len(source) < 16 || factor <= 0 || math.Abs(factor-1) < 0.001 {
		return append([]float64(nil), source...)
	}
	factor = math.Max(0.75, math.Min(1.35, factor))
	return linearResample(source, max(16, int(math.Round(float64(len(source))/factor))))
}

func (c sourceCache) load(path string) (*audio.PCM, error) {
	if pcm, ok := c[path]; ok {
		return pcm, nil
	}
	pcm, err := audio.ReadWav(path)
	if err != nil {
		return nil, err
	}
	c[path] = pcm
	return pcm, nil
}

func toMono(pcm *audio.PCM) *audio.PCM {
	if pcm.Channels == 1 {
		return pcm
	}
	frames := len(pcm.Data) / pcm.Channels
	data := make([]int16, frames)
	for frame := 0; frame < frames; frame++ {
		sum := 0
		for channel := 0; channel < pcm.Channels; channel++ {
			sum += int(pcm.Data[frame*pcm.Channels+channel])
		}
		data[frame] = int16(sum / pcm.Channels)
	}
	return &audio.PCM{SampleRate: pcm.SampleRate, Channels: 1, Data: data}
}

func resampleRate(pcm *audio.PCM, targetRate int) *audio.PCM {
	frames := len(pcm.Data)
	targetFrames := int(math.Round(float64(frames) * float64(targetRate) / float64(pcm.SampleRate)))
	return &audio.PCM{SampleRate: targetRate, Channels: 1, Data: floatPCM(linearResample(pcmFloats(pcm.Data), targetFrames))}
}

func envelope(frame, total, fadeIn, fadeOut int) float64 {
	gain := 1.0
	if fadeIn > 0 && frame < fadeIn {
		gain = smoothstep(float64(frame) / float64(fadeIn))
	}
	remaining := total - 1 - frame
	if fadeOut > 0 && remaining < fadeOut {
		outGain := smoothstep(float64(remaining) / float64(fadeOut))
		if outGain < gain {
			gain = outGain
		}
	}
	if gain < 0 {
		return 0
	}
	return gain
}

func smoothstep(value float64) float64 {
	value = math.Max(0, math.Min(1, value))
	return value * value * (3 - 2*value)
}

func preventClipping(data []float64, limit float64) {
	peak := 0.0
	for _, value := range data {
		if absolute := math.Abs(value); absolute > peak {
			peak = absolute
		}
	}
	if peak <= limit || peak == 0 {
		return
	}
	scale := limit / peak
	for i := range data {
		data[i] *= scale
	}
}

func pcmFloats(data []int16) []float64 {
	result := make([]float64, len(data))
	for i, value := range data {
		result[i] = float64(value) / 32768
	}
	return result
}

func floatPCM(data []float64) []int16 {
	result := make([]int16, len(data))
	for i, value := range data {
		value = math.Max(-1, math.Min(1, value))
		result[i] = int16(math.Round(value * 32767))
	}
	return result
}

func msToFrames(ms float64, sampleRate int) int {
	if ms <= 0 {
		return 0
	}
	return int(math.Round(ms * float64(sampleRate) / 1000))
}

func msToFramesSigned(ms float64, sampleRate int) int {
	return int(math.Round(ms * float64(sampleRate) / 1000))
}
