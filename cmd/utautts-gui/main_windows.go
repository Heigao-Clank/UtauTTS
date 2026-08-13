//go:build windows

package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"utautts/internal/audio"
	"utautts/internal/plugin"
	"utautts/internal/render"
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
	wmExportDone           = 0x8004
	wmDrawItem             = 0x002B
	wmLButtonDown          = 0x0201
	wmLButtonUp            = 0x0202
	wmMouseMove            = 0x0200
	wmSize                 = 0x0005
	enChange               = 0x0300

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

	cbAddString       = 0x0143
	cbGetCurSel       = 0x0147
	cbResetContent    = 0x014B
	cbSetCurSel       = 0x014E
	cbSetItemHeight   = 0x0153
	cbnSelChange      = 1
	lbAddString       = 0x0180
	lbDeleteString    = 0x0182
	lbResetContent    = 0x0184
	lbGetCurSel       = 0x0188
	lbSetCurSel       = 0x0186
	lbnSelChange      = 1
	lbSetItemHeight   = 0x01A0
	lbsNotify         = 0x0001
	lbsOwnerDrawFixed = 0x0010
	lbsHasStrings     = 0x0040
	defaultFont       = 17
	colorBtnFace      = 15

	idVoicebank       = 1001
	idReload          = 1002
	idText            = 1003
	idRenderer        = 1004
	idIntonate        = 1005
	idOutput          = 1006
	idGenerate        = 1007
	idPlay            = 1008
	idSave            = 1009
	idStop            = 1010
	idRendererInfo    = 1011
	idVoiceSummary    = 1012
	idAdvanced        = 1013
	idProsodyModel    = 1014
	idManualPitch     = 1015
	idUtteranceList   = 1016
	idUtteranceAdd    = 1017
	idUtteranceDelete = 1018
	idProjectSave     = 1019
	idProjectOpen     = 1020
	idMenuProjectOpen = 4101
	idMenuProjectSave = 4102
	idMenuAudioSave   = 4103
	idMenuSelectedWav = 4105
	idMenuAllWav      = 4106
	idMenuExit        = 4104
	idMenuLicense     = 4201
	idMenuGitHub      = 4202
	idMenuAbout       = 4203

	menuString    = 0x0000
	menuPopup     = 0x0010
	menuSeparator = 0x0800

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
	getClientRect   = user32.NewProc("GetClientRect")
	enableWindow    = user32.NewProc("EnableWindow")
	messageBox      = user32.NewProc("MessageBoxW")
	loadCursor      = user32.NewProc("LoadCursorW")
	setFocus        = user32.NewProc("SetFocus")
	createMenu      = user32.NewProc("CreateMenu")
	createPopupMenu = user32.NewProc("CreatePopupMenu")
	appendMenu      = user32.NewProc("AppendMenuW")
	setMenu         = user32.NewProc("SetMenu")
	drawMenuBar     = user32.NewProc("DrawMenuBar")
	shellExecute    = syscall.NewLazyDLL("shell32.dll").NewProc("ShellExecuteW")

	getModuleHandle = kernel32.NewProc("GetModuleHandleW")
	rtlMoveMemory   = kernel32.NewProc("RtlMoveMemory")
	getStockObject  = gdi32.NewProc("GetStockObject")
	coInitializeEx  = ole32.NewProc("CoInitializeEx")
	coUninitialize  = ole32.NewProc("CoUninitialize")

	mainWindow             uintptr
	generateButton         uintptr
	playButton             uintptr
	stopButton             uintptr
	saveButton             uintptr
	reloadButton           uintptr
	advancedButton         uintptr
	manualPitchButton      uintptr
	pitchGraphPanel        uintptr
	projectSaveButton      uintptr
	projectOpenButton      uintptr
	utteranceListControl   uintptr
	mainSplitterX          = 270
	mainSplitterDragging   bool
	pitchSplitterX         = 650
	pitchSplitterDragging  bool
	mainClientWidth        = 900
	mainClientHeight       = 700
	statusLabel            uintptr
	rendererInfoLabel      uintptr
	voiceSummaryLabel      uintptr
	controls               []uintptr
	completionMutex        sync.Mutex
	completionError        error
	exportError            error
	exportMessage          string
	synthesisRunning       bool
	lastOutput             string
	lastAudio              *audio.PCM
	previewPath            string
	initialVoicebank       string
	voiceDirectory         string
	availableBanks         []voicebank.Summary
	availableProsodyModels []prosodyModelOption
	editor                 *editorState
	initialText            string
	initialOutput          string
)

