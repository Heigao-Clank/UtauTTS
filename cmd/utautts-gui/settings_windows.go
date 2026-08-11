//go:build windows

package main

import (
	"fmt"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"unsafe"

	"utautts/internal/tts"
	"utautts/internal/voicebank"
)

const (
	idSettingsReading           = 2101
	idSettingsTone              = 2102
	idSettingsMora              = 2103
	idSettingsPause             = 2104
	idSettingsRelease           = 2105
	idSettingsSelection         = 2106
	idSettingsPitchOnly         = 2107
	idSettingsApplyPitch        = 2108
	idSettingsProsody           = 2109
	idSettingsProsodyBrowse     = 2110
	idSettingsFeatures          = 2111
	idSettingsFeaturesBrowse    = 2112
	idSettingsFeatureCase       = 2113
	idSettingsJoin              = 2114
	idSettingsJoinBrowse        = 2115
	idSettingsJoinScale         = 2116
	idSettingsWorldline         = 2117
	idSettingsWorldlineBrowse   = 2118
	idSettingsBridge            = 2119
	idSettingsBridgeBrowse      = 2120
	idSettingsResampler         = 2121
	idSettingsResamplerBrowse   = 2122
	idSettingsBoundaryMS        = 2123
	idSettingsBoundaryThreshold = 2124
	idSettingsApply             = 2125
	idSettingsIntonation        = 2126

	bsAutoCheckbox = 0x0003
	bmGetCheck     = 0x00F0
	bmSetCheck     = 0x00F1
	bstChecked     = 1
)

type synthesisSettings struct {
	Reading                 string
	Tone                    string
	MoraMS                  float64
	PauseMS                 float64
	ReleaseMS               float64
	Selection               voicebank.SelectionMode
	IntonationStrength      float64
	ProsodyPitchOnly        bool
	ApplyPitch              bool
	JoinModelPath           string
	JoinScale               float64
	WorldlinePath           string
	WorldlineBridgePath     string
	UTAUResamplerPath       string
	BoundaryBridgeMS        float64
	BoundaryBridgeThreshold float64
}

var (
	advancedSettings  = defaultSynthesisSettings()
	settingsWindow    uintptr
	settingsClassName []uint16
)

func defaultSynthesisSettings() synthesisSettings {
	return synthesisSettings{
		Tone: "C4", MoraMS: 140, PauseMS: 180, ReleaseMS: 20,
		Selection: voicebank.SelectionViterbi, ProsodyPitchOnly: true, ApplyPitch: true,
	}
}

func registerSettingsClass(instance, cursor uintptr) error {
	settingsClassName = windowsString("UtauTTSSynthesisSettingsWindow")
	class := windowClassEx{
		Size: uint32(unsafe.Sizeof(windowClassEx{})), WndProc: syscall.NewCallback(settingsProc),
		Instance: instance, Cursor: cursor, Background: colorBtnFace + 1, ClassName: &settingsClassName[0],
	}
	if result, _, callErr := registerClassEx.Call(uintptr(unsafe.Pointer(&class))); result == 0 {
		return fmt.Errorf("詳細設定ウィンドウを登録できません: %v", callErr)
	}
	return nil
}

func showSynthesisSettings(parent uintptr) {
	if settingsWindow != 0 {
		setForegroundWindow.Call(settingsWindow)
		return
	}
	instance, _, _ := getModuleHandle.Call(0)
	title := windowsString("UtauTTS 詳細設定")
	settingsWindow, _, _ = createWindowEx.Call(0, uintptr(unsafe.Pointer(&settingsClassName[0])), uintptr(unsafe.Pointer(&title[0])),
		wsOverlappedWindow, 210, 90, 790, 650, parent, 0, instance, 0)
	runtime.KeepAlive(title)
	if settingsWindow == 0 {
		showError(parent, fmt.Errorf("詳細設定ウィンドウを作成できません"))
		return
	}
	createSettingsControls(settingsWindow, instance)
	showWindow.Call(settingsWindow, 5)
	updateWindow.Call(settingsWindow)
	setForegroundWindow.Call(settingsWindow)
}

