package render

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"utautts/internal/audio"
	"utautts/internal/pitch"
	"utautts/internal/plan"
	"utautts/internal/processutil"
)

const worldlineFrameMS = 10.0

type worldlineManifest struct {
	Engine        string                  `json:"engine,omitempty"`
	WorldlinePath string                  `json:"worldline_path"`
	GPUPath       string                  `json:"gpu_path,omitempty"`
	OutputPath    string                  `json:"output_path"`
	SampleRate    int                     `json:"sample_rate"`
	F0Curve       []float64               `json:"f0_curve"`
	Units         []worldlineManifestUnit `json:"units"`
}

func renderOpenUtauClassicWorldlineFaithful(synthesisPlan *plan.Plan, cfg Config) (*audio.PCM, error) {
	return renderWorldlineEngine(synthesisPlan, cfg, "classic-worldline-faithful", true)
}

func renderOpenUtauClassicWorldlineFaithfulGPU(synthesisPlan *plan.Plan, cfg Config) (*audio.PCM, error) {
	return renderWorldlineEngine(synthesisPlan, cfg, "classic-worldline-faithful-gpu", true)
}

type worldlineManifestUnit struct {
	Source            string                   `json:"source"`
	FRQPath           string                   `json:"frq_path,omitempty"`
	PositionMS        float64                  `json:"position_ms"`
	SkipMS            float64                  `json:"skip_ms"`
	LengthMS          float64                  `json:"length_ms"`
	FadeInMS          float64                  `json:"fade_in_ms"`
	FadeOutMS         float64                  `json:"fade_out_ms"`
	OffsetMS          float64                  `json:"offset_ms"`
	RequiredLengthMS  float64                  `json:"required_length_ms"`
	ConsonantMS       float64                  `json:"consonant_ms"`
	CutoffMS          float64                  `json:"cutoff_ms"`
	Tone              int                      `json:"tone"`
	ConsonantVelocity float64                  `json:"consonant_velocity"`
	PitchStartMS      float64                  `json:"pitch_start_ms,omitempty"`
	PitchLengthMS     float64                  `json:"pitch_length_ms,omitempty"`
	Volume            float64                  `json:"volume,omitempty"`
	Modulation        float64                  `json:"modulation,omitempty"`
	Tempo             float64                  `json:"tempo,omitempty"`
	Envelope          []worldlineEnvelopePoint `json:"envelope,omitempty"`
}

type worldlineEnvelopePoint struct {
	XMS float64 `json:"x_ms"`
	Y   float64 `json:"y"`
}

