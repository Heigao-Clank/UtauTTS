//go:build windows

package main

import (
	"fmt"
	"runtime"
	"strings"
	"unsafe"

	"utautts/internal/prosody"
)

// utteranceState is the editable state of one item in the speech list.
// Keeping it separate from HWNDs lets the list, settings panel, and pitch
// graph all bind to the same selected item as the GUI grows.
type utteranceState struct {
	Text                   string
	Reading                string
	VoicebankPath          string
	Renderer               string
	RendererExecutable     string
	RendererExecutableKind string
	ProsodyModel           string
	ManualPitch            *prosody.ManualPitchFile
	Synthesis              synthesisSettings
	ProsodyPitchOnly       bool
}

func newUtteranceState(text string) utteranceState {
	return utteranceState{
		Text:      text,
		Renderer:  defaultRendererBackend,
		Synthesis: defaultSynthesisSettings(),
	}
}

func (u *utteranceState) setText(text string) {
	if u == nil {
		return
	}
	u.Text = strings.TrimSpace(text)
	// A manual curve is tied to a reading. It must not silently carry over
	// when the text is replaced and the old reading is no longer applicable.
	if u.ManualPitch != nil && u.Reading != "" {
		u.ManualPitch = nil
	}
	u.Reading = ""
}

type editorState struct {
	Utterances []utteranceState
	Selected   int
}

var syncingUtteranceControls bool

func newEditorState(initialText string) *editorState {
	return &editorState{
		Utterances: []utteranceState{newUtteranceState(initialText)},
		Selected:   0,
	}
}

func (e *editorState) selected() *utteranceState {
	if e == nil || e.Selected < 0 || e.Selected >= len(e.Utterances) {
		return nil
	}
	return &e.Utterances[e.Selected]
}

func (e *editorState) selectIndex(index int) bool {
	if e == nil || index < 0 || index >= len(e.Utterances) {
		return false
	}
	e.Selected = index
	return true
}

func (e *editorState) add(text string) int {
	if e == nil {
		return -1
	}
	e.Utterances = append(e.Utterances, newUtteranceState(text))
	e.Selected = len(e.Utterances) - 1
	return e.Selected
}

func (e *editorState) remove(index int) bool {
	if e == nil || index < 0 || index >= len(e.Utterances) || len(e.Utterances) <= 1 {
		return false
	}
	e.Utterances = append(e.Utterances[:index], e.Utterances[index+1:]...)
	if e.Selected >= len(e.Utterances) {
		e.Selected = len(e.Utterances) - 1
	} else if e.Selected > index {
		e.Selected--
	}
	return true
}

func syncSelectedUtteranceText(hwnd uintptr) {
	if syncingUtteranceControls || editor == nil {
		return
	}
	selected := editor.selected()
	if selected == nil {
		return
	}
	selected.setText(windowText(child(hwnd, idText)))
}

func selectedUtteranceText() string {
	if editor != nil {
		if selected := editor.selected(); selected != nil {
			return strings.TrimSpace(selected.Text)
		}
	}
	return ""
}

func loadSelectedUtteranceText(hwnd uintptr) {
	if editor == nil {
		return
	}
	selected := editor.selected()
	if selected == nil {
		return
	}
	syncingUtteranceControls = true
	setText(child(hwnd, idText), selected.Text)
	syncingUtteranceControls = false
}

func syncSelectedUtteranceControls(hwnd uintptr) {
	if editor == nil {
		return
	}
	selected := editor.selected()
	if selected == nil {
		return
	}
	voicebankIndex, _, _ := sendMessage.Call(child(hwnd, idVoicebank), cbGetCurSel, 0, 0)
	if int(voicebankIndex) >= 0 && int(voicebankIndex) < len(availableBanks) {
		selected.VoicebankPath = availableBanks[int(voicebankIndex)].Path
	}
	rendererIndex, _, _ := sendMessage.Call(child(hwnd, idRenderer), cbGetCurSel, 0, 0)
	selected.Renderer = rendererBackend(int(rendererIndex))
	selected.RendererExecutable = rendererOptionAt(int(rendererIndex)).executable
	selected.RendererExecutableKind = rendererOptionAt(int(rendererIndex)).executableKind
	modelIndex, _, _ := sendMessage.Call(child(hwnd, idProsodyModel), cbGetCurSel, 0, 0)
	if model := prosodyModelAt(int(modelIndex)); model != nil {
		selected.ProsodyModel = model.Path
	} else {
		selected.ProsodyModel = ""
	}
}

