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
    blankFrames := msToFramesAllowNegative(blankMs, pcm.SampleRate)
    end := frames - blankFrames
    if end > frames {
        end = frames
    }
    if end < 0 {
        end = 0
    }
    if fixedMs > 0 {
        fixedFrames := msToFrames(fixedMs, pcm.SampleRate)
        if fixedFrames > 0 && end-start < fixedFrames {
            end = start + fixedFrames
            if end > frames {
                end = frames
            }
        }
    }
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

func msToFramesAllowNegative(ms float64, sampleRate int) int {
    if ms == 0 {
        return 0
    }
    return int(math.Round((ms / 1000.0) * float64(sampleRate)))
}
