//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"utautts/internal/prosody"
)

type prosodyModelOption struct {
	Path             string
	Label            string
	Version          int
	Mode             string
	RequiresFeatures bool
	FrameContour     bool
}

func discoverProsodyModels() []prosodyModelOption {
	var directories []string
	if executable, err := os.Executable(); err == nil {
		directories = append(directories, filepath.Join(filepath.Dir(executable), "models"))
	}
	if current, err := os.Getwd(); err == nil {
		directories = append(directories, filepath.Join(current, "models"), filepath.Join(current, "out", "prosody"))
	}
	seen := map[string]bool{}
	result := configuredModelOptions()
	var resultPaths = map[string]bool{}
	for _, option := range result {
		resultPaths[strings.ToLower(option.Path)] = true
	}
	for _, directory := range directories {
		entries, err := os.ReadDir(directory)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".json") {
				continue
			}
			path, err := filepath.Abs(filepath.Join(directory, entry.Name()))
			if err != nil || seen[strings.ToLower(path)] || resultPaths[strings.ToLower(path)] {
				continue
			}
			model, err := prosody.LoadModel(path)
			if err != nil {
				continue
			}
			seen[strings.ToLower(path)] = true
			result = append(result, prosodyModelOption{
				Path: path, Label: prosodyModelLabel(model, entry.Name()), Version: model.Version, Mode: model.Mode,
				RequiresFeatures: model.RequiresExternalFeatures(), FrameContour: model.HasFrameContour(),
			})
		}
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].Version != result[j].Version {
			return result[i].Version > result[j].Version
		}
		return result[i].Label < result[j].Label
	})
	return result
}

func prosodyModelLabel(model *prosody.Model, filename string) string {
	if model.Version == prosody.FramePitchModelVersion && model.Mode == "intonation_frame_tcn_accent_bounded" {
		return "v8 学習イントネーション"
	}
	if model.Version == prosody.StandardAccentModelVersion &&
		(model.Mode == "intonation_phrase_anchor_v9" || model.Mode == "intonation_phrase_anchor_v9_1") {
		if model.Mode == "intonation_phrase_anchor_v9_1" {
			return "v9.1 phrase-anchor intonation"
		}
		return "v9 phrase-anchor intonation"
	}
	if model.Version == prosody.StandardAccentModelVersion && model.Mode == "standard_japanese_accent" {
		return "v9 標準語アクセント"
	}
	name := strings.TrimSuffix(filename, filepath.Ext(filename))
	return fmt.Sprintf("v%d %s (%s)", model.Version, name, model.Mode)
}

func prosodyModelAt(index int) *prosodyModelOption {
	index--
	if index < 0 || index >= len(availableProsodyModels) {
		return nil
	}
	return &availableProsodyModels[index]
}