func loadSelectedUtteranceControls(hwnd uintptr) {
	if editor == nil {
		return
	}
	selected := editor.selected()
	if selected == nil {
		return
	}
	syncingUtteranceControls = true
	for index, bank := range availableBanks {
		if samePath(bank.Path, selected.VoicebankPath) {
			sendMessage.Call(child(hwnd, idVoicebank), cbSetCurSel, uintptr(index), 0)
			break
		}
	}
	for index, option := range rendererOptions {
		if option.backend == selected.Renderer {
			sendMessage.Call(child(hwnd, idRenderer), cbSetCurSel, uintptr(index), 0)
			break
		}
	}
	modelIndex := 0
	for index, model := range availableProsodyModels {
		if model.Path == selected.ProsodyModel {
			modelIndex = index + 1
			break
		}
	}
	sendMessage.Call(child(hwnd, idProsodyModel), cbSetCurSel, uintptr(modelIndex), 0)
	syncingUtteranceControls = false
	selectedBank, _, _ := sendMessage.Call(child(hwnd, idVoicebank), cbGetCurSel, 0, 0)
	updateVoicebankPortrait(int(selectedBank))
	updateVoicebankSummary(int(selectedBank))
	updateRendererInfo(hwnd)
}

func loadSelectedDetailedSettings() {
	if editor == nil {
		return
	}
	if selected := editor.selected(); selected != nil && selected.Synthesis.MoraMS > 0 {
		advancedSettings = selected.Synthesis
	}
}

func storeSelectedDetailedSettings(settings synthesisSettings) {
	if editor == nil {
		return
	}
	if selected := editor.selected(); selected != nil {
		selected.Synthesis = settings
	}
}

func utteranceListLabel(index int, utterance utteranceState) string {
	text := strings.Join(strings.Fields(utterance.Text), " ")
	if text == "" {
		text = "（空の発話）"
	}
	if len([]rune(text)) > 36 {
		text = string([]rune(text)[:36]) + "…"
	}
	return fmt.Sprintf("%d. %s", index+1, text)
}

func refreshUtteranceList(hwnd, list uintptr) {
	if list == 0 || editor == nil {
		return
	}
	sendMessage.Call(list, lbResetContent, 0, 0)
	for index, utterance := range editor.Utterances {
		value := windowsString(utteranceListLabel(index, utterance))
		sendMessage.Call(list, lbAddString, 0, uintptr(unsafe.Pointer(&value[0])))
		runtime.KeepAlive(value)
	}
	if editor.Selected >= 0 && editor.Selected < len(editor.Utterances) {
		sendMessage.Call(list, lbSetCurSel, uintptr(editor.Selected), 0)
	}
}

func refreshSelectedUtteranceListItem(hwnd uintptr) {
	if editor == nil {
		return
	}
	list := child(hwnd, idUtteranceList)
	if list == 0 || editor.Selected < 0 || editor.Selected >= len(editor.Utterances) {
		return
	}
	refreshUtteranceList(hwnd, list)
}

func addUtterance(hwnd uintptr) {
	if editor == nil {
		return
	}
	syncSelectedUtteranceText(hwnd)
	syncSelectedUtteranceControls(hwnd)
	var inherited utteranceState
	if selected := editor.selected(); selected != nil {
		inherited = *selected
		inherited.Text = ""
		inherited.Reading = ""
		inherited.ManualPitch = nil
	}
	index := editor.add("")
	if index >= 0 && inherited.Renderer != "" {
		editor.Utterances[index] = inherited
	}
	activeManualPitch = nil
	refreshUtteranceList(hwnd, child(hwnd, idUtteranceList))
	loadSelectedUtteranceText(hwnd)
	if index >= 0 {
		setFocus.Call(child(hwnd, idText))
	}
}

func deleteSelectedUtterance(hwnd uintptr) {
	if editor == nil {
		return
	}
	syncSelectedUtteranceText(hwnd)
	if !editor.remove(editor.Selected) {
		return
	}
	if selected := editor.selected(); selected != nil {
		activeManualPitch = selected.ManualPitch
	}
	refreshUtteranceList(hwnd, child(hwnd, idUtteranceList))
	loadSelectedUtteranceText(hwnd)
}
