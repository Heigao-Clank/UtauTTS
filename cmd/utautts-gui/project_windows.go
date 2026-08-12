//go:build windows

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

const projectVersion = 1

type editorProject struct {
	Version    int               `json:"version"`
	Utterances []utteranceState  `json:"utterances"`
	Selected   int               `json:"selected"`
	Settings   synthesisSettings `json:"settings"`
	OutputPath string            `json:"output_path,omitempty"`
}

var projectPath string

func saveProject(hwnd uintptr) {
	if editor == nil {
		return
	}
	syncSelectedUtteranceText(hwnd)
	syncSelectedUtteranceControls(hwnd)
	storeSelectedDetailedSettings(advancedSettings)
	path, err := saveJSONDialog(hwnd, "プロジェクトを保存", projectPath)
	if err != nil {
		showError(hwnd, err)
		return
	}
	if path == "" {
		return
	}
	project := editorProject{
		Version: projectVersion, Utterances: editor.Utterances, Selected: editor.Selected,
		Settings: advancedSettings, OutputPath: stringsTrim(windowText(child(hwnd, idOutput))),
	}
	data, err := json.MarshalIndent(project, "", "  ")
	if err == nil {
		err = os.WriteFile(path, data, 0o644)
	}
	if err != nil {
		showError(hwnd, fmt.Errorf("プロジェクトを保存できません: %w", err))
		return
	}
	projectPath = path
	setText(statusLabel, "プロジェクトを保存しました")
}

func openProject(hwnd uintptr) {
	path, err := openJSONDialog(hwnd, "プロジェクトを開く", projectPath)
	if err != nil {
		showError(hwnd, err)
		return
	}
	if path == "" {
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		showError(hwnd, fmt.Errorf("プロジェクトを読み込めません: %w", err))
		return
	}
	var project editorProject
	if err := json.Unmarshal(data, &project); err != nil {
		showError(hwnd, fmt.Errorf("プロジェクトJSONが不正です: %w", err))
		return
	}
	if project.Version != projectVersion {
		showError(hwnd, fmt.Errorf("未対応のプロジェクトバージョンです: %d", project.Version))
		return
	}
	if len(project.Utterances) == 0 {
		showError(hwnd, fmt.Errorf("プロジェクトに発話がありません"))
		return
	}
	if project.Selected < 0 || project.Selected >= len(project.Utterances) {
		project.Selected = 0
	}
	editor = &editorState{Utterances: project.Utterances, Selected: project.Selected}
	advancedSettings = project.Settings
	if advancedSettings.MoraMS <= 0 {
		advancedSettings = defaultSynthesisSettings()
	}
	projectPath = path
	setText(child(hwnd, idOutput), project.OutputPath)
	refreshUtteranceList(hwnd, child(hwnd, idUtteranceList))
	loadSelectedUtteranceText(hwnd)
	loadSelectedUtteranceControls(hwnd)
	if selected := editor.selected(); selected != nil {
		activeManualPitch = selected.ManualPitch
	}
	setText(statusLabel, "プロジェクトを開きました")
}

func stringsTrim(value string) string {
	return strings.TrimSpace(value)
}