func createSettingsControls(parent, instance uintptr) {
	firstControl := len(controls)
	font, _, _ := getStockObject.Call(defaultFont)
	row := func(caption string, id int, value string, y int) uintptr {
		label(parent, instance, caption, 20, y+5, 145, 22)
		return control(wsExClientEdge, "EDIT", value, wsChild|wsVisible|wsTabStop, 170, y, 575, 28, parent, id, instance)
	}
	row("読み（空欄で自動）", idSettingsReading, advancedSettings.Reading, 18)
	label(parent, instance, "音高", 20, 58, 50, 22)
	control(wsExClientEdge, "EDIT", advancedSettings.Tone, wsChild|wsVisible|wsTabStop, 70, 53, 75, 28, parent, idSettingsTone, instance)
	label(parent, instance, "モーラ ms", 170, 58, 75, 22)
	control(wsExClientEdge, "EDIT", numberText(advancedSettings.MoraMS), wsChild|wsVisible|wsTabStop, 250, 53, 75, 28, parent, idSettingsMora, instance)
	label(parent, instance, "休止 ms", 350, 58, 65, 22)
	control(wsExClientEdge, "EDIT", numberText(advancedSettings.PauseMS), wsChild|wsVisible|wsTabStop, 420, 53, 75, 28, parent, idSettingsPause, instance)
	label(parent, instance, "末尾 ms", 520, 58, 65, 22)
	control(wsExClientEdge, "EDIT", numberText(advancedSettings.ReleaseMS), wsChild|wsVisible|wsTabStop, 590, 53, 75, 28, parent, idSettingsRelease, instance)

	label(parent, instance, "音素選択", 20, 98, 145, 22)
	selection := control(0, "COMBOBOX", "", wsChild|wsVisible|wsTabStop|cbsDropdownList, 170, 93, 245, 130, parent, idSettingsSelection, instance)
	for _, value := range []string{"Viterbi（推奨）", "Greedy", "Target only"} {
		comboAdd(selection, value)
	}
	sendMessage.Call(selection, cbSetCurSel, uintptr(selectionIndex(advancedSettings.Selection)), 0)
	label(parent, instance, "従来抑揚", 440, 98, 75, 22)
	control(wsExClientEdge, "EDIT", numberText(advancedSettings.IntonationStrength), wsChild|wsVisible|wsTabStop, 515, 93, 55, 28, parent, idSettingsIntonation, instance)
	pitchOnly := control(0, "BUTTON", "ピッチだけ", wsChild|wsVisible|wsTabStop|bsAutoCheckbox, 580, 94, 100, 26, parent, idSettingsPitchOnly, instance)
	applyPitch := control(0, "BUTTON", "ピッチ処理", wsChild|wsVisible|wsTabStop|bsAutoCheckbox, 680, 94, 90, 26, parent, idSettingsApplyPitch, instance)
	setChecked(pitchOnly, advancedSettings.ProsodyPitchOnly)
	setChecked(applyPitch, advancedSettings.ApplyPitch)

	pathRow(parent, instance, "結合モデル", idSettingsJoin, idSettingsJoinBrowse, advancedSettings.JoinModelPath, 140)
	label(parent, instance, "結合スコア倍率", 20, 190, 145, 22)
	control(wsExClientEdge, "EDIT", numberText(advancedSettings.JoinScale), wsChild|wsVisible|wsTabStop, 170, 185, 110, 28, parent, idSettingsJoinScale, instance)

	pathRow(parent, instance, "Worldline DLL", idSettingsWorldline, idSettingsWorldlineBrowse, advancedSettings.WorldlinePath, 230)
	pathRow(parent, instance, "Worldline bridge", idSettingsBridge, idSettingsBridgeBrowse, advancedSettings.WorldlineBridgePath, 270)
	pathRow(parent, instance, "UTAU resampler", idSettingsResampler, idSettingsResamplerBrowse, advancedSettings.UTAUResamplerPath, 310)

	label(parent, instance, "境界補修 ms", 20, 360, 145, 22)
	control(wsExClientEdge, "EDIT", numberText(advancedSettings.BoundaryBridgeMS), wsChild|wsVisible|wsTabStop, 170, 355, 110, 28, parent, idSettingsBoundaryMS, instance)
	label(parent, instance, "適用閾値", 310, 360, 90, 22)
	control(wsExClientEdge, "EDIT", numberText(advancedSettings.BoundaryBridgeThreshold), wsChild|wsVisible|wsTabStop, 405, 355, 110, 28, parent, idSettingsBoundaryThreshold, instance)

	label(parent, instance, "モデルはメイン画面で選択します。OpenJTalk特徴と実行環境パスは自動検出されます。", 20, 405, 725, 42)
	control(0, "BUTTON", "適用して閉じる", wsChild|wsVisible|wsTabStop, 575, 460, 170, 32, parent, idSettingsApply, instance)
	for _, handle := range controls[firstControl:] {
		sendMessage.Call(handle, wmSetFont, font, 1)
	}
	setComboItemHeight(selection, 24)
}

