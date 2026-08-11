//go:build windows

package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"utautts/internal/audio"
	"utautts/internal/tts"
	"utautts/internal/voicebank"
)

const (
	wmDestroy              = 0x0002
	wmGetText              = 0x000D
	wmCommand              = 0x0111
	wmSetFont              = 0x0030
	wmSynthesisDone        = 0x8001
	wmInitializeVoicebanks = 0x8002
	wmVoicebanksDone       = 0x8003

	wsOverlappedWindow = 0x00CF0000
	wsVisible          = 0x10000000
	wsChild            = 0x40000000
	wsTabStop          = 0x00010000
	wsBorder           = 0x00800000
	wsVScroll          = 0x00200000

	wsExClientEdge  = 0x00000200
	esMultiline     = 0x0004
	esAutoVScroll   = 0x0040
	esWantReturn    = 0x1000
	esReadOnly      = 0x0800
	cbsDropdownList = 0x0003

	cbAddString     = 0x0143
	cbGetCurSel     = 0x0147
	cbResetContent  = 0x014B
	cbSetCurSel     = 0x014E
	cbSetItemHeight = 0x0154
	cbnSelChange    = 1
	defaultFont     = 17
	colorBtnFace    = 15

	idVoicebank    = 1001
	idReload       = 1002
	idText         = 1003
	idRenderer     = 1004
	idIntonate     = 1005
	idOutput       = 1006
	idGenerate     = 1007
	idPlay         = 1008
	idSave         = 1009
	idStop         = 1010
	idRendererInfo = 1011

	mbIconError = 0x10
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	ole32    = syscall.NewLazyDLL("ole32.dll")

	registerClassEx = user32.NewProc("RegisterClassExW")
	createWindowEx  = user32.NewProc("CreateWindowExW")
	defWindowProc   = user32.NewProc("DefWindowProcW")
	showWindow      = user32.NewProc("ShowWindow")
	updateWindow    = user32.NewProc("UpdateWindow")
	getMessage      = user32.NewProc("GetMessageW")
	translateMsg    = user32.NewProc("TranslateMessage")
	dispatchMsg     = user32.NewProc("DispatchMessageW")
	postQuitMessage = user32.NewProc("PostQuitMessage")
	sendMessage     = user32.NewProc("SendMessageW")
	postMessage     = user32.NewProc("PostMessageW")
	setWindowText   = user32.NewProc("SetWindowTextW")
	getDlgItem      = user32.NewProc("GetDlgItem")
	enableWindow    = user32.NewProc("EnableWindow")
	messageBox      = user32.NewProc("MessageBoxW")
	loadCursor      = user32.NewProc("LoadCursorW")

	getModuleHandle = kernel32.NewProc("GetModuleHandleW")
	getStockObject  = gdi32.NewProc("GetStockObject")
	coInitializeEx  = ole32.NewProc("CoInitializeEx")
	coUninitialize  = ole32.NewProc("CoUninitialize")

	mainWindow        uintptr
	generateButton    uintptr
	playButton        uintptr
	stopButton        uintptr
	saveButton        uintptr
	reloadButton      uintptr
	statusLabel       uintptr
	rendererInfoLabel uintptr
	controls          []uintptr
	completionMutex   sync.Mutex
	completionError   error
	synthesisRunning  bool
	lastOutput        string
	lastAudio         *audio.PCM
	previewPath       string
	initialVoicebank  string
	voiceDirectory    string
	availableBanks    []voicebank.Summary
	initialText       string
	initialOutput     string
)

type point struct {
	X int32
	Y int32
}

type rendererOption struct {
	label       string
	backend     string
	description string
}

