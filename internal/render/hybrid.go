package render

import (
	"fmt"
	"math"

	"utautts/internal/audio"
	"utautts/internal/pitch"
	"utautts/internal/plan"
	"utautts/internal/voicebank"
)

type cvRestoreMode int

const (
	cvRestoreNone cvRestoreMode = iota
	cvRestoreBalanced
	cvRestoreFull
)

func renderWorldlineHybrid(synthesisPlan *plan.Plan, cfg Config, restoreCV cvRestoreMode) (*audio.PCM, error) {
	worldlinePCM, err := renderWorldline(synthesisPlan, cfg)
	if err != nil {
		return nil, err
	}
	waveformPCM, err := renderWaveform(synthesisPlan, Config{
		ReleaseMS: cfg.ReleaseMS, IntonationStrength: cfg.IntonationStrength, Backend: "waveform",
	})
	if err != nil {
		return nil, err
	}
	if waveformPCM.SampleRate != worldlinePCM.SampleRate || waveformPCM.Channels != 1 || worldlinePCM.Channels != 1 {
		return nil, fmt.Errorf("hybrid renderer format mismatch")
	}
	length := max(len(waveformPCM.Data), len(worldlinePCM.Data))
	baseline := pcmFloats(waveformPCM.Data)
	vocoder := pcmFloats(worldlinePCM.Data)
	weights := directConsonantWeights(synthesisPlan, cfg.ReleaseMS, waveformPCM.SampleRate, length, baseline, vocoder, restoreCV)
	result := make([]int16, length)
	for i := range result {
		if weights[i] == 0 {
			if i < len(worldlinePCM.Data) {
				result[i] = worldlinePCM.Data[i]
			}
			continue
		}
		direct, synthesized := 0.0, 0.0
		if i < len(baseline) {
			direct = baseline[i]
		}
		if i < len(vocoder) {
			synthesized = vocoder[i]
		}
		mixed := synthesized*(1-weights[i]) + direct*weights[i]
		mixed = math.Max(-0.98, math.Min(0.98, mixed))
		result[i] = int16(math.Round(mixed * 32767))
	}
	return &audio.PCM{SampleRate: waveformPCM.SampleRate, Channels: 1, Data: result}, nil
}

func directConsonantWeights(synthesisPlan *plan.Plan, releaseMS float64, sampleRate, length int, baseline, synthesized []float64, restoreCV cvRestoreMode) []float64 {
	weights := make([]float64, length)
	window := max(16, msToFrames(30, sampleRate))
	hop := max(1, msToFrames(5, sampleRate))
	for _, unit := range synthesisPlan.Units {
		timing := normalizeTiming(unit, releaseMS)
		start := max(0, msToFramesSigned(unit.NoteStartMS-timing.preutteranceMS, sampleRate))
		end := min(length, start+msToFrames(timing.consonantMS, sampleRate))
		isCV := voicebank.ClassifyAlias(unit.Alias) == voicebank.AliasCV
		if restoreCV == cvRestoreFull && isCV {
			for index := start; index < end; index++ {
				weights[index] = 1
			}
			continue
		}
		if restoreCV == cvRestoreBalanced && isCV {
			// A CV fixed region often extends into its periodic vowel. Restoring the
			// whole region makes the consonant clear, but also exposes rough WSOLA
			// pitch artifacts in the vowel. Protect the preutterance (the consonant
			// attack) and only a short release after the note boundary instead.
			attackEnd := min(end, msToFramesSigned(unit.NoteStartMS+8, sampleRate))
			for index := start; index < attackEnd; index++ {
				weights[index] = math.Max(weights[index], 0.85)
			}
		}
		for center := start; center < end; center += hop {
			frameStart := max(0, center-window/2)
			frameEnd := min(len(baseline), frameStart+window)
			if frameEnd-frameStart < 16 {
				continue
			}
			baselineRMS := frameRMS(baseline[frameStart:frameEnd])
			if baselineRMS < 0.003 {
				continue
			}
			synthEnd := min(len(synthesized), frameEnd)
			synthRMS := 0.0
			if synthEnd > frameStart {
				synthRMS = frameRMS(synthesized[frameStart:synthEnd])
			}
			aperiodic := pitch.Estimate(baseline[frameStart:frameEnd], sampleRate) == 0
			attenuated := baselineRMS >= 0.006 && synthRMS < baselineRMS*0.72
			if !aperiodic && !attenuated {
				continue
			}
			blockEnd := min(end, center+hop)
			for i := center; i < blockEnd; i++ {
				weights[i] = 1
			}
		}
	}

	transition := max(1, msToFrames(4, sampleRate))
	step := 1 / float64(transition)
	for i := 1; i < len(weights); i++ {
		weights[i] = math.Max(weights[i], weights[i-1]-step)
	}
	for i := len(weights) - 2; i >= 0; i-- {
		weights[i] = math.Max(weights[i], weights[i+1]-step)
	}
	return weights
}

func frameRMS(values []float64) float64 {
	energy := 0.0
	for _, value := range values {
		energy += value * value
	}
	if len(values) == 0 {
		return 0
	}
	return math.Sqrt(energy / float64(len(values)))
}
