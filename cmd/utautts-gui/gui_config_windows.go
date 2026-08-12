//go:build windows

package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"utautts/internal/prosody"
)

const guiConfigVersion = 1

type guiConfig struct {
	Version   int                  `json:"version"`
	Renderers []rendererDefinition `json:"renderers,omitempty"`
	Models    []modelDefinition    `json:"models,omitempty"`
}

type rendererDefinition struct {
	ID             string `json:"id"`
	Label          string `json:"label"`
	Backend        string `json:"backend"`
	Description    string `json:"description,omitempty"`
	Executable     string `json:"executable,omitempty"`
	ExecutableKind string `json:"executable_kind,omitempty"`
	Enabled        *bool  `json:"enabled,omitempty"`
}

type modelDefinition struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	Path    string `json:"path"`
	Enabled *bool  `json:"enabled,omitempty"`
}

var (
	guiConfigDirectory string
	configuredModels   []modelDefinition
)

func loadGUIConfiguration() {
	paths := []string{}
	if executable, err := os.Executable(); err == nil {
		paths = append(paths, filepath.Join(filepath.Dir(executable), "gui-config.json"))
	}
	if current, err := os.Getwd(); err == nil {
		paths = append(paths, filepath.Join(current, "gui-config.json"))
	}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var config guiConfig
		if err := json.Unmarshal(data, &config); err != nil {
			log.Printf("GUI設定を読み込めません: path=%q error=%v", path, err)
			return
		}
		if config.Version != guiConfigVersion {
			log.Printf("未対応のGUI設定バージョンを無視します: path=%q version=%d", path, config.Version)
			return
		}
		guiConfigDirectory = filepath.Dir(path)
		applyRendererDefinitions(config.Renderers)
		configuredModels = config.Models
		return
	}
}

func applyRendererDefinitions(definitions []rendererDefinition) {
	for _, definition := range definitions {
		if definition.Label == "" || definition.Backend == "" || (definition.Enabled != nil && !*definition.Enabled) {
			continue
		}
		option := rendererOption{
			ID: definition.ID, label: definition.Label, backend: definition.Backend,
			description: definition.Description, executable: resolveGUIPath(definition.Executable),
			executableKind: definition.ExecutableKind,
		}
		if option.description == "" {
			option.description = "外部設定で追加されたRendererです。"
		}
		found := false
		for index := range rendererOptions {
			if rendererOptions[index].backend == option.backend || (option.ID != "" && rendererOptions[index].ID == option.ID) {
				rendererOptions[index] = option
				found = true
				break
			}
		}
		if !found {
			rendererOptions = append(rendererOptions, option)
		}
	}
}

func resolveGUIPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" || filepath.IsAbs(path) || guiConfigDirectory == "" {
		return path
	}
	return filepath.Join(guiConfigDirectory, path)
}

func configuredModelOptions() []prosodyModelOption {
	var result []prosodyModelOption
	for _, definition := range configuredModels {
		if definition.Path == "" || (definition.Enabled != nil && !*definition.Enabled) {
			continue
		}
		path := resolveGUIPath(definition.Path)
		model, err := prosody.LoadModel(path)
		if err != nil {
			log.Printf("設定モデルを読み込めません: path=%q error=%v", path, err)
			continue
		}
		label := definition.Label
		if label == "" {
			label = prosodyModelLabel(model, filepath.Base(path))
		}
		result = append(result, prosodyModelOption{Path: path, Label: label, Version: model.Version, Mode: model.Mode, RequiresFeatures: model.RequiresExternalFeatures(), FrameContour: model.HasFrameContour()})
	}
	return result
}

func guiConfigTemplate() []byte {
	config := guiConfig{Version: guiConfigVersion, Renderers: []rendererDefinition{{ID: "openutau-classic-faithful", Label: "OpenUTAU Classic faithful", Backend: "openutau-classic-worldline-faithful", Description: "OpenUTAU互換の標準Renderer", ExecutableKind: "bridge", Executable: "tools/worldline-bridge/bin/Release/net8.0/WorldlineBridge.dll"}}, Models: []modelDefinition{{ID: "v8", Label: "v8 学習イントネーション", Path: "models/frame-intonation-v8.json"}}}
	data, _ := json.MarshalIndent(config, "", "  ")
	return append(data, '\n')
}

func validateGUIConfig(config guiConfig) error {
	if config.Version != guiConfigVersion {
		return fmt.Errorf("unsupported GUI config version %d", config.Version)
	}
	for _, renderer := range config.Renderers {
		if renderer.Label == "" || renderer.Backend == "" {
			return fmt.Errorf("renderer requires label and backend")
		}
	}
	return nil
}

func sortRendererOptions() {
	// Keep the built-in faithful/default renderer first; external entries follow.
	faithful := defaultRendererIndex()
	sort.SliceStable(rendererOptions, func(i, j int) bool {
		if i == faithful {
			return true
		}
		if j == faithful {
			return false
		}
		return rendererOptions[i].label < rendererOptions[j].label
	})
}
