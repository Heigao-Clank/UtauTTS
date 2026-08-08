package audio

import (
	"errors"
	"math"
)

func TrimPCM(pcm *PCM, offsetMs float64, fixedMs float64, blankMs float64) (*PCM, error) {
	if pcm.Channels <= 0 {
		return nil, errors.New("invalid channel count")
	}
	frames := len(pcm.Data) / pcm.Channels
	if frames == 0 {
		return nil, errors.New("empty pcm data")
	}

	start := msToFrames(offsetMs, pcm.SampleRate)
	if start < 0 {
		start = 0
	}
	var end int
	if blankMs < 0 {
		// A negative cutoff specifies the usable length measured from offset.
		end = start + msToFrames(-blankMs, pcm.SampleRate)
	} else {
		// A positive cutoff specifies the amount removed from the right edge.
		end = frames - msToFrames(blankMs, pcm.SampleRate)
	}
	if end > frames {
		end = frames
	}
	if end < 0 {
		end = 0
	}
	// fixedMs is the non-stretchable consonant length, not a trimming bound.
	_ = fixedMs
	if start >= end {
		return nil, errors.New("invalid trim range")
	}

	startIndex := start * pcm.Channels
	endIndex := end * pcm.Channels
	trimmed := make([]int16, endIndex-startIndex)
	copy(trimmed, pcm.Data[startIndex:endIndex])

	return &PCM{
		SampleRate: pcm.SampleRate,
		Channels:   pcm.Channels,
		Data:       trimmed,
	}, nil
}

func msToFrames(ms float64, sampleRate int) int {
	if ms <= 0 {
		return 0
	}
	return int(math.Round((ms / 1000.0) * float64(sampleRate)))
}
