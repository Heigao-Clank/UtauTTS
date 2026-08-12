package render

import (
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"utautts/internal/audio"
	"utautts/internal/plan"
)

const utauPitchFrameMS = 60000.0 / 120.0 * 5.0 / 480.0

var utauClassicCache = struct {
	sync.Mutex
	items map[string]*audio.PCM
}{items: map[string]*audio.PCM{}}

func renderUTAUClassic(synthesisPlan *plan.Plan, cfg Config) (*audio.PCM, error) {
	if synthesisPlan == nil || len(synthesisPlan.Units) == 0 {
		return nil, errors.New("empty synthesis plan")
	}
	if cfg.ReleaseMS <= 0 {
		cfg.ReleaseMS = 20
	}
	resampler, err := resolveUTAUResampler(cfg.UTAUResamplerPath)
	if err != nil {
		return nil, err
	}
	work, err := os.MkdirTemp("", "utautts-classic-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(work)

	cache := newSourceCache()
	timings := make([]effectiveTiming, len(synthesisPlan.Units))
	for i := range synthesisPlan.Units {
		timings[i] = normalizeTiming(synthesisPlan.Units[i], cfg.ReleaseMS)
		unit := &synthesisPlan.Units[i]
		unit.TimingScale = timings[i].scale
		unit.EffectivePreutteranceMS = timings[i].preutteranceMS
		unit.EffectiveConsonantMS = timings[i].consonantMS
		unit.EffectiveOverlapMS = timings[i].overlapMS
		unit.IntonationFactor = 1
	}
	intonation := identityFactors(len(synthesisPlan.Units))
	if cfg.IntonationStrength > 0 {
		intonation = analyzeIntonation(synthesisPlan, timings, &cache, cfg.IntonationStrength)
	}
	pitches, _, err := measureWorldlinePitches(synthesisPlan, &cache)
	if err != nil {
		return nil, err
	}
	reference := medianFloat(nonzeroFloats(pitches))
	if reference <= 0 {
		reference = 220
	}
	targets := make([]float64, len(synthesisPlan.Units))
	for i, unit := range synthesisPlan.Units {
		sourceF0 := pitches[i]
		if sourceF0 <= 0 {
			sourceF0 = reference
		}
		factor := intonation[i]
		if unit.PitchFactor > 0 {
			factor *= unit.PitchFactor
		}
		targets[i] = sourceF0 * factor
		unit.SourceF0Hz = pitches[i]
		unit.TargetF0Hz = targets[i] * pitchCurveFactorAt(cfg.PitchCurve, unit.NoteStartMS)
		unit.IntonationFactor = intonation[i]
	}

	var mix, weights []float64
	sampleRate := 0
	for i, unit := range synthesisPlan.Units {
		if unit.Silent {
			continue
		}
		timing := timings[i]
		input := filepath.Join(work, fmt.Sprintf("unit-%04d.wav", i))
		output := filepath.Join(work, fmt.Sprintf("render-%04d.wav", i))
		if err := copyFile(unit.Source, input); err != nil {
			return nil, fmt.Errorf("copy unit %q: %w", unit.Alias, err)
		}
		if frq := findFRQPath(unit.Source); frq != "" {
			if err := copyFile(frq, strings.TrimSuffix(input, filepath.Ext(input))+"_wav.frq"); err != nil {
				return nil, fmt.Errorf("copy frq for %q: %w", unit.Alias, err)
			}
		}
		requiredMS := timing.preutteranceMS + unit.DurationMS + cfg.ReleaseMS
		positionMS := unit.NoteStartMS - timing.preutteranceMS
		baseF0 := pitches[i]
		if baseF0 <= 0 {
			baseF0 = reference
		}
		tone := int(math.Round(69 + 12*math.Log2(baseF0/440)))
		bendCount := max(2, int(math.Ceil(requiredMS/utauPitchFrameMS)))
		bends := make([]int, bendCount)
		for frame := range bends {
			globalMS := positionMS + float64(frame)*utauPitchFrameMS
			target := interpolatedTargetF0(synthesisPlan, targets, globalMS)
			target *= pitchCurveFactorAt(cfg.PitchCurve, globalMS)
			bends[frame] = int(math.Round(1200 * math.Log2(target/midiF0(tone))))
			bends[frame] = max(-2048, min(2047, bends[frame]))
		}
		velocity := 100.0
		if timing.consonantMS > 0 && unit.ConsonantMS > 0 {
			velocity = 100 * (1 + math.Log2(unit.ConsonantMS/timing.consonantMS))
		}
		args := []string{
			input, output, midiToneName(tone), strconv.Itoa(int(math.Round(velocity))), "",
			formatFloat(unit.OffsetMS), formatFloat(requiredMS), formatFloat(unit.ConsonantMS), formatFloat(unit.CutoffMS),
			"100", "0", "!120", encodeInt12(bends),
		}
		cacheKey := unit.Source + "\x00" + strings.Join(args[2:], "\x00")
		utauClassicCache.Lock()
		cached := clonePCM(utauClassicCache.items[cacheKey])
		utauClassicCache.Unlock()
		pcm := cached
		if pcm == nil {
			if result, runErr := exec.Command(resampler, args...).CombinedOutput(); runErr != nil {
				return nil, fmt.Errorf("UTAU resampler failed for %q: %w: %s", unit.Alias, runErr, result)
			}
			pcm, err = audio.ReadWav(output)
			if err != nil {
				return nil, fmt.Errorf("read resampled unit %q: %w", unit.Alias, err)
			}
			utauClassicCache.Lock()
			utauClassicCache.items[cacheKey] = clonePCM(pcm)
			utauClassicCache.Unlock()
		}
		mono := toMono(pcm)
		if sampleRate == 0 {
			sampleRate = mono.SampleRate
		}
		if mono.SampleRate != sampleRate {
			mono = resampleRate(mono, sampleRate)
		}
		wave := pcmFloats(mono.Data)
		if unit.EnergyFactor > 0 {
			for frame := range wave {
				wave[frame] *= unit.EnergyFactor
			}
		}
		start := msToFramesSigned(positionMS, sampleRate)
		sourceStart := 0
		if start < 0 {
			sourceStart = -start
			start = 0
		}
		if sourceStart >= len(wave) {
			continue
		}
		end := start + len(wave) - sourceStart
		if end > len(mix) {
			mix = append(mix, make([]float64, end-len(mix))...)
			weights = append(weights, make([]float64, end-len(weights))...)
		}
		fadeIn := msToFrames(fadeInDurationMS(timing), sampleRate)
		fadeOut := msToFrames(cfg.ReleaseMS, sampleRate)
		for sourceFrame := sourceStart; sourceFrame < len(wave); sourceFrame++ {
			position := start + sourceFrame - sourceStart
			gain := envelope(sourceFrame, len(wave), fadeIn, fadeOut)
			gain *= handoffGain(position, i, synthesisPlan, timings, sampleRate)
			mix[position] += wave[sourceFrame] * gain
			weights[position] += gain
		}
	}
	if sampleRate == 0 || len(mix) == 0 {
		return nil, errors.New("UTAU resampler produced no audio")
	}
	minimum := msToFrames(synthesisPlan.DurationMS+cfg.ReleaseMS, sampleRate)
	if len(mix) < minimum {
		mix = append(mix, make([]float64, minimum-len(mix))...)
		weights = append(weights, make([]float64, minimum-len(weights))...)
	}
	for i := range mix {
		if weights[i] > 1 {
			mix[i] /= weights[i]
		}
	}
	preventClipping(mix, 0.98)
	return &audio.PCM{SampleRate: sampleRate, Channels: 1, Data: floatPCM(mix)}, nil
}

func clonePCM(source *audio.PCM) *audio.PCM {
	if source == nil {
		return nil
	}
	return &audio.PCM{SampleRate: source.SampleRate, Channels: source.Channels, Data: append([]int16(nil), source.Data...)}
}

func interpolatedTargetF0(synthesisPlan *plan.Plan, targets []float64, timeMS float64) float64 {
	index := 0
	for index+1 < len(synthesisPlan.Units) && synthesisPlan.Units[index+1].NoteStartMS <= timeMS {
		index++
	}
	value := targets[index]
	if index+1 < len(targets) {
		left := synthesisPlan.Units[index].NoteStartMS
		right := synthesisPlan.Units[index+1].NoteStartMS
		if right > left {
			progress := math.Max(0, math.Min(1, (timeMS-left)/(right-left)))
			value = math.Exp(math.Log(targets[index])*(1-progress) + math.Log(targets[index+1])*progress)
		}
	}
	return value
}

func resolveUTAUResampler(configured string) (string, error) {
	if configured != "" {
		if info, err := os.Stat(configured); err != nil || info.IsDir() {
			return "", fmt.Errorf("UTAU resampler %q not found", configured)
		}
		return configured, nil
	}
	candidates := []string{}
	if runtime.GOOS == "windows" {
		if root := os.Getenv("ProgramFiles(x86)"); root != "" {
			candidates = append(candidates, filepath.Join(root, "UTAU", "resampler.exe"))
		}
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	if found, err := exec.LookPath("resampler.exe"); err == nil {
		return found, nil
	}
	return "", errors.New("UTAU resampler not found; pass --utau-resampler")
}

func copyFile(source, destination string) error {
	data, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	return os.WriteFile(destination, data, 0o600)
}

func midiF0(tone int) float64 { return 440 * math.Pow(2, float64(tone-69)/12) }

func midiToneName(tone int) string {
	names := [...]string{"C", "C#", "D", "D#", "E", "F", "F#", "G", "G#", "A", "A#", "B"}
	note := ((tone % 12) + 12) % 12
	return names[note] + strconv.Itoa(tone/12-1)
}

func formatFloat(value float64) string {
	return strconv.FormatFloat(value, 'f', 6, 64)
}

func encodeInt12(values []int) string {
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	var result strings.Builder
	last := ""
	duplicates := 0
	flush := func() {
		if duplicates > 0 {
			result.WriteByte('#')
			result.WriteString(strconv.Itoa(duplicates))
			result.WriteByte('#')
			duplicates = 0
		}
	}
	for _, value := range values {
		if value < 0 {
			value += 4096
		}
		encoded := string([]byte{alphabet[(value>>6)&63], alphabet[value&63]})
		if encoded == last {
			duplicates++
			continue
		}
		flush()
		result.WriteString(encoded)
		last = encoded
	}
	flush()
	return result.String()
}
