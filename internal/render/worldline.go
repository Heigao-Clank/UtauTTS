package render

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"utautts/internal/audio"
	"utautts/internal/pitch"
	"utautts/internal/plan"
)

const worldlineFrameMS = 10.0

type worldlineManifest struct {
	WorldlinePath string                  `json:"worldline_path"`
	OutputPath    string                  `json:"output_path"`
	SampleRate    int                     `json:"sample_rate"`
	F0Curve       []float64               `json:"f0_curve"`
	Units         []worldlineManifestUnit `json:"units"`
}

type worldlineManifestUnit struct {
	Source            string  `json:"source"`
	FRQPath           string  `json:"frq_path,omitempty"`
	PositionMS        float64 `json:"position_ms"`
	SkipMS            float64 `json:"skip_ms"`
	LengthMS          float64 `json:"length_ms"`
	FadeInMS          float64 `json:"fade_in_ms"`
	FadeOutMS         float64 `json:"fade_out_ms"`
	OffsetMS          float64 `json:"offset_ms"`
	RequiredLengthMS  float64 `json:"required_length_ms"`
	ConsonantMS       float64 `json:"consonant_ms"`
	CutoffMS          float64 `json:"cutoff_ms"`
	Tone              int     `json:"tone"`
	ConsonantVelocity float64 `json:"consonant_velocity"`
}

func renderWorldline(synthesisPlan *plan.Plan, cfg Config) (*audio.PCM, error) {
	if synthesisPlan == nil || len(synthesisPlan.Units) == 0 {
		return nil, errors.New("empty synthesis plan")
	}
	if cfg.ReleaseMS <= 0 {
		cfg.ReleaseMS = 20
	}
	library, err := resolveWorldlineLibrary(cfg.WorldlinePath)
	if err != nil {
		return nil, err
	}
	bridge, err := resolveWorldlineBridge(cfg.WorldlineBridgePath)
	if err != nil {
		return nil, err
	}

	cache := sourceCache{}
	timings := make([]effectiveTiming, len(synthesisPlan.Units))
	for i := range synthesisPlan.Units {
		unit := &synthesisPlan.Units[i]
		timings[i] = normalizeTiming(*unit, cfg.ReleaseMS)
		unit.TimingScale = timings[i].scale
		unit.EffectivePreutteranceMS = timings[i].preutteranceMS
		unit.EffectiveConsonantMS = timings[i].consonantMS
		unit.EffectiveOverlapMS = timings[i].overlapMS
		unit.IntonationFactor = 1
	}
	intonation := analyzeIntonation(synthesisPlan, timings, cache, cfg.IntonationStrength)
	pitches, sampleRate, err := measureWorldlinePitches(synthesisPlan, cache)
	if err != nil {
		return nil, err
	}
	reference := medianFloat(nonzeroFloats(pitches))
	if reference <= 0 {
		reference = 220
	}

	manifest := worldlineManifest{
		WorldlinePath: library,
		SampleRate:    sampleRate,
		F0Curve: worldlineF0Curve(
			synthesisPlan, pitches, intonation, reference,
			max(2, int(math.Ceil((synthesisPlan.DurationMS+cfg.ReleaseMS)/worldlineFrameMS))+2),
		),
	}
	for i := range synthesisPlan.Units {
		unit := &synthesisPlan.Units[i]
		timing := timings[i]
		unitPitch := pitches[i]
		if unitPitch <= 0 {
			unitPitch = reference
		}
		unit.SourceF0Hz = pitches[i]
		unit.TargetF0Hz = unitPitch * intonation[i]
		unit.IntonationFactor = intonation[i]
		consonantVelocity := 100.0
		if timing.consonantMS > 0 && unit.ConsonantMS > 0 {
			consonantVelocity = 100 * (1 + math.Log2(unit.ConsonantMS/timing.consonantMS))
		}
		requiredLength := timing.preutteranceMS + unit.DurationMS + cfg.ReleaseMS
		positionMS := unit.NoteStartMS - timing.preutteranceMS
		skipMS := 0.0
		lengthMS := requiredLength
		if positionMS < 0 {
			skipMS = -positionMS
			lengthMS -= skipMS
			positionMS = 0
		}
		manifest.Units = append(manifest.Units, worldlineManifestUnit{
			Source: unit.Source, FRQPath: findFRQPath(unit.Source), PositionMS: positionMS, SkipMS: skipMS,
			LengthMS: lengthMS, FadeInMS: math.Max(2, timing.preutteranceMS-timing.overlapMS),
			FadeOutMS: cfg.ReleaseMS, OffsetMS: unit.OffsetMS, RequiredLengthMS: requiredLength,
			ConsonantMS: unit.ConsonantMS, CutoffMS: unit.CutoffMS,
			Tone: int(math.Round(69 + 12*math.Log2(unitPitch/440))), ConsonantVelocity: consonantVelocity,
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
	if output, err := command.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("worldline bridge failed: %w: %s", err, output)
	}
	pcm, err := audio.ReadWav(manifest.OutputPath)
	if err != nil {
		return nil, fmt.Errorf("read worldline output: %w", err)
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

func resolveWorldlineBridge(configured string) (string, error) {
	if configured != "" {
		if _, err := os.Stat(configured); err != nil {
			return "", fmt.Errorf("worldline bridge %q: %w", configured, err)
		}
		return configured, nil
	}
	name := "utautts-worldline-bridge"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	if current, err := os.Executable(); err == nil {
		candidate := filepath.Join(filepath.Dir(current), name)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	if candidate, err := exec.LookPath(name); err == nil {
		return candidate, nil
	}
	return "", fmt.Errorf("worldline bridge not found; pass --worldline-bridge")
}

func resolveWorldlineLibrary(configured string) (string, error) {
	if configured != "" {
		if _, err := os.Stat(configured); err != nil {
			return "", fmt.Errorf("worldline library %q: %w", configured, err)
		}
		return configured, nil
	}
	name := "libworldline.so"
	if runtime.GOOS == "windows" {
		name = "worldline.dll"
	} else if runtime.GOOS == "darwin" {
		name = "libworldline.dylib"
	}
	if current, err := os.Executable(); err == nil {
		candidate := filepath.Join(filepath.Dir(current), name)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	if candidate, err := exec.LookPath(name); err == nil {
		return candidate, nil
	}
	return "", fmt.Errorf("worldline library not found; pass --worldline")
}

func measureWorldlinePitches(synthesisPlan *plan.Plan, cache sourceCache) ([]float64, int, error) {
	values := make([]float64, len(synthesisPlan.Units))
	sampleRate := 0
	for i, unit := range synthesisPlan.Units {
		raw, err := cache.load(unit.Source)
		if err != nil {
			return nil, 0, err
		}
		mono := toMono(raw)
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
	return values, sampleRate, nil
}

func worldlineF0Curve(synthesisPlan *plan.Plan, pitches, factors []float64, reference float64, length int) []float64 {
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
		timeMS := float64(frame) * worldlineFrameMS
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

func nonzeroFloats(values []float64) []float64 {
	result := make([]float64, 0, len(values))
	for _, value := range values {
		if value > 0 {
			result = append(result, value)
		}
	}
	return result
}