func pathRow(parent, instance uintptr, caption string, editID, browseID int, value string, y int) {
	label(parent, instance, caption, 20, y+5, 145, 22)
	control(wsExClientEdge, "EDIT", value, wsChild|wsVisible|wsTabStop, 170, y, 485, 28, parent, editID, instance)
	control(0, "BUTTON", "参照...", wsChild|wsVisible|wsTabStop, 665, y, 80, 28, parent, browseID, instance)
}

func settingsProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	switch msg {
	case wmCommand:
		switch int(wParam & 0xffff) {
		case idSettingsJoinBrowse:
			browseSettingJSON(hwnd, idSettingsJoin, "結合モデルを選択")
		case idSettingsWorldlineBrowse:
			browseSettingExecutable(hwnd, idSettingsWorldline, "worldline.dllを選択")
		case idSettingsBridgeBrowse:
			browseSettingExecutable(hwnd, idSettingsBridge, "Worldline bridgeを選択")
		case idSettingsResamplerBrowse:
			browseSettingExecutable(hwnd, idSettingsResampler, "resampler.exeを選択")
		case idSettingsApply:
			if err := saveSettingsFromWindow(hwnd); err != nil {
				showError(hwnd, err)
				return 0
			}
			destroyWindow.Call(hwnd)
		}
		return 0
	case wmClose:
		destroyWindow.Call(hwnd)
		return 0
	case wmDestroy:
		settingsWindow = 0
		return 0
	}
	result, _, _ := defWindowProc.Call(hwnd, uintptr(msg), wParam, lParam)
	return result
}

func browseSettingJSON(hwnd uintptr, editID int, title string) bool {
	path, err := openJSONDialog(hwnd, title, strings.TrimSpace(windowText(child(hwnd, editID))))
	if err != nil {
		showError(hwnd, err)
		return false
	}
	if path == "" {
		return false
	}
	setText(child(hwnd, editID), path)
	return true
}

func browseSettingExecutable(hwnd uintptr, editID int, title string) {
	path, err := openExecutableDialog(hwnd, title, strings.TrimSpace(windowText(child(hwnd, editID))))
	if err != nil {
		showError(hwnd, err)
		return
	}
	if path != "" {
		setText(child(hwnd, editID), path)
	}
}

func saveSettingsFromWindow(hwnd uintptr) error {
	var next synthesisSettings
	next.Reading = strings.TrimSpace(windowText(child(hwnd, idSettingsReading)))
	next.Tone = strings.TrimSpace(windowText(child(hwnd, idSettingsTone)))
	if next.Tone == "" {
		next.Tone = "C4"
	}
	var err error
	if next.MoraMS, err = positiveSetting(hwnd, idSettingsMora, "モーラ長"); err != nil {
		return err
	}
	if next.PauseMS, err = nonnegativeSetting(hwnd, idSettingsPause, "休止長"); err != nil {
		return err
	}
	if next.ReleaseMS, err = nonnegativeSetting(hwnd, idSettingsRelease, "末尾長"); err != nil {
		return err
	}
	selection, _, _ := sendMessage.Call(child(hwnd, idSettingsSelection), cbGetCurSel, 0, 0)
	next.Selection = selectionModeAt(int(selection))
	if next.IntonationStrength, err = boundedSetting(hwnd, idSettingsIntonation, "従来抑揚", 0, 1); err != nil {
		return err
	}
	next.ProsodyPitchOnly = checked(child(hwnd, idSettingsPitchOnly))
	next.ApplyPitch = checked(child(hwnd, idSettingsApplyPitch))
	next.JoinModelPath = strings.TrimSpace(windowText(child(hwnd, idSettingsJoin)))
	if next.JoinScale, err = nonnegativeSetting(hwnd, idSettingsJoinScale, "結合スコア倍率"); err != nil {
		return err
	}
	next.WorldlinePath = strings.TrimSpace(windowText(child(hwnd, idSettingsWorldline)))
	next.WorldlineBridgePath = strings.TrimSpace(windowText(child(hwnd, idSettingsBridge)))
	next.UTAUResamplerPath = strings.TrimSpace(windowText(child(hwnd, idSettingsResampler)))
	if next.BoundaryBridgeMS, err = nonnegativeSetting(hwnd, idSettingsBoundaryMS, "境界補修幅"); err != nil {
		return err
	}
	if next.BoundaryBridgeThreshold, err = nonnegativeSetting(hwnd, idSettingsBoundaryThreshold, "境界補修閾値"); err != nil {
		return err
	}
	advancedSettings = next
	return nil
}

