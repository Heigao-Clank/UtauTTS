package render

import (
	"errors"
	"fmt"
	"math"

	"utautts/internal/audio"
	"utautts/internal/plan"
)

type Config struct {
	ReleaseMS float64
}

type sourceCache map[string]*audio.PCM

// Render places every unit on an absolute timeline. It intentionally performs
// no random processing so a plan always produces the same waveform.
func Render(synthesisPlan *plan.Plan, cfg Config) (*audio.PCM, error) {
	if synthesisPlan == nil || len(synthesisPlan.Units) == 0 {
		return nil, errors.New("empty synthesis plan")
	}
	if cfg.ReleaseMS <= 0 {
		cfg.ReleaseMS = 20
	}

	cache := sourceCache{}
	sampleRate := 0
	var mix []float64
	for _, unit := range synthesisPlan.Units {
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

		targetMS := math.Max(1, unit.PreutteranceMS+unit.DurationMS+cfg.ReleaseMS)
		targetFrames := msToFrames(targetMS, sampleRate)
		consonantFrames := msToFrames(unit.ConsonantMS, sampleRate)
		wave := pcmFloats(trimmed.Data)
		wave = shiftPitch(wave, unit.PitchFactor, sampleRate)
		wave = stretchPreservingPrefix(wave, targetFrames, consonantFrames, sampleRate)
		if unit.EnergyFactor > 0 {
			for i := range wave {
				wave[i] *= unit.EnergyFactor
			}
		}

		startFrame := msToFramesSigned(unit.NoteStartMS-unit.PreutteranceMS, sampleRate)
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
		}

		fadeInMS := unit.PreutteranceMS - unit.OverlapMS
		if fadeInMS < 2 {
			fadeInMS = 2
		}
		fadeInFrames := msToFrames(fadeInMS, sampleRate)
		fadeOutFrames := msToFrames(cfg.ReleaseMS, sampleRate)
		for sourceFrame := sourceStart; sourceFrame < len(wave); sourceFrame++ {
			gain := envelope(sourceFrame, len(wave), fadeInFrames, fadeOutFrames)
			mix[startFrame+sourceFrame-sourceStart] += wave[sourceFrame] * gain
		}
	}
	if sampleRate == 0 || len(mix) == 0 {
		return nil, errors.New("render produced no samples")
	}

	minimumFrames := msToFrames(synthesisPlan.DurationMS+cfg.ReleaseMS, sampleRate)
	if len(mix) < minimumFrames {
		mix = append(mix, make([]float64, minimumFrames-len(mix))...)
	}
	preventClipping(mix, 0.98)
	return &audio.PCM{SampleRate: sampleRate, Channels: 1, Data: floatPCM(mix)}, nil
}

func shiftPitch(source []float64, factor float64, sampleRate int) []float64 {
	if len(source) < 16 || factor <= 0 || math.Abs(factor-1) < 0.001 {
		return append([]float64(nil), source...)
	}
	factor = math.Max(0.75, math.Min(1.35, factor))
	resampled := linearResample(source, max(16, int(math.Round(float64(len(source))/factor))))
	return wsola(resampled, len(source), sampleRate)
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
		gain = float64(frame) / float64(fadeIn)
	}
	remaining := total - 1 - frame
	if fadeOut > 0 && remaining < fadeOut {
		outGain := float64(remaining) / float64(fadeOut)
		if outGain < gain {
			gain = outGain
		}
	}
	if gain < 0 {
		return 0
	}
	return gain
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