var rendererOptions = []rendererOption{
	{label: "標準 waveform（推奨）", backend: "waveform", description: "原音の明瞭さを最優先する安定版です。イントネーション加工は行いません。"},
	{label: "OpenUTAU Classic faithful（研究版）", backend: "openutau-classic-worldline-faithful", description: "接続品質が最良だった研究rendererです。worldlineとbridgeが必要です。v8任意文章モデルはGUIへまだ統合していません。"},
	{label: "長い原音 waveform-long（研究版）", backend: "waveform-long", description: "連続する同一原音を長く使う診断方式です。通常利用では標準waveformを選んでください。"},
	{label: "WORLD hybrid gentle（過去比較用）", backend: "worldline-hybrid-cv-gentle", description: "過去の比較を再現するbackendです。歯抜けや加工感が出るため推奨しません。"},
	{label: "WORLD hybrid balanced（過去比較用）", backend: "worldline-hybrid-cv-balanced", description: "過去の比較を再現するbackendです。歯抜けや加工感が出るため推奨しません。"},
	{label: "WORLD hybrid（過去比較用）", backend: "worldline-hybrid", description: "純粋な研究・回帰確認用です。通常の音声生成には使用しません。"},
}

type message struct {
	HWnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      point
	Private uint32
}

type windowClassEx struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   uintptr
	Icon       uintptr
	Cursor     uintptr
	Background uintptr
	MenuName   *uint16
	ClassName  *uint16
	IconSmall  uintptr
}

func main() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	flag.StringVar(&initialVoicebank, "voicebank", "", "initial voicebank directory")
	flag.StringVar(&voiceDirectory, "voice-dir", "voice", "directory containing voicebanks")
	flag.StringVar(&initialText, "text", "あらゆる現実をすべて自分のほうへねじ曲げたのだ。", "initial text")
	flag.StringVar(&initialOutput, "out", "output.wav", "initial output WAV")
	flag.Parse()
	closeLog := initializeLog()
	defer closeLog()

	comResult, _, comErr := coInitializeEx.Call(0, 2)
	if int32(comResult) < 0 {
		log.Printf("CoInitializeEx failed: HRESULT=0x%08X error=%v", uint32(comResult), comErr)
	} else {
		defer coUninitialize.Call()
	}

	instance, _, _ := getModuleHandle.Call(0)
	cursor, _, _ := loadCursor.Call(0, 32512)
	className := windowsString("UtauTTSWindow")
	class := windowClassEx{
		Size: uint32(unsafe.Sizeof(windowClassEx{})), WndProc: syscall.NewCallback(windowProc),
		Instance: instance, Cursor: cursor, Background: colorBtnFace + 1, ClassName: &className[0],
	}
	if result, _, callErr := registerClassEx.Call(uintptr(unsafe.Pointer(&class))); result == 0 {
		fatalMessage(fmt.Errorf("RegisterClassExW: %v", callErr))
		return
	}

	title := windowsString("UtauTTS")
	mainWindow, _, _ = createWindowEx.Call(
		0, uintptr(unsafe.Pointer(&className[0])), uintptr(unsafe.Pointer(&title[0])),
		wsOverlappedWindow, 120, 100, 780, 610, 0, 0, instance, 0,
	)
	runtime.KeepAlive(className)
	runtime.KeepAlive(title)
	if mainWindow == 0 {
		fatalMessage(fmt.Errorf("ウィンドウを作成できません"))
		return
	}
	if err := createControls(mainWindow, instance); err != nil {
		fatalMessage(err)
		return
	}
	showWindow.Call(mainWindow, 5)
	updateWindow.Call(mainWindow)

	var msg message
	for {
		result, _, callErr := getMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(result) == -1 {
			log.Printf("GetMessageW failed: %v", callErr)
			break
		}
		if int32(result) <= 0 {
			break
		}
		translateMsg.Call(uintptr(unsafe.Pointer(&msg)))
		dispatchMsg.Call(uintptr(unsafe.Pointer(&msg)))
	}
}