func configuredTTSConfig(base tts.Config, selectedModel *prosodyModelOption) (tts.Config, error) {
	s := advancedSettings
	base.Reading = s.Reading
	base.Tone = s.Tone
	base.MoraDurationMS = s.MoraMS
	base.PauseDurationMS = s.PauseMS
	base.ReleaseMS = s.ReleaseMS
	base.IntonationStrength = s.IntonationStrength
	base.ApplyPitch = s.ApplyPitch
	base.SelectionMode = s.Selection
	base.JoinModelPath = s.JoinModelPath
	base.JoinScoreScale = s.JoinScale
	base.WorldlinePath = s.WorldlinePath
	base.WorldlineBridgePath = s.WorldlineBridgePath
	base.UTAUResamplerPath = s.UTAUResamplerPath
	base.BoundaryBridgeMS = s.BoundaryBridgeMS
	base.BoundaryBridgeThreshold = s.BoundaryBridgeThreshold
	if selectedModel == nil {
		return base, nil
	}
	if selectedModel.FrameContour && base.Renderer != "openutau-classic-worldline-faithful" {
		return base, fmt.Errorf("選択した抑揚モデルは音声モードOpenUTAU Classic faithfulで使用してください")
	}
	base.ProsodyModelPath = selectedModel.Path
	base.ProsodyPitchOnly = s.ProsodyPitchOnly
	return base, nil
}

func positiveSetting(hwnd uintptr, id int, name string) (float64, error) {
	value, err := strconv.ParseFloat(strings.TrimSpace(windowText(child(hwnd, id))), 64)
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("%sは0より大きい数値で指定してください", name)
	}
	return value, nil
}

func nonnegativeSetting(hwnd uintptr, id int, name string) (float64, error) {
	value, err := strconv.ParseFloat(strings.TrimSpace(windowText(child(hwnd, id))), 64)
	if err != nil || value < 0 {
		return 0, fmt.Errorf("%sは0以上の数値で指定してください", name)
	}
	return value, nil
}

func boundedSetting(hwnd uintptr, id int, name string, low, high float64) (float64, error) {
	value, err := strconv.ParseFloat(strings.TrimSpace(windowText(child(hwnd, id))), 64)
	if err != nil || value < low || value > high {
		return 0, fmt.Errorf("%sは%gから%gで指定してください", name, low, high)
	}
	return value, nil
}

func numberText(value float64) string { return strconv.FormatFloat(value, 'f', -1, 64) }
func setChecked(hwnd uintptr, value bool) {
	state := uintptr(0)
	if value {
		state = bstChecked
	}
	sendMessage.Call(hwnd, bmSetCheck, state, 0)
}
func checked(hwnd uintptr) bool {
	state, _, _ := sendMessage.Call(hwnd, bmGetCheck, 0, 0)
	return state == bstChecked
}
func selectionIndex(mode voicebank.SelectionMode) int {
	if mode == voicebank.SelectionGreedy {
		return 1
	}
	if mode == voicebank.SelectionTargetOnly {
		return 2
	}
	return 0
}
func selectionModeAt(index int) voicebank.SelectionMode {
	if index == 1 {
		return voicebank.SelectionGreedy
	}
	if index == 2 {
		return voicebank.SelectionTargetOnly
	}
	return voicebank.SelectionViterbi
}