type point struct {
	X int32
	Y int32
}

type rendererOption struct {
	ID             string
	label          string
	backend        string
	description    string
	executable     string
	executableKind string
	configurable   bool
	framePitch     bool
}

var (
	rendererOptions        []rendererOption
	defaultRendererBackend string
)

func defaultRendererIndex() int {
	return 0
}

func defaultProsodyModelIndex() int {
	return 0
}

func loadRendererPlugins() {
	directories, _ := plugin.DefaultDirectories()
	items, err := plugin.DiscoverRenderers(directories, render.IsKnownRenderer)
	if err != nil {
		log.Printf("renderer plugin discovery: %v", err)
	}
	rendererOptions = make([]rendererOption, 0, len(items))
	for _, item := range items {
		rendererOptions = append(rendererOptions, rendererOption{ID: item.ID, label: item.DisplayName, backend: item.Backend, description: item.Description, framePitch: item.Capabilities.FramePitch})
	}
	if len(rendererOptions) > 0 {
		defaultRendererBackend = rendererOptions[0].backend
	}
}

func init() {
	loadRendererPlugins()
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
	loadGUIConfiguration()
	editor = newEditorState(initialText)
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
	if err := registerVoicebankInfoClass(instance, cursor); err != nil {
		fatalMessage(err)
		return
	}
	if err := registerSettingsClass(instance, cursor); err != nil {
		fatalMessage(err)
		return
	}
	if err := registerManualPitchClass(instance, cursor); err != nil {
		fatalMessage(err)
		return
	}
	if err := registerPitchGraphClass(instance, cursor); err != nil {
		fatalMessage(err)
		return
	}

	title := windowsString("UtauTTS")
	mainWindow, _, _ = createWindowEx.Call(
		0, uintptr(unsafe.Pointer(&className[0])), uintptr(unsafe.Pointer(&title[0])),
		wsOverlappedWindow, 120, 80, 900, 700, 0, 0, instance, 0,
	)
	runtime.KeepAlive(className)
	runtime.KeepAlive(title)
	if mainWindow == 0 {
		fatalMessage(fmt.Errorf("ウィンドウを作成できません"))
		return
	}
	installMainMenu(mainWindow)
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
	createVoicebankPortrait(parent, instance, 20, 42, 106)
	voicebankCombo := control(wsExClientEdge, "COMBOBOX", "", wsChild|wsVisible|wsTabStop|wsVScroll|cbsDropdownList, 145, 42, 625, comboDropHeight, parent, idVoicebank, instance)
	setComboItemHeight(voicebankCombo, singleControlHeight-6)
	reloadButton = control(0, "BUTTON", "再読込", wsChild|wsVisible|wsTabStop, 780, 42, 90, singleControlHeight, parent, idReload, instance)
	voiceSummaryLabel = control(0, "STATIC", "音源を読み込んでいます...", wsChild|wsVisible, 145, 82, 725, 76, parent, idVoiceSummary, instance)

	label(parent, instance, "発話", 20, 184, 240, labelHeight)
	utteranceList := control(wsExClientEdge, "LISTBOX", "", wsChild|wsVisible|wsTabStop|wsVScroll|lbsNotify|lbsOwnerDrawFixed|lbsHasStrings, 20, 208, 240, 170, parent, idUtteranceList, instance)
	utteranceListControl = utteranceList
	subclassUtteranceList(utteranceList)
	sendMessage.Call(utteranceList, lbSetItemHeight, 0, 54)
	control(0, "BUTTON", "＋ 発話追加", wsChild|wsVisible|wsTabStop, 20, 384, 110, 30, parent, idUtteranceAdd, instance)
	control(0, "BUTTON", "削除", wsChild|wsVisible|wsTabStop, 140, 384, 120, 30, parent, idUtteranceDelete, instance)
	label(parent, instance, "文章", 280, 184, 360, labelHeight)
	control(wsExClientEdge, "EDIT", initialText, wsChild|wsVisible|wsTabStop|wsVScroll|esMultiline|esAutoVScroll|esWantReturn, 280, 208, 590, 206, parent, idText, instance)
	refreshUtteranceList(parent, utteranceList)
	layoutMainTextArea(parent, 900, 700)
	pitchGraphPanel = createPitchGraphPanel(parent, instance)
	prepareManualPitchForCurrent(parent)

	label(parent, instance, "音声モード", 20, 416, 95, labelHeight)
	rendererCombo := control(0, "COMBOBOX", "", wsChild|wsVisible|wsTabStop|cbsDropdownList, 115, 412, 390, comboDropHeight, parent, idRenderer, instance)
	setComboItemHeight(rendererCombo, singleControlHeight-6)
	for _, option := range rendererOptions {
		comboAdd(rendererCombo, option.label)
	}
	sendMessage.Call(rendererCombo, cbSetCurSel, uintptr(defaultRendererIndex()), 0)

	label(parent, instance, "抑揚モデル", 530, 416, 90, labelHeight)
	prosodyCombo := control(0, "COMBOBOX", "", wsChild|wsVisible|wsTabStop|cbsDropdownList, 620, 412, 250, comboDropHeight, parent, idProsodyModel, instance)
	comboAdd(prosodyCombo, "なし（原音のピッチを維持）")
	availableProsodyModels = discoverProsodyModels()
	for _, option := range availableProsodyModels {
		comboAdd(prosodyCombo, option.Label)
	}
	sendMessage.Call(prosodyCombo, cbSetCurSel, uintptr(defaultProsodyModelIndex()), 0)
	rendererInfoLabel = control(0, "STATIC", "", wsChild|wsVisible, 20, 450, 665, 42, parent, idRendererInfo, instance)
	manualPitchButton = control(0, "BUTTON", "イントネーション編集", wsChild|wsVisible|wsTabStop, 700, 450, 170, singleControlHeight, parent, idManualPitch, instance)
	updateRendererInfo(parent)

	label(parent, instance, "保存先WAV", 20, 504, 90, labelHeight)
	control(wsExClientEdge, "EDIT", initialOutput, wsChild|wsVisible|wsTabStop|esReadOnly, 110, 500, 760, singleControlHeight, parent, idOutput, instance)

	generateButton = control(0, "BUTTON", "生成", wsChild|wsVisible|wsTabStop, 20, 548, 110, singleControlHeight, parent, idGenerate, instance)
	playButton = control(0, "BUTTON", "再生", wsChild|wsVisible|wsTabStop, 140, 548, 110, singleControlHeight, parent, idPlay, instance)
	stopButton = control(0, "BUTTON", "停止", wsChild|wsVisible|wsTabStop, 260, 548, 110, singleControlHeight, parent, idStop, instance)
	advancedButton = control(0, "BUTTON", "詳細設定", wsChild|wsVisible|wsTabStop, 380, 548, 145, singleControlHeight, parent, idAdvanced, instance)
	statusLabel = label(parent, instance, "準備完了", 20, 596, 850, 40)
	for _, handle := range controls {
		sendMessage.Call(handle, wmSetFont, font, 1)
	}

	setComboItemHeight(voicebankCombo, singleControlHeight-6)
	setComboItemHeight(rendererCombo, singleControlHeight-6)
	setComboItemHeight(prosodyCombo, singleControlHeight-6)
	matchControlHeight(reloadButton, voicebankCombo, 90)
	matchControlHeight(generateButton, rendererCombo, 110)
	matchControlHeight(playButton, rendererCombo, 110)
	matchControlHeight(stopButton, rendererCombo, 110)
	matchControlHeight(advancedButton, rendererCombo, 145)
	matchControlHeight(manualPitchButton, rendererCombo, 170)
	for _, id := range []int{idVoicebank, idVoicePortrait, idVoiceSummary, idReload, idText, idUtteranceList, idUtteranceAdd, idUtteranceDelete, idRenderer, idRendererInfo, idProsodyModel, idManualPitch, idOutput, idGenerate, idPlay, idStop, idAdvanced} {
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
	case wmSize:
		layoutMainTextArea(hwnd, int(lParam&0xffff), int((lParam>>16)&0xffff))
		return 0
	case wmLButtonDown:
		if pitchSplitterHit(lParam) {
			pitchSplitterDragging = true
			manualSetCapture.Call(hwnd)
			return 0
		}
		if mainSplitterHit(lParam) {
			mainSplitterDragging = true
			manualSetCapture.Call(hwnd)
			return 0
		}
	case wmMouseMove:
		if pitchSplitterDragging {
			pitchSplitterX = clampInt(int(int16(lParam&0xffff)), 500, maxInt(520, mainClientWidth-170))
			layoutMainTextArea(hwnd, 0, 0)
			return 0
		}
		if mainSplitterDragging {
			mainSplitterX = clampInt(int(int16(lParam&0xffff)), 180, 500)
			layoutMainTextArea(hwnd, 0, 0)
			return 0
		}
	case wmLButtonUp:
		if pitchSplitterDragging {
			pitchSplitterDragging = false
			manualReleaseCapture.Call()
			return 0
		}
		if mainSplitterDragging {
			mainSplitterDragging = false
			manualReleaseCapture.Call()
			return 0
		}
	case wmCommand:
		commandID := int(wParam & 0xffff)
		notification := int((wParam >> 16) & 0xffff)
		if commandID == idVoicebank && notification == cbnSelChange {
			if !syncingUtteranceControls {
				syncSelectedUtteranceControls(hwnd)
			}
			selected, _, _ := sendMessage.Call(child(hwnd, idVoicebank), cbGetCurSel, 0, 0)
			updateVoicebankPortrait(int(selected))
			updateVoicebankSummary(int(selected))
			return 0
		}
		if commandID == idRenderer && notification == cbnSelChange {
			if !syncingUtteranceControls {
				syncSelectedUtteranceControls(hwnd)
			}
			updateRendererInfo(hwnd)
			return 0
		}
		if commandID == idProsodyModel && notification == cbnSelChange {
			if !syncingUtteranceControls {
				syncSelectedUtteranceControls(hwnd)
			}
			selected, _, _ := sendMessage.Call(child(hwnd, idProsodyModel), cbGetCurSel, 0, 0)
			if model := prosodyModelAt(int(selected)); model != nil && len(model.RecommendedRenderers) > 0 {
				for index, option := range rendererOptions {
					if option.ID == model.RecommendedRenderers[0] || option.backend == model.RecommendedRenderers[0] {
						sendMessage.Call(child(hwnd, idRenderer), cbSetCurSel, uintptr(index), 0)
						break
					}
				}
			}
			updateRendererInfo(hwnd)
			return 0
		}
		if notification == 0 {
			switch commandID {
			case idMenuProjectOpen:
				openProject(hwnd)
				return 0
			case idMenuProjectSave:
				saveProject(hwnd)
				return 0
			case idMenuAudioSave:
				saveOutput(hwnd)
				return 0
			case idMenuSelectedWav:
				exportSelectedUtterance(hwnd)
				return 0
			case idMenuAllWav:
				exportAllUtterances(hwnd)
				return 0
			case idMenuExit:
				destroyWindow.Call(hwnd)
				return 0
			case idMenuLicense:
				showLicense(hwnd)
				return 0
			case idMenuGitHub:
				openGitHub(hwnd)
				return 0
			case idMenuAbout:
				showAbout(hwnd)
				return 0
			}
		}
		if commandID == idText && notification == enChange {
			syncSelectedUtteranceText(hwnd)
			refreshSelectedUtteranceListItem(hwnd)
			prepareManualPitchForCurrent(hwnd)
			invalidatePitchGraph()
			return 0
		}
		if commandID == idUtteranceList && notification == lbnSelChange {
			syncSelectedUtteranceText(hwnd)
			syncSelectedUtteranceControls(hwnd)
			storeSelectedDetailedSettings(advancedSettings)
			selected, _, _ := sendMessage.Call(child(hwnd, idUtteranceList), lbGetCurSel, 0, 0)
			if editor != nil && editor.selectIndex(int(selected)) {
				if selectedUtterance := editor.selected(); selectedUtterance != nil {
					activeManualPitch = selectedUtterance.ManualPitch
				}
				loadSelectedUtteranceText(hwnd)
				loadSelectedUtteranceControls(hwnd)
				loadSelectedDetailedSettings()
				prepareManualPitchForCurrent(hwnd)
				invalidatePitchGraph()
			}
			return 0
		}
		switch commandID {
		case idUtteranceAdd:
			addUtterance(hwnd)
		case idUtteranceDelete:
			deleteSelectedUtterance(hwnd)
		case idProjectSave:
			saveProject(hwnd)
		case idProjectOpen:
			openProject(hwnd)
		case idVoicePortrait:
			showSelectedVoicebankInfo(hwnd)
		case idReload:
			refreshVoicebanks(hwnd)
		case idAdvanced:
			showSynthesisSettings(hwnd)
		case idManualPitch:
			showManualPitchEditor(hwnd)
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
	case wmExportDone:
		finishExport(hwnd)
		return 0
	case wmDrawItem:
		if drawUtteranceListItem(lParam) {
			return 1
		}
	case wmInitializeVoicebanks:
		refreshVoicebanks(hwnd)
		return 0
	case wmVoicebanksDone:
		finishVoicebankRefresh(hwnd)
		return 0
	case wmDestroy:
		stopPlayback()
		disposeVoicebankPortrait()
		disposeUtteranceBitmaps()
		if voiceInfoWindow != 0 {
			destroyWindow.Call(voiceInfoWindow)
		}
		if settingsWindow != 0 {
			destroyWindow.Call(settingsWindow)
		}
		if manualPitchWindow != 0 {
			destroyWindow.Call(manualPitchWindow)
		}
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

func layoutMainTextArea(hwnd uintptr, width, height int) {
	if width > 0 {
		mainClientWidth = width
	}
	if height > 0 {
		mainClientHeight = height
	}
	list := child(hwnd, idUtteranceList)
	text := child(hwnd, idText)
	if list == 0 || text == 0 {
		return
	}
	listWidth := clampInt(mainSplitterX-20, 140, 460)
	textX := mainSplitterX + 10
	textWidth := clampInt(mainClientWidth-textX-30, 180, 1600)
	textWidth = clampInt(pitchSplitterX-textX-10, 180, 1600)
	moveWindow.Call(list, 20, 208, uintptr(listWidth), 170, 1)
	moveWindow.Call(child(hwnd, idUtteranceAdd), 20, 384, 110, 30, 1)
	moveWindow.Call(child(hwnd, idUtteranceDelete), 140, 384, uintptr(maxInt(80, listWidth-120)), 30, 1)
	moveWindow.Call(text, uintptr(textX), 208, uintptr(textWidth), 206, 1)
	if pitchGraphPanel != 0 {
		graphX := pitchSplitterX + 10
		graphWidth := maxInt(150, mainClientWidth-graphX-30)
		moveWindow.Call(pitchGraphPanel, uintptr(graphX), 208, uintptr(graphWidth), 206, 1)
	}
}

func mainSplitterHit(lParam uintptr) bool {
	x := int(int16(lParam & 0xffff))
	y := int(int16((lParam >> 16) & 0xffff))
	return y >= 200 && y <= 420 && absInt(x-mainSplitterX) <= 6
}

func pitchSplitterHit(lParam uintptr) bool {
	x := int(int16(lParam & 0xffff))
	y := int(int16((lParam >> 16) & 0xffff))
	return y >= 200 && y <= 420 && absInt(x-pitchSplitterX) <= 6
}

func clampInt(value, low, high int) int {
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}

func startSynthesis(hwnd uintptr) {
	if synthesisRunning {
		return
	}
	states, err := snapshotUtterances(hwnd)
	if err != nil {
		showError(hwnd, err)
		return
	}

	stopPlayback()
	enableWindow.Call(generateButton, 0)
	enableWindow.Call(playButton, 0)
	enableWindow.Call(stopButton, 0)
	enableWindow.Call(saveButton, 0)
	synthesisRunning = true
	setText(statusLabel, fmt.Sprintf("全%d発話を生成中...", len(states)))
	go func() {
		var generated *audio.PCM
		var preview string
		started := time.Now()
		synthErr := runSafely("音声合成", func() error {
			result, err := synthesizeUtterances(states)
			if err != nil {
				return err
			}
			preview, err = writePreviewFile(result)
			if err != nil {
				return err
			}
			generated = result
			return nil
		})
		log.Printf("synthesis finished: utterances=%d elapsed=%s error=%v", len(states), time.Since(started).Round(time.Millisecond), synthErr)
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
	description := rendererOptionAt(int(selected)).description
	modelSelection, _, _ := sendMessage.Call(child(hwnd, idProsodyModel), cbGetCurSel, 0, 0)
	if model := prosodyModelAt(int(modelSelection)); model != nil {
		description += " / 抑揚: " + model.Label
	} else {
		description += " / 抑揚モデルなし"
	}
	setText(rendererInfoLabel, description)
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