func createControls(parent, instance uintptr) error {
	const (
		singleControlHeight = 30
		comboDropHeight     = 180
		labelHeight         = 20
	)
	font, _, _ := getStockObject.Call(defaultFont)
	label(parent, instance, "ボイスバンク（voiceディレクトリ）", 20, 18, 300, labelHeight)
	voicebankCombo := control(wsExClientEdge, "COMBOBOX", "", wsChild|wsVisible|wsTabStop|wsVScroll|cbsDropdownList, 20, 42, 630, comboDropHeight, parent, idVoicebank, instance)
	setComboItemHeight(voicebankCombo, singleControlHeight-6)
	reloadButton = control(0, "BUTTON", "再読込", wsChild|wsVisible|wsTabStop, 660, 42, 90, singleControlHeight, parent, idReload, instance)

	label(parent, instance, "文章", 20, 82, 120, labelHeight)
	control(wsExClientEdge, "EDIT", initialText, wsChild|wsVisible|wsTabStop|wsVScroll|esMultiline|esAutoVScroll|esWantReturn, 20, 106, 730, 190, parent, idText, instance)

	label(parent, instance, "音声モード", 20, 306, 95, labelHeight)
	rendererCombo := control(0, "COMBOBOX", "", wsChild|wsVisible|wsTabStop|cbsDropdownList, 115, 302, 350, comboDropHeight, parent, idRenderer, instance)
	setComboItemHeight(rendererCombo, singleControlHeight-6)
	for _, option := range rendererOptions {
		comboAdd(rendererCombo, option.label)
	}
	sendMessage.Call(rendererCombo, cbSetCurSel, 0, 0)

	label(parent, instance, "従来抑揚 0〜1", 485, 306, 120, labelHeight)
	control(wsExClientEdge, "EDIT", "0", wsChild|wsVisible|wsTabStop, 610, 302, 55, singleControlHeight, parent, idIntonate, instance)
	label(parent, instance, "旧実験値", 675, 306, 75, labelHeight)
	rendererInfoLabel = control(0, "STATIC", "", wsChild|wsVisible, 20, 340, 730, 42, parent, idRendererInfo, instance)
	updateRendererInfo(parent)

	label(parent, instance, "保存先WAV", 20, 394, 90, labelHeight)
	control(wsExClientEdge, "EDIT", initialOutput, wsChild|wsVisible|wsTabStop|esReadOnly, 110, 390, 640, singleControlHeight, parent, idOutput, instance)

	generateButton = control(0, "BUTTON", "生成", wsChild|wsVisible|wsTabStop, 20, 438, 110, singleControlHeight, parent, idGenerate, instance)
	playButton = control(0, "BUTTON", "再生", wsChild|wsVisible|wsTabStop, 140, 438, 110, singleControlHeight, parent, idPlay, instance)
	stopButton = control(0, "BUTTON", "停止", wsChild|wsVisible|wsTabStop, 260, 438, 110, singleControlHeight, parent, idStop, instance)
	saveButton = control(0, "BUTTON", "名前を付けて保存", wsChild|wsVisible|wsTabStop, 380, 438, 170, singleControlHeight, parent, idSave, instance)
	statusLabel = label(parent, instance, "準備完了", 20, 486, 730, 40)
	for _, handle := range controls {
		sendMessage.Call(handle, wmSetFont, font, 1)
	}
	for _, id := range []int{idVoicebank, idReload, idText, idRenderer, idRendererInfo, idIntonate, idOutput, idGenerate, idPlay, idStop, idSave} {
		if child(parent, id) == 0 {
			return fmt.Errorf("GUIコントロールを作成できませんでした: id=%d", id)
		}
	}
	if statusLabel == 0 {
		return fmt.Errorf("GUIステータス欄を作成できませんでした")
	}
	enableWindow.Call(playButton, 0)
	enableWindow.Call(stopButton, 0)
	enableWindow.Call(saveButton, 0)
	postMessage.Call(parent, wmInitializeVoicebanks, 0, 0)
	return nil
}

func windowProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) (result uintptr) {
	defer func() {
		if recovered := recover(); recovered != nil {
			_ = logRecoveredPanic("GUI処理", recovered)
			if statusLabel != 0 {
				setText(statusLabel, "内部エラー。ログを確認してください")
			}
			result = 0
		}
	}()
	switch msg {
	case wmCommand:
		commandID := int(wParam & 0xffff)
		notification := int((wParam >> 16) & 0xffff)
		if commandID == idRenderer && notification == cbnSelChange {
			updateRendererInfo(hwnd)
			return 0
		}
		switch commandID {
		case idReload:
			refreshVoicebanks(hwnd)
		case idGenerate:
			startSynthesis(hwnd)
		case idPlay:
			playOutput(hwnd)
		case idStop:
			stopOutput(hwnd)
		case idSave:
			saveOutput(hwnd)
		}
		return 0
	case wmSynthesisDone:
		finishSynthesis(hwnd)
		return 0
	case wmInitializeVoicebanks:
		refreshVoicebanks(hwnd)
		return 0
	case wmVoicebanksDone:
		finishVoicebankRefresh(hwnd)
		return 0
	case wmDestroy:
		stopPlayback()
		completionMutex.Lock()
		preview := previewPath
		previewPath = ""
		completionMutex.Unlock()
		removePreviewFile(preview)
		postQuitMessage.Call(0)
		return 0
	}
	result, _, _ = defWindowProc.Call(hwnd, uintptr(msg), wParam, lParam)
	return result
}

