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
		if definition.ID == "" {
			continue
		}
		for index := range rendererOptions {
			if rendererOptions[index].ID != definition.ID {
				continue
			}
			if definition.Enabled != nil && !*definition.Enabled {
				rendererOptions = append(rendererOptions[:index], rendererOptions[index+1:]...)
			} else {
				rendererOptions[index].executable = resolveGUIPath(definition.Executable)
				rendererOptions[index].executableKind = definition.ExecutableKind
			}
			break
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
		if strings.TrimSpace(model.ID) == "" || strings.TrimSpace(model.DisplayName) == "" {
			log.Printf("設定モデルにid/display_nameがありません: path=%q", path)
			continue
		}
		result = append(result, prosodyModelOption{Path: path, Label: model.DisplayName, Version: model.Version, Mode: model.Mode, RequiresFeatures: model.RequiresExternalFeatures(), FrameContour: model.HasFrameContour(), RecommendedRenderers: append([]string(nil), model.RecommendedRenderers...)})
	}
	return result
}

func guiConfigTemplate() []byte {
	config := guiConfig{Version: guiConfigVersion}
	data, _ := json.MarshalIndent(config, "", "  ")
	return append(data, '\n')
}

func validateGUIConfig(config guiConfig) error {
	if config.Version != guiConfigVersion {
		return fmt.Errorf("unsupported GUI config version %d", config.Version)
	}
	for _, renderer := range config.Renderers {
		if renderer.ID == "" {
			return fmt.Errorf("renderer override requires plugin id")
		}
	}
	return nil
}

func sortRendererOptions() {
	sort.SliceStable(rendererOptions, func(i, j int) bool {
		return rendererOptions[i].label < rendererOptions[j].label
	})
}
