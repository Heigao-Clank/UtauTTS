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
	cbsDropdownList = 0x0003

	cbAddString    = 0x0143
	cbGetCurSel    = 0x0147
	cbResetContent = 0x014B
	cbSetCurSel    = 0x014E
	defaultFont    = 17
	colorBtnFace   = 15

	idVoicebank = 1001
	idReload    = 1002
	idText      = 1003
	idRenderer  = 1004
	idIntonate  = 1005
	idOutput    = 1006
	idGenerate  = 1007
	idPlay      = 1008

	mbIconError = 0x10
	swNormal    = 1
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	shell32  = syscall.NewLazyDLL("shell32.dll")
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
	shellExecute    = shell32.NewProc("ShellExecuteW")
	coInitializeEx  = ole32.NewProc("CoInitializeEx")
	coUninitialize  = ole32.NewProc("CoUninitialize")

	mainWindow       uintptr
	generateButton   uintptr
	playButton       uintptr
	reloadButton     uintptr
	statusLabel      uintptr
	controls         []uintptr
	completionMutex  sync.Mutex
	completionError  error
	synthesisRunning bool
	lastOutput       string
	initialVoicebank string
	voiceDirectory   string
	availableBanks   []voicebank.Summary
	initialText      string
	initialOutput    string
)

type point struct {
	X int32
	Y int32
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
		wsOverlappedWindow, 120, 100, 700, 535, 0, 0, instance, 0,
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
	font, _, _ := getStockObject.Call(defaultFont)
	label(parent, instance, "ボイスバンク（voiceディレクトリ）", 20, 18, 300, 22)
	control(wsExClientEdge, "COMBOBOX", "", wsChild|wsVisible|wsTabStop|wsVScroll|cbsDropdownList, 20, 42, 550, 300, parent, idVoicebank, instance)
	reloadButton = control(0, "BUTTON", "再読込", wsChild|wsVisible|wsTabStop, 580, 41, 90, 29, parent, idReload, instance)

	label(parent, instance, "文章", 20, 82, 120, 22)
	control(wsExClientEdge, "EDIT", initialText, wsChild|wsVisible|wsTabStop|wsVScroll|esMultiline|esAutoVScroll|esWantReturn, 20, 106, 650, 190, parent, idText, instance)

	label(parent, instance, "レンダラ", 20, 312, 90, 22)
	rendererCombo := control(0, "COMBOBOX", "", wsChild|wsVisible|wsTabStop|cbsDropdownList, 110, 308, 200, 200, parent, idRenderer, instance)
	comboAdd(rendererCombo, "worldline-hybrid")
	comboAdd(rendererCombo, "waveform")
	comboAdd(rendererCombo, "waveform-long (実験的)")
	sendMessage.Call(rendererCombo, cbSetCurSel, 0, 0)

	label(parent, instance, "イントネーション", 340, 312, 120, 22)
	control(wsExClientEdge, "EDIT", "0.6", wsChild|wsVisible|wsTabStop, 465, 308, 70, 27, parent, idIntonate, instance)

	label(parent, instance, "出力WAV", 20, 354, 90, 22)
	control(wsExClientEdge, "EDIT", initialOutput, wsChild|wsVisible|wsTabStop, 110, 350, 560, 27, parent, idOutput, instance)

	generateButton = control(0, "BUTTON", "生成", wsChild|wsVisible|wsTabStop, 20, 405, 120, 38, parent, idGenerate, instance)
	playButton = control(0, "BUTTON", "再生", wsChild|wsVisible|wsTabStop, 150, 405, 120, 38, parent, idPlay, instance)
	statusLabel = label(parent, instance, "準備完了", 290, 414, 380, 22)
	for _, handle := range controls {
		sendMessage.Call(handle, wmSetFont, font, 1)
	}
	for _, id := range []int{idVoicebank, idReload, idText, idRenderer, idIntonate, idOutput, idGenerate, idPlay} {
		if child(parent, id) == 0 {
			return fmt.Errorf("GUIコントロールを作成できませんでした: id=%d", id)
		}
	}
	if statusLabel == 0 {
		return fmt.Errorf("GUIステータス欄を作成できませんでした")
	}
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
		switch int(wParam & 0xffff) {
		case idReload:
			refreshVoicebanks(hwnd)
		case idGenerate:
			startSynthesis(hwnd)
		case idPlay:
			playOutput(hwnd)
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
	output := strings.TrimSpace(windowText(child(hwnd, idOutput)))
	strength, err := strconv.ParseFloat(strings.TrimSpace(windowText(child(hwnd, idIntonate))), 64)
	var missing []string
	if voicebankPath == "" {
		missing = append(missing, "ボイスバンク")
	}
	if text == "" {
		missing = append(missing, "文章")
	}
	if output == "" {
		missing = append(missing, "出力先")
	}
	if len(missing) > 0 {
		showError(hwnd, fmt.Errorf("未入力: %s", strings.Join(missing, "、")))
		return
	}
	if err != nil || strength < 0 || strength > 1 {
		showError(hwnd, fmt.Errorf("イントネーションは0から1で指定してください"))
		return
	}
	renderer := "worldline-hybrid"
	if selected, _, _ := sendMessage.Call(child(hwnd, idRenderer), cbGetCurSel, 0, 0); selected == 1 {
		renderer = "waveform"
	} else if selected == 2 {
		renderer = "waveform-long"
	}

	enableWindow.Call(generateButton, 0)
	enableWindow.Call(playButton, 0)
	synthesisRunning = true
	setText(statusLabel, "生成中...")
	go func() {
		synthErr := runSafely("音声合成", func() error {
			result, err := tts.Synthesize(tts.Config{
				VoicebankPath: voicebankPath, Text: text, Renderer: renderer, IntonationStrength: strength,
			})
			if err != nil {
				return err
			}
			return audio.WriteWav(output, result.Audio)
		})
		completionMutex.Lock()
		completionError = synthErr
		if synthErr == nil {
			lastOutput = output
		}
		completionMutex.Unlock()
		postMessage.Call(hwnd, wmSynthesisDone, 0, 0)
	}()
}

func finishSynthesis(hwnd uintptr) {
	completionMutex.Lock()
	err := completionError
	output := lastOutput
	completionMutex.Unlock()
	synthesisRunning = false
	enableWindow.Call(generateButton, 1)
	enableWindow.Call(playButton, 1)
	if err != nil {
		setText(statusLabel, "生成失敗")
		showError(hwnd, err)
		return
	}
	setText(statusLabel, "生成完了: "+output)
}

func playOutput(hwnd uintptr) {
	path := strings.TrimSpace(windowText(child(hwnd, idOutput)))
	completionMutex.Lock()
	completedOutput := lastOutput
	completionMutex.Unlock()
	if completedOutput != "" {
		path = completedOutput
	}
	if _, err := os.Stat(path); err != nil {
		showError(hwnd, fmt.Errorf("再生するWAVがありません"))
		return
	}
	verb := windowsString("open")
	pathBuffer := windowsString(path)
	result, _, _ := shellExecute.Call(hwnd, uintptr(unsafe.Pointer(&verb[0])), uintptr(unsafe.Pointer(&pathBuffer[0])), 0, 0, swNormal)
	runtime.KeepAlive(verb)
	runtime.KeepAlive(pathBuffer)
	if result <= 32 {
		showError(hwnd, fmt.Errorf("WAVを開けませんでした"))
	}
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
