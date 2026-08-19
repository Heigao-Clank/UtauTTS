package audio

import (
	"errors"
	"math"
)

// TrimPCMは録音の利用可能領域[offset, frames-blank]を返す。
// oto.iniではoffset（利用可能領域の開始）、
// blank（ファイル末尾から取り除く量。負の値は「offset 以降の -blank ms のみを保持」を意味する）、
// 子音境界（fixed）が分かれている。子音境界は切り出し領域の内部構造
// （無声部分が終わり母音が始まる位置）を表すため、切り出し範囲には影響しない。
// 必要な呼び出し側はEntry/planの値を使う。
func TrimPCM(pcm *PCM, offsetMs float64, blankMs float64) (*PCM, error) {
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
		end = start + msToFrames(-blankMs, pcm.SampleRate)
	} else {
		end = frames - msToFrames(blankMs, pcm.SampleRate)
	}
	if end > frames {
		end = frames
	}
	if end < 0 {
		end = 0
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
