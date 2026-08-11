//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"unicode/utf16"
	"unsafe"

	"utautts/internal/audio"
)

var (
	winmm       = syscall.NewLazyDLL("winmm.dll")
	comdlg32    = syscall.NewLazyDLL("comdlg32.dll")
	playSound   = winmm.NewProc("PlaySoundW")
	getSaveFile = comdlg32.NewProc("GetSaveFileNameW")
	getOpenFile = comdlg32.NewProc("GetOpenFileNameW")
)

const (
	sndAsync     = 0x0001
	sndNodefault = 0x0002
	sndPurge     = 0x0040
	sndFilename  = 0x20000

	ofnOverwritePrompt = 0x00000002
	ofnNoChangeDir     = 0x00000008
	ofnPathMustExist   = 0x00000800
	ofnExplorer        = 0x00080000
	ofnFileMustExist   = 0x00001000
)
type openFileName struct {
	StructSize       uint32
	Owner            uintptr
	Instance         uintptr
	Filter           *uint16
	CustomFilter     *uint16
	MaxCustomFilter  uint32
	FilterIndex      uint32
	File             *uint16
	MaxFile          uint32
	FileTitle        *uint16
	MaxFileTitle     uint32
	InitialDirectory *uint16
	Title            *uint16
	Flags            uint32
	FileOffset       uint16
	FileExtension    uint16
	DefExt           *uint16
	CustData         uintptr
	Hook             uintptr
	TemplateName     *uint16
	Reserved         uintptr
	Reserved2        uint32
	FlagsEx          uint32
}

func openJSONDialog(owner uintptr, titleText, currentPath string) (string, error) {
	return openPathDialog(owner, titleText, currentPath, []string{
		"JSONファイル (*.json)", "*.json",
		"すべてのファイル (*.*)", "*.*",
	})
}

func openExecutableDialog(owner uintptr, titleText, currentPath string) (string, error) {
	return openPathDialog(owner, titleText, currentPath, []string{
		"実行ファイル (*.exe;*.dll)", "*.exe;*.dll",
		"すべてのファイル (*.*)", "*.*",
	})
}

func openPathDialog(owner uintptr, titleText, currentPath string, filters []string) (string, error) {
	fileBuffer := make([]uint16, 32768)
	if currentPath != "" {
		copy(fileBuffer, windowsString(currentPath))
	}
	filter := doubleNullWindowsString(filters)
	title := windowsString(titleText)
	dialog := openFileName{
		StructSize: uint32(unsafe.Sizeof(openFileName{})), Owner: owner,
		Filter: &filter[0], FilterIndex: 1, File: &fileBuffer[0], MaxFile: uint32(len(fileBuffer)),
		Title: &title[0], Flags: ofnNoChangeDir | ofnPathMustExist | ofnFileMustExist | ofnExplorer,
	}
	result, _, callErr := getOpenFile.Call(uintptr(unsafe.Pointer(&dialog)))
	runtime.KeepAlive(filter)
	runtime.KeepAlive(title)
	runtime.KeepAlive(fileBuffer)
	if result == 0 {
		return "", nil
	}
	path := syscall.UTF16ToString(fileBuffer)
	if path == "" {
		return "", fmt.Errorf("選択したファイルのパスが空です: %v", callErr)
	}
	return path, nil
}

func writePreviewFile(pcm *audio.PCM) (string, error) {
	file, err := os.CreateTemp("", "utautts-preview-*.wav")
	if err != nil {
		return "", fmt.Errorf("create preview file: %w", err)
	}
	path := file.Name()
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return "", fmt.Errorf("close preview file: %w", err)
	}
	if err := audio.WriteWav(path, pcm); err != nil {
		_ = os.Remove(path)
		return "", fmt.Errorf("write preview file: %w", err)
	}
	return path, nil
}

func startPlayback(path string) error {
	buffer := windowsString(path)
	result, _, callErr := playSound.Call(
		uintptr(unsafe.Pointer(&buffer[0])),
		0,
		sndFilename|sndAsync|sndNodefault,
	)
	runtime.KeepAlive(buffer)
	if result == 0 {
		if callErr != nil {
			return fmt.Errorf("WAVを再生できませんでした: %v", callErr)
		}
		return fmt.Errorf("WAVを再生できませんでした")
	}
	return nil
}

func stopPlayback() {
	playSound.Call(0, 0, sndPurge)
}

func removePreviewFile(path string) {
	if path != "" {
		_ = os.Remove(path)
	}
}

func saveAudioDialog(owner uintptr, pcm *audio.PCM, suggestedPath string) (string, error) {
	name := filepath.Base(suggestedPath)
	if name == "" || name == "." || name == string(filepath.Separator) {
		name = "output.wav"
	}
	fileBuffer := make([]uint16, 32768)
	copy(fileBuffer, windowsString(name))
	filter := doubleNullWindowsString([]string{
		"WAVファイル (*.wav)", "*.wav",
		"すべてのファイル (*.*)", "*.*",
	})
	title := windowsString("音声を保存")
	defExt := windowsString("wav")
	dialog := openFileName{
		StructSize:  uint32(unsafe.Sizeof(openFileName{})),
		Owner:       owner,
		Filter:      &filter[0],
		FilterIndex: 1,
		File:        &fileBuffer[0],
		MaxFile:     uint32(len(fileBuffer)),
		Title:       &title[0],
		Flags:       ofnOverwritePrompt | ofnNoChangeDir | ofnPathMustExist | ofnExplorer,
		DefExt:      &defExt[0],
	}
	result, _, callErr := getSaveFile.Call(uintptr(unsafe.Pointer(&dialog)))
	runtime.KeepAlive(filter)
	runtime.KeepAlive(title)
	runtime.KeepAlive(defExt)
	runtime.KeepAlive(fileBuffer)
	if result == 0 {
		return "", nil
	}
	path := syscall.UTF16ToString(fileBuffer)
	if path == "" {
		return "", fmt.Errorf("保存先が空です")
	}
	if err := audio.WriteWav(path, pcm); err != nil {
		return "", fmt.Errorf("WAVを保存できませんでした: %w (dialog: %v)", err, callErr)
	}
	return path, nil
}

func doubleNullWindowsString(values []string) []uint16 {
	buffer := make([]uint16, 0, 64)
	for _, value := range values {
		buffer = append(buffer, utf16.Encode([]rune(value))...)
		buffer = append(buffer, 0)
	}
	buffer = append(buffer, 0)
	return buffer
}
