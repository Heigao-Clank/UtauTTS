//go:build windows

package main

import (
	"fmt"

	"utautts/internal/audio"
	"utautts/internal/tts"
)

func snapshotUtterances(hwnd uintptr) ([]utteranceState, error) {
	if editor == nil || len(editor.Utterances) == 0 {
		return nil, fmt.Errorf("発話がありません")
	}
	syncSelectedUtteranceText(hwnd)
	syncSelectedUtteranceControls(hwnd)
	storeSelectedDetailedSettings(advancedSettings)
	snapshot := make([]utteranceState, len(editor.Utterances))
	copy(snapshot, editor.Utterances)
	for index := range snapshot {
		if snapshot[index].Text == "" {
			return nil, fmt.Errorf("発話%dの文章が空です", index+1)
		}
		if snapshot[index].VoicebankPath == "" {
			return nil, fmt.Errorf("発話%dのボイスバンクが選択されていません", index+1)
		}
		if snapshot[index].Renderer == "" {
			snapshot[index].Renderer = defaultRendererBackend
		}
	}
	return snapshot, nil
}

func modelByPath(path string) *prosodyModelOption {
	if path == "" {
		return nil
	}
	for index := range availableProsodyModels {
		if availableProsodyModels[index].Path == path {
			return &availableProsodyModels[index]
		}
	}
	return nil
}

func configForUtterance(state utteranceState) (tts.Config, error) {
	base := tts.Config{VoicebankPath: state.VoicebankPath, Text: state.Text, Renderer: state.Renderer}
	settings := state.Synthesis
	if state.RendererExecutable != "" {
		switch state.RendererExecutableKind {
		case "worldline":
			settings.WorldlinePath = state.RendererExecutable
		case "bridge":
			settings.WorldlineBridgePath = state.RendererExecutable
		case "resampler":
			settings.UTAUResamplerPath = state.RendererExecutable
		}
	}
	config, err := configuredTTSConfigForState(base, modelByPath(state.ProsodyModel), settings, state.ManualPitch)
	if err != nil {
		return tts.Config{}, err
	}
	return config, nil
}

func synthesizeUtterances(states []utteranceState) (*audio.PCM, error) {
	var combined *audio.PCM
	for index, state := range states {
		config, err := configForUtterance(state)
		if err != nil {
			return nil, fmt.Errorf("発話%dの設定: %w", index+1, err)
		}
		result, err := tts.Synthesize(config)
		if err != nil {
			return nil, fmt.Errorf("発話%dの生成: %w", index+1, err)
		}
		combined, err = appendPCM(combined, result.Audio)
		if err != nil {
			return nil, fmt.Errorf("発話%dの連結: %w", index+1, err)
		}
	}
	return combined, nil
}

func appendPCM(left, right *audio.PCM) (*audio.PCM, error) {
	if right == nil {
		return left, nil
	}
	if left == nil {
		copyData := append([]int16(nil), right.Data...)
		return &audio.PCM{SampleRate: right.SampleRate, Channels: right.Channels, Data: copyData}, nil
	}
	if left.SampleRate != right.SampleRate || left.Channels != right.Channels {
		return nil, fmt.Errorf("音声形式が一致しません: %dHz/%dch と %dHz/%dch", left.SampleRate, left.Channels, right.SampleRate, right.Channels)
	}
	data := make([]int16, 0, len(left.Data)+len(right.Data))
	data = append(data, left.Data...)
	data = append(data, right.Data...)
	return &audio.PCM{SampleRate: left.SampleRate, Channels: left.Channels, Data: data}, nil
}