func startSynthesis(hwnd uintptr) {
	if synthesisRunning {
		return
	}
	voicebankPath := ""
	selected, _, _ := sendMessage.Call(child(hwnd, idVoicebank), cbGetCurSel, 0, 0)
	if int(selected) >= 0 && int(selected) < len(availableBanks) {
		voicebankPath = availableBanks[int(selected)].Path
	}
	text := strings.TrimSpace(windowText(child(hwnd, idText)))
	strength, err := strconv.ParseFloat(strings.TrimSpace(windowText(child(hwnd, idIntonate))), 64)
	var missing []string
	if voicebankPath == "" {
		missing = append(missing, "ボイスバンク")
	}
	if text == "" {
		missing = append(missing, "文章")
	}
	if len(missing) > 0 {
		showError(hwnd, fmt.Errorf("未入力: %s", strings.Join(missing, "、")))
		return
	}
	if err != nil || strength < 0 || strength > 1 {
		showError(hwnd, fmt.Errorf("イントネーションは0から1で指定してください"))
		return
	}
	selectedRenderer, _, _ := sendMessage.Call(child(hwnd, idRenderer), cbGetCurSel, 0, 0)
	renderer := rendererBackend(int(selectedRenderer))

	stopPlayback()
	enableWindow.Call(generateButton, 0)
	enableWindow.Call(playButton, 0)
	enableWindow.Call(stopButton, 0)
	enableWindow.Call(saveButton, 0)
	synthesisRunning = true
	setText(statusLabel, "生成中: "+rendererOptionAt(int(selectedRenderer)).label)
	go func() {
		var generated *audio.PCM
		var preview string
		synthErr := runSafely("音声合成", func() error {
			result, err := tts.Synthesize(tts.Config{
				VoicebankPath: voicebankPath, Text: text, Renderer: renderer, IntonationStrength: strength,
			})
			if err != nil {
				return err
			}
			preview, err = writePreviewFile(result.Audio)
			if err != nil {
				return err
			}
			generated = result.Audio
			return nil
		})
		if synthErr == nil {
			completionMutex.Lock()
			oldPreview := previewPath
			lastAudio = generated
			previewPath = preview
			lastOutput = ""
			completionMutex.Unlock()
			removePreviewFile(oldPreview)
		} else {
			removePreviewFile(preview)
		}
		completionMutex.Lock()
		completionError = synthErr
		completionMutex.Unlock()
		postMessage.Call(hwnd, wmSynthesisDone, 0, 0)
	}()
}

func rendererBackend(index int) string {
	return rendererOptionAt(index).backend
}

func rendererOptionAt(index int) rendererOption {
	if index < 0 || index >= len(rendererOptions) {
		return rendererOptions[0]
	}
	return rendererOptions[index]
}

func updateRendererInfo(hwnd uintptr) {
	combo := child(hwnd, idRenderer)
	selected, _, _ := sendMessage.Call(combo, cbGetCurSel, 0, 0)
	setText(rendererInfoLabel, rendererOptionAt(int(selected)).description)
}

func finishSynthesis(hwnd uintptr) {
	completionMutex.Lock()
	err := completionError
	completionMutex.Unlock()
	synthesisRunning = false
	enableWindow.Call(generateButton, 1)
	if err != nil {
		enableWindow.Call(playButton, 0)
		enableWindow.Call(stopButton, 0)
		enableWindow.Call(saveButton, 0)
		setText(statusLabel, "生成失敗")
		showError(hwnd, err)
		return
	}
	enableWindow.Call(playButton, 1)
	enableWindow.Call(stopButton, 0)
	enableWindow.Call(saveButton, 1)
	setText(statusLabel, "生成完了。保存または再生できます")
}

