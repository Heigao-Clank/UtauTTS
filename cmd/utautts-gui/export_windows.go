//go:build windows

package main

import (
	"fmt"
	"log"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"utautts/internal/audio"
	"utautts/internal/tts"
)

var (
	shBrowseForFolder   = syscall.NewLazyDLL("shell32.dll").NewProc("SHBrowseForFolderW")
	shGetPathFromIDList = syscall.NewLazyDLL("shell32.dll").NewProc("SHGetPathFromIDListW")
	coTaskMemFree       = ole32.NewProc("CoTaskMemFree")
)

const (
	bifReturnOnlyFileSystemDirs = 0x0001
	bifNewDialogStyle           = 0x0040
)

type browseInfo struct {
	Owner       uintptr
	Root        uintptr
	DisplayName *uint16
	Title       *uint16
	Flags       uint32
	Callback    uintptr
	Param       uintptr
	Image       int
}

func selectOutputDirectory(owner uintptr) (string, error) {
	display := make([]uint16, 32768)
	title := windowsString("全発話の出力先フォルダーを選択")
	info := browseInfo{Owner: owner, DisplayName: &display[0], Title: &title[0], Flags: bifReturnOnlyFileSystemDirs | bifNewDialogStyle}
	item, _, callErr := shBrowseForFolder.Call(uintptr(unsafe.Pointer(&info)))
	runtime.KeepAlive(display)
	runtime.KeepAlive(title)
	if item == 0 {
		return "", nil
	}
	defer coTaskMemFree.Call(item)
	path := make([]uint16, 32768)
	if result, _, _ := shGetPathFromIDList.Call(item, uintptr(unsafe.Pointer(&path[0]))); result == 0 {
		return "", fmt.Errorf("出力先フォルダーを取得できませんでした: %v", callErr)
	}
	return syscall.UTF16ToString(path), nil
}

func snapshotSelectedUtterance(hwnd uintptr) (utteranceState, error) {
	if editor == nil || editor.selected() == nil {
		return utteranceState{}, fmt.Errorf("選択中の発話がありません")
	}
	syncSelectedUtteranceText(hwnd)
	syncSelectedUtteranceControls(hwnd)
	storeSelectedDetailedSettings(advancedSettings)
	state := *editor.selected()
	if strings.TrimSpace(state.Text) == "" {
		return state, fmt.Errorf("文章が空です")
	}
	if state.VoicebankPath == "" {
		return state, fmt.Errorf("ボイスバンクが選択されていません")
	}
	return state, nil
}

func voicebankNameForPath(path string) string {
	for _, bank := range availableBanks {
		if samePath(bank.Path, path) {
			return sanitizeDisplayName(bank.Name)
		}
	}
	return "voicebank"
}

func outputFilename(state utteranceState) string {
	text := strings.Join(strings.Fields(state.Text), " ")
	text = sanitizeFilenamePart(text)
	if len([]rune(text)) > 80 {
		text = string([]rune(text)[:80])
	}
	if text == "" {
		text = "speech"
	}
	return sanitizeFilenamePart(voicebankNameForPath(state.VoicebankPath)) + "_" + text + ".wav"
}

func sanitizeFilenamePart(value string) string {
	value = sanitizeDisplayName(value)
	value = strings.Map(func(r rune) rune {
		switch r {
		case '<', '>', ':', '"', '/', '\\', '|', '?', '*':
			return '_'
		default:
			return r
		}
	}, value)
	return strings.Trim(value, " .")
}

func exportSelectedUtterance(hwnd uintptr) {
	if synthesisRunning {
		return
	}
	state, err := snapshotSelectedUtterance(hwnd)
	if err != nil {
		showError(hwnd, err)
		return
	}
	path, err := saveAudioDialog(hwnd, nil, outputFilename(state))
	if err != nil || path == "" {
		if err != nil {
			showError(hwnd, err)
		}
		return
	}
	startExport(hwnd, "選択中の発話を書き出し中...", func() error {
		config, err := configForUtterance(state)
		if err != nil {
			return err
		}
		result, err := tts.Synthesize(config)
		if err != nil {
			return err
		}
		return audio.WriteWav(path, result.Audio)
	})
}

func exportAllUtterances(hwnd uintptr) {
	if synthesisRunning {
		return
	}
	states, err := snapshotUtterances(hwnd)
	if err != nil {
		showError(hwnd, err)
		return
	}
	directory, err := selectOutputDirectory(hwnd)
	if err != nil || directory == "" {
		if err != nil {
			showError(hwnd, err)
		}
		return
	}
	startExport(hwnd, fmt.Sprintf("全%d発話を書き出し中...", len(states)), func() error {
		used := map[string]int{}
		for index, state := range states {
			config, err := configForUtterance(state)
			if err != nil {
				return fmt.Errorf("発話%d: %w", index+1, err)
			}
			result, err := tts.Synthesize(config)
			if err != nil {
				return fmt.Errorf("発話%d: %w", index+1, err)
			}
			name := outputFilename(state)
			base := strings.TrimSuffix(name, filepath.Ext(name))
			used[base]++
			if used[base] > 1 {
				name = fmt.Sprintf("%s_%d.wav", base, used[base])
			}
			if err := audio.WriteWav(filepath.Join(directory, name), result.Audio); err != nil {
				return fmt.Errorf("発話%dの保存: %w", index+1, err)
			}
		}
		return nil
	})
}

func startExport(hwnd uintptr, message string, operation func() error) {
	synthesisRunning = true
	exportError = nil
	exportMessage = message
	enableWindow.Call(generateButton, 0)
	setText(statusLabel, message)
	go func() {
		started := time.Now()
		exportError = runSafely("音声書き出し", operation)
		log.Printf("audio export finished: elapsed=%s error=%v", time.Since(started).Round(time.Millisecond), exportError)
		postMessage.Call(hwnd, wmExportDone, 0, 0)
	}()
}

func finishExport(hwnd uintptr) {
	synthesisRunning = false
	enableWindow.Call(generateButton, 1)
	if exportError != nil {
		setText(statusLabel, "書き出し失敗")
		showError(hwnd, exportError)
		return
	}
	setText(statusLabel, "音声を書き出しました")
}