func renderWorldlineEngine(synthesisPlan *plan.Plan, cfg Config, engine string, localSourcePitch bool) (*audio.PCM, error) {
	if synthesisPlan == nil || len(synthesisPlan.Units) == 0 {
		return nil, errors.New("empty synthesis plan")
	}
	library, err := resolveWorldlineLibrary(cfg.WorldlinePath)
	if err != nil {
		return nil, err
	}
	bridge, err := resolveWorldlineBridge(cfg.WorldlineBridgePath)
	if err != nil {
		return nil, err
	}
	gpuPath := ""
	if strings.HasSuffix(engine, "-gpu") {
		if err := gpuWaveformAvailable(); err != nil {
			return nil, err
		}
		gpuPath, err = gpuWaveformLibraryPath()
		if err != nil {
			return nil, err
		}
	}

	cache := newSourceCache()
	timings := make([]effectiveTiming, len(synthesisPlan.Units))
	var classicTimings []openUtauClassicTiming
	phraseStartMS := 0.0
	faithfulClassic := strings.HasPrefix(engine, "classic-worldline-faithful")
	if faithfulClassic {
		classicTimings, phraseStartMS = openUtauClassicTimings(synthesisPlan.Units)
	}
	for i := range synthesisPlan.Units {
		unit := &synthesisPlan.Units[i]
		timings[i] = normalizeTiming(*unit, cfg.ReleaseMS)
		if len(classicTimings) == len(synthesisPlan.Units) && !unit.Silent {
			timings[i].preutteranceMS = classicTimings[i].preutter
			timings[i].overlapMS = classicTimings[i].overlap
			timings[i].consonantMS = unit.ConsonantMS
			timings[i].scale = 1
		}
		unit.TimingScale = timings[i].scale
		unit.EffectivePreutteranceMS = timings[i].preutteranceMS
		unit.EffectiveConsonantMS = timings[i].consonantMS
		unit.EffectiveOverlapMS = timings[i].overlapMS
		unit.IntonationFactor = 1
	}
	intonation := identityFactors(len(synthesisPlan.Units))
	if cfg.ApplyPitch {
		intonation = analyzeIntonation(synthesisPlan, timings, &cache, cfg.IntonationStrength)
	}
	pitches, sampleRate, err := measureWorldlinePitches(synthesisPlan, &cache)
	if err != nil {
		return nil, err
	}
	reference := medianFloat(nonzeroFloats(pitches))
	if reference <= 0 {
		reference = 220
	}

	pitchFactors := make([]float64, len(synthesisPlan.Units))
	for i, unit := range synthesisPlan.Units {
		pitchFactors[i] = intonation[i]
		pitchFactors[i] *= effectiveUnitPitchFactor(unit, cfg.ApplyPitch)
	}
	frameMS := worldlineFrameMS
	f0Curve := worldlineF0CurveAt(synthesisPlan, pitches, pitchFactors, reference, max(2, int(math.Ceil((synthesisPlan.DurationMS+cfg.ReleaseMS)/frameMS))+2), frameMS)
	if localSourcePitch {
		f0Curve = worldlineLocalF0Curve(synthesisPlan, pitches, pitchFactors, reference, len(f0Curve))
	}
	manifest := worldlineManifest{
		Engine:        engine,
		WorldlinePath: library,
		GPUPath:       gpuPath,
		SampleRate:    sampleRate,
		F0Curve:       f0Curve,
	}
	for frame := range manifest.F0Curve {
		manifest.F0Curve[frame] *= pitchCurveFactorAt(cfg.PitchCurve, float64(frame)*frameMS)
	}
	for i := range synthesisPlan.Units {
		unit := &synthesisPlan.Units[i]
		if unit.Silent {
			continue
		}
		timing := timings[i]
		unitPitch := pitches[i]
		if unitPitch <= 0 {
			unitPitch = reference
		}
		unit.SourceF0Hz = pitches[i]
		unit.TargetF0Hz = unitPitch * pitchFactors[i] * pitchCurveFactorAt(cfg.PitchCurve, unit.NoteStartMS)
		unit.IntonationFactor = intonation[i]
		consonantVelocity := 100.0
		if timing.consonantMS > 0 && unit.ConsonantMS > 0 {
			consonantVelocity = 100 * (1 + math.Log2(unit.ConsonantMS/timing.consonantMS))
		}
		requiredLength := timing.preutteranceMS + unit.DurationMS + cfg.ReleaseMS
		positionMS := unit.NoteStartMS - timing.preutteranceMS
		skipMS := 0.0
		lengthMS := requiredLength
		pitchStartMS := positionMS
		volume, modulation, tempo := 100.0, 0.0, 120.0
		var envelopePoints []worldlineEnvelopePoint
		pitchLengthMS := 0.0
		if strings.HasPrefix(engine, "classic-worldline-") {
			// OpenUtau's ResamplerItem starts its bend array at the unscaled
			// source preutterance and asks the resampler for a 50 ms rounded
			// buffer. The convergence mixer then skips any leading excess.
			pitchLeadingMS := unit.PreutteranceMS
			skipMS = math.Max(0, pitchLeadingMS-timing.preutteranceMS)
			pitchStartMS = unit.NoteStartMS - pitchLeadingMS
			durCorrection := 0.0
			if faithfulClassic {
				phoneTiming := classicTimings[i]
				skipMS = pitchLeadingMS - phoneTiming.preutter
				durCorrection = phoneTiming.preutter - phoneTiming.tailIntrude + phoneTiming.tailOverlap
				envelopePoints = openUtauEnvelopeFromTiming(*unit, phoneTiming)
				pitchLengthMS = envelopePoints[4].XMS + pitchLeadingMS
				positionMS = unit.NoteStartMS - phoneTiming.preutter - phraseStartMS
			}
			requiredLength = math.Max(unit.DurationMS+durCorrection+skipMS, unit.ConsonantMS)
			requiredLength = math.Ceil(requiredLength/50+0.5) * 50
			lengthMS = timing.preutteranceMS + unit.DurationMS + cfg.ReleaseMS
			consonantVelocity = 100
		}
		if !faithfulClassic && positionMS < 0 {
			leadingTrimMS := -positionMS
			skipMS += leadingTrimMS
			lengthMS -= leadingTrimMS
			positionMS = 0
		}
		manifest.Units = append(manifest.Units, worldlineManifestUnit{
			Source: unit.Source, FRQPath: findFRQPath(unit.Source), PositionMS: positionMS, SkipMS: skipMS,
			LengthMS: lengthMS, FadeInMS: math.Max(2, timing.preutteranceMS-timing.overlapMS),
			FadeOutMS: cfg.ReleaseMS, OffsetMS: unit.OffsetMS, RequiredLengthMS: requiredLength,
			ConsonantMS: unit.ConsonantMS, CutoffMS: unit.CutoffMS,
			Tone: int(math.Round(69 + 12*math.Log2(unitPitch/440))), ConsonantVelocity: consonantVelocity,
			PitchStartMS: pitchStartMS, Volume: volume, Modulation: modulation, Tempo: tempo,
			PitchLengthMS: pitchLengthMS, Envelope: envelopePoints,
		})
	}

	tempDir, err := os.MkdirTemp("", "utautts-worldline-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tempDir)
	manifest.OutputPath = filepath.Join(tempDir, "output.wav")
	manifestPath := filepath.Join(tempDir, "manifest.json")
	data, err := json.Marshal(manifest)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(manifestPath, data, 0o600); err != nil {
		return nil, err
	}
	command := exec.Command(bridge, manifestPath)
	processutil.Configure(command)
	if output, err := command.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("worldline bridge failed: %w: %s", err, output)
	}
	pcm, err := audio.ReadWav(manifest.OutputPath)
	if err != nil {
		return nil, fmt.Errorf("read worldline output: %w", err)
	}
	if faithfulClassic && phraseStartMS < 0 {
		trim := min(len(pcm.Data), msToFrames(-phraseStartMS, pcm.SampleRate))
		pcm.Data = pcm.Data[trim:]
	}
	minimumFrames := msToFrames(synthesisPlan.DurationMS+cfg.ReleaseMS, pcm.SampleRate)
	if len(pcm.Data) < minimumFrames {
		pcm.Data = append(pcm.Data, make([]int16, minimumFrames-len(pcm.Data))...)
	}
	return pcm, nil
}