func playOutput(hwnd uintptr) {
	completionMutex.Lock()
	path := previewPath
	if path == "" {
		path = lastOutput
	}
	completionMutex.Unlock()
	if _, err := os.Stat(path); err != nil {
		showError(hwnd, fmt.Errorf("再生するWAVがありません"))
		return
	}
	if err := startPlayback(path); err != nil {
		showError(hwnd, err)
		return
	}
	enableWindow.Call(stopButton, 1)
	setText(statusLabel, "再生中...")
}

func stopOutput(hwnd uintptr) {
	stopPlayback()
	enableWindow.Call(stopButton, 0)
	setText(statusLabel, "再生を停止しました")
}

func saveOutput(hwnd uintptr) {
	completionMutex.Lock()
	pcm := lastAudio
	suggestedPath := strings.TrimSpace(windowText(child(hwnd, idOutput)))
	completionMutex.Unlock()
	if pcm == nil {
		showError(hwnd, fmt.Errorf("保存する音声がありません"))
		return
	}
	path, err := saveAudioDialog(hwnd, pcm, suggestedPath)
	if err != nil {
		showError(hwnd, err)
		return
	}
	if path == "" {
		return
	}
	completionMutex.Lock()
	lastOutput = path
	completionMutex.Unlock()
	setText(child(hwnd, idOutput), path)
	setText(statusLabel, "保存完了: "+path)
}

func control(exStyle uintptr, class, text string, style uintptr, x, y, width, height int, parent uintptr, id int, instance uintptr) uintptr {
	classBuffer := windowsString(class)
	textBuffer := windowsString(text)
	handle, _, callErr := createWindowEx.Call(
		exStyle, uintptr(unsafe.Pointer(&classBuffer[0])), uintptr(unsafe.Pointer(&textBuffer[0])), style,
		uintptr(x), uintptr(y), uintptr(width), uintptr(height), parent, uintptr(id), instance, 0,
	)
	runtime.KeepAlive(classBuffer)
	runtime.KeepAlive(textBuffer)
	if handle != 0 {
		controls = append(controls, handle)
	} else {
		log.Printf("CreateWindowExW failed: class=%q id=%d error=%v", class, id, callErr)
	}
	return handle
}

func label(parent, instance uintptr, text string, x, y, width, height int) uintptr {
	return control(0, "STATIC", text, wsChild|wsVisible, x, y, width, height, parent, 0, instance)
}

func comboAdd(combo uintptr, value string) {
	buffer := windowsString(value)
	sendMessage.Call(combo, cbAddString, 0, uintptr(unsafe.Pointer(&buffer[0])))
	runtime.KeepAlive(buffer)
}

func setComboItemHeight(combo uintptr, height int) {
	if combo == 0 {
		return
	}
	// -1 targets the always-visible selection field; zero targets rows in the
	// expanded list. Keeping both at the same height avoids a visual jump when
	// the list opens.
	sendMessage.Call(combo, cbSetItemHeight, ^uintptr(0), uintptr(height))
	sendMessage.Call(combo, cbSetItemHeight, 0, uintptr(height))
}

func child(parent uintptr, id int) uintptr {
	handle, _, _ := getDlgItem.Call(parent, uintptr(id))
	return handle
}

func windowText(hwnd uintptr) string {
	buffer := make([]uint16, 32768)
	sendMessage.Call(hwnd, wmGetText, uintptr(len(buffer)), uintptr(unsafe.Pointer(&buffer[0])))
	runtime.KeepAlive(buffer)
	return syscall.UTF16ToString(buffer)
}

func setText(hwnd uintptr, value string) {
	buffer := windowsString(value)
	setWindowText.Call(hwnd, uintptr(unsafe.Pointer(&buffer[0])))
	runtime.KeepAlive(buffer)
}

func showError(hwnd uintptr, err error) {
	log.Printf("error: %v", err)
	message := windowsString(err.Error())
	title := windowsString("UtauTTS")
	messageBox.Call(hwnd, uintptr(unsafe.Pointer(&message[0])), uintptr(unsafe.Pointer(&title[0])), mbIconError)
	runtime.KeepAlive(message)
	runtime.KeepAlive(title)
}

func fatalMessage(err error) {
	showError(0, err)
}