func findFRQPath(wavPath string) string {
	extension := filepath.Ext(wavPath)
	candidates := []string{
		strings.TrimSuffix(wavPath, extension) + "_wav.frq",
		strings.TrimSuffix(wavPath, extension) + ".frq",
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	return ""
}

type openUtauClassicTiming struct {
	preutter    float64
	overlap     float64
	tailIntrude float64
	tailOverlap float64
	overlapped  bool
}

func openUtauClassicTimings(units []plan.Unit) ([]openUtauClassicTiming, float64) {
	result := make([]openUtauClassicTiming, len(units))
	previous := -1
	first := -1
	for index, unit := range units {
		if unit.Silent {
			continue
		}
		if first < 0 {
			first = index
		}
		autoPreutter := unit.PreutteranceMS
		autoOverlap := unit.OverlapMS
		adjacent := false
		if previous >= 0 {
			previousUnit := units[previous]
			gapMS := unit.NoteStartMS - (previousUnit.NoteStartMS + previousUnit.DurationMS)
			previousDuration := previousUnit.DurationMS
			maxPreutter := autoPreutter
			if gapMS <= 0 {
				adjacent = true
				if autoOverlap > 0 && autoPreutter-autoOverlap > previousDuration*0.5 {
					maxPreutter = previousDuration * 0.5 / (autoPreutter - autoOverlap) * autoPreutter
				} else if autoOverlap <= 0 {
					maxPreutter = math.Min(maxPreutter, previousDuration*0.9)
				}
				maxPreutter = math.Min(maxPreutter, previousDuration)
				if result[previous].preutter < 5 {
					maxPreutter = math.Min(maxPreutter, previousDuration+result[previous].preutter-5)
				}
			} else if gapMS < autoPreutter {
				maxPreutter = gapMS
			}
			if autoPreutter > maxPreutter && autoPreutter > 0 {
				ratio := maxPreutter / autoPreutter
				autoPreutter = maxPreutter
				autoOverlap *= ratio
			}
			if autoOverlap < 0 {
				autoOverlap = math.Max(autoOverlap, math.Min(0, 35-previousDuration+autoPreutter))
			}
		}
		autoPreutter = math.Max(0, autoPreutter)
		result[index].preutter = autoPreutter
		result[index].overlap = autoOverlap
		result[index].overlapped = previous >= 0 && adjacent && autoOverlap > 0
		if previous >= 0 {
			if adjacent {
				result[previous].tailIntrude = math.Max(autoPreutter, autoPreutter-autoOverlap)
				result[previous].tailOverlap = math.Max(autoOverlap, 0)
			}
		}
		previous = index
	}
	phraseStart := 0.0
	if first >= 0 {
		phraseStart = units[first].NoteStartMS - result[first].preutter
	}
	return result, phraseStart
}

func openUtauEnvelopeFromTiming(unit plan.Unit, timing openUtauClassicTiming) []worldlineEnvelopePoint {
	fadeIn := 5.0
	if timing.overlapped {
		fadeIn = timing.overlap
	}
	fadeOut := 35.0
	if timing.tailOverlap > 0 {
		fadeOut = timing.tailOverlap
	}
	p0 := -timing.preutter
	p1 := math.Max(p0+5, p0+fadeIn)
	p2 := math.Max(0, p1)
	p4 := unit.DurationMS - timing.tailIntrude + timing.tailOverlap
	p3 := math.Max(p2, p4-fadeOut)
	return []worldlineEnvelopePoint{
		{XMS: p0, Y: 0}, {XMS: p1, Y: 1}, {XMS: p2, Y: 1},
		{XMS: p3, Y: 1}, {XMS: p4, Y: 0},
	}
}

func resolveWorldlineBridge(configured string) (string, error) {
	if configured == "" {
		return "", errors.New("worldline bridge is not configured by the renderer plugin")
	}
	if _, err := os.Stat(configured); err != nil {
		return "", fmt.Errorf("worldline bridge %q: %w", configured, err)
	}
	return configured, nil
}

func resolveWorldlineLibrary(configured string) (string, error) {
	if configured == "" {
		return "", errors.New("worldline library is not configured by the renderer plugin")
	}
	if _, err := os.Stat(configured); err != nil {
		return "", fmt.Errorf("worldline library %q: %w", configured, err)
	}
	return configured, nil
}

func measureWorldlinePitches(synthesisPlan *plan.Plan, cache *sourceCache) ([]float64, int, error) {
	values := make([]float64, len(synthesisPlan.Units))
	sampleRate := 0
	for i, unit := range synthesisPlan.Units {
		if unit.Silent {
			continue
		}
		mono, err := cache.loadMono(unit.Source)
		if err != nil {
			return nil, 0, err
		}
		if sampleRate == 0 {
			sampleRate = mono.SampleRate
		}
		if mono.SampleRate != sampleRate {
			return nil, 0, fmt.Errorf("worldline requires one sample rate per phrase: %s is %d Hz, want %d Hz", unit.Source, mono.SampleRate, sampleRate)
		}
		trimmed, err := audio.TrimPCM(mono, unit.OffsetMS, unit.ConsonantMS, unit.CutoffMS)
		if err != nil {
			return nil, 0, err
		}
		wave := pcmFloats(trimmed.Data)
		start := min(len(wave), msToFrames(unit.ConsonantMS, mono.SampleRate))
		end := min(len(wave), start+msToFrames(180, mono.SampleRate))
		if end-start >= msToFrames(30, mono.SampleRate) {
			values[i] = pitch.EstimateMedian(wave[start:end], mono.SampleRate)
		}
	}
	return stabilizeWorldlinePitches(values), sampleRate, nil
}

// stabilizeWorldlinePitches removes the most common short-recording pitch
// tracker failures before a source pitch is handed to Worldline. A voiced
// unit can occasionally be reported at a harmonic-related period (for
// example, about 3/2 or 2 times its neighbours) when the vowel has a strong
// formant or an irregular waveform. Treating that value as the target pitch
// makes the resampler shift the otherwise natural recording by several
// semitones. The lower-frequency member is kept as the local anchor and only
// the higher-frequency member is folded down. This one-way rule is important
// for very short phrases: a two-unit phrase must not make both units swap
// their pitches while correcting each other.
func stabilizeWorldlinePitches(values []float64) []float64 {
	result := append([]float64(nil), values...)
	for index, value := range values {
		if value <= 0 {
			continue
		}
		neighbor := nearestWorldlinePitch(values, index)
		if neighbor <= 0 {
			continue
		}
		ratio := value / neighbor
		if ratio < 1.35 {
			continue
		}
		factor := 2.0 / 3.0
		if ratio >= 1.8 {
			factor = 0.5
		}
		correctedRatio := ratio * factor
		if correctedRatio >= 0.87 && correctedRatio <= 1.15 {
			result[index] = value * factor
		}
	}
	return result
}

func nearestWorldlinePitch(values []float64, index int) float64 {
	for distance := 1; distance < len(values); distance++ {
		left := index - distance
		if left >= 0 && values[left] > 0 {
			return values[left]
		}
		right := index + distance
		if right < len(values) && values[right] > 0 {
			return values[right]
		}
	}
	return 0
}

func worldlineF0Curve(synthesisPlan *plan.Plan, pitches, factors []float64, reference float64, length int) []float64 {
	return worldlineF0CurveAt(synthesisPlan, pitches, factors, reference, length, worldlineFrameMS)
}

func worldlineF0CurveAt(synthesisPlan *plan.Plan, pitches, factors []float64, reference float64, length int, frameMS float64) []float64 {
	targets := make([]float64, len(pitches))
	for i, value := range pitches {
		if value <= 0 {
			value = reference
		}
		targets[i] = value * factors[i]
	}
	curve := make([]float64, length)
	unitIndex := 0
	for frame := range curve {
		timeMS := float64(frame) * frameMS
		for unitIndex+1 < len(synthesisPlan.Units) && synthesisPlan.Units[unitIndex+1].NoteStartMS <= timeMS {
			unitIndex++
		}
		value := targets[unitIndex]
		if unitIndex+1 < len(targets) {
			left := synthesisPlan.Units[unitIndex].NoteStartMS
			right := synthesisPlan.Units[unitIndex+1].NoteStartMS
			if right > left {
				progress := math.Max(0, math.Min(1, (timeMS-left)/(right-left)))
				value = math.Exp(math.Log(targets[unitIndex])*(1-progress) + math.Log(targets[unitIndex+1])*progress)
			}
		}
		curve[frame] = value
	}
	return curve
}

func worldlineLocalF0Curve(synthesisPlan *plan.Plan, pitches, factors []float64, reference float64, length int) []float64 {
	targets := make([]float64, len(pitches))
	for index, value := range pitches {
		if value <= 0 {
			value = reference
		}
		targets[index] = value * factors[index]
	}
	curve := make([]float64, length)
	unitIndex := 0
	for frame := range curve {
		timeMS := float64(frame) * worldlineFrameMS
		for unitIndex+1 < len(synthesisPlan.Units) && synthesisPlan.Units[unitIndex+1].NoteStartMS <= timeMS {
			unitIndex++
		}
		curve[frame] = targets[unitIndex]
	}
	return curve
}

func nonzeroFloats(values []float64) []float64 {
	result := make([]float64, 0, len(values))
	for _, value := range values {
		if value > 0 {
			result = append(result, value)
		}
	}
	return result
}
