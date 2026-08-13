//go:build windows

package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"utautts/internal/prosody"
	"utautts/internal/tts"
	"utautts/internal/voicebank"
)

func TestRendererBackend(t *testing.T) {
	if len(rendererOptions) == 0 {
		t.Fatal("renderer plugins were not discovered")
	}
	if got := rendererBackend(0); got != "waveform" {
		t.Fatalf("manifest-selected default backend = %q", got)
	}
	for _, index := range []int{-1, len(rendererOptions) + 1} {
		if got := rendererBackend(index); got != rendererOptions[0].backend {
			t.Errorf("rendererBackend(%d) = %q, want manifest default %q", index, got, rendererOptions[0].backend)
		}
	}
}

func TestGUIDefaultsComeFromPluginCatalogAndNoModel(t *testing.T) {
	if got := defaultRendererIndex(); got != 0 {
		t.Fatalf("default renderer index = %d, want catalog index 0", got)
	}
	previous := availableProsodyModels
	t.Cleanup(func() { availableProsodyModels = previous })
	availableProsodyModels = []prosodyModelOption{
		{Version: 9, Mode: "intonation_phrase_anchor_v9_1"},
		{Version: prosody.FramePitchModelVersion, Mode: "intonation_frame_tcn_accent_bounded"},
	}
	if got := defaultProsodyModelIndex(); got != 0 {
		t.Fatalf("default prosody combo index = %d, want none", got)
	}
}

func TestPresentationTextCombinesCharacterAndReadme(t *testing.T) {
	got := presentationText(voicebank.Presentation{
		Summary:       voicebank.Summary{Name: "音源", Path: `C:\voice\bank`, ReadmePath: `C:\voice\bank\readme.txt`},
		CharacterText: "name=音源\nimage=portrait.bmp",
		ReadmeText:    "音源の説明です。",
	}, nil)
	for _, want := range []string{`C:\voice\bank`, "【character.txt】", "image=portrait.bmp", "【readme.txt】", "音源の説明です。"} {
		if !strings.Contains(got, want) {
			t.Fatalf("presentation text does not contain %q: %q", want, got)
		}
	}
	if strings.Contains(strings.ReplaceAll(got, "\r\n", ""), "\n") {
		t.Fatalf("presentation text contains bare LF: %q", got)
	}
}

func TestPresentationTextShowsReadError(t *testing.T) {
	got := presentationText(voicebank.Presentation{Summary: voicebank.Summary{Path: `C:\voice\bank`}}, errors.New("decode failed"))
	if !strings.Contains(got, "読込エラー") || !strings.Contains(got, "decode failed") {
		t.Fatalf("presentation error is hidden: %q", got)
	}
}

func TestRendererOptionsExplainEveryMode(t *testing.T) {
	for index, option := range rendererOptions {
		if option.label == "" || option.backend == "" || option.description == "" {
			t.Fatalf("renderer option %d is incomplete: %#v", index, option)
		}
	}
}

func TestGUIConfigValidationAndRendererDefinition(t *testing.T) {
	if err := validateGUIConfig(guiConfig{Version: guiConfigVersion, Renderers: []rendererDefinition{{ID: "waveform"}}}); err != nil {
		t.Fatal(err)
	}
	if err := validateGUIConfig(guiConfig{Version: guiConfigVersion, Renderers: []rendererDefinition{{ID: ""}}}); err == nil {
		t.Fatal("invalid renderer definition was accepted")
	}
	if err := validateGUIConfig(guiConfig{Version: guiConfigVersion + 1}); err == nil {
		t.Fatal("unsupported config version was accepted")
	}
}

func TestOutputFilenameSanitizesWindowsCharacters(t *testing.T) {
	previous := availableBanks
	t.Cleanup(func() { availableBanks = previous })
	availableBanks = []voicebank.Summary{{Name: "音源:テスト", Path: `C:\voice\bank`}}
	name := outputFilename(utteranceState{VoicebankPath: `C:\voice\bank`, Text: `a/b:c?d`})
	if name != "音源_テスト_a_b_c_d.wav" {
		t.Fatalf("output filename = %q", name)
	}
}

func TestSanitizeDisplayName(t *testing.T) {
	if got := sanitizeDisplayName("  音源\x00名\r\n  "); got != "音源 名" {
		t.Fatalf("sanitizeDisplayName() = %q", got)
	}
	long := strings.Repeat("声", 200)
	if got := []rune(sanitizeDisplayName(long)); len(got) != 121 || got[120] != '…' {
		t.Fatalf("long display name has %d runes", len(got))
	}
}

func TestDiscoverVoicebanksKeepsExplicitInitialBank(t *testing.T) {
	bank := filepath.Join(t.TempDir(), "bank")
	if err := os.Mkdir(bank, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bank, "oto.ini"), []byte("a.wav=あ,0,0,0,0,0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	banks, err := discoverVoicebanks(filepath.Join(t.TempDir(), "missing"), bank)
	if err == nil {
		t.Fatal("missing scan root did not report an error")
	}
	if len(banks) != 1 || !samePath(banks[0].Path, bank) {
		t.Fatalf("voicebanks = %#v", banks)
	}
}

func TestConfiguredTTSConfigAppliesAdvancedSettingsWithoutProsody(t *testing.T) {
	previous := advancedSettings
	t.Cleanup(func() { advancedSettings = previous })
	advancedSettings = synthesisSettings{
		Tone: "D4", MoraMS: 125, PauseMS: 90, ReleaseMS: 15,
		Selection: voicebank.SelectionGreedy, BoundaryBridgeMS: 12, BoundaryBridgeThreshold: 0.4,
	}
	config, err := configuredTTSConfig(tts.Config{Text: "test", Renderer: "waveform"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if config.Tone != "D4" || config.MoraDurationMS != 125 || config.SelectionMode != voicebank.SelectionGreedy || config.ProsodyModelPath != "" {
		t.Fatalf("advanced config was not applied: %#v", config)
	}
}

func TestConfiguredTTSConfigSelectsV8ForRuntimeFeatures(t *testing.T) {
	previous := advancedSettings
	t.Cleanup(func() { advancedSettings = previous })
	advancedSettings = synthesisSettings{
		Tone: "C4", MoraMS: 140, PauseMS: 180, ReleaseMS: 20,
		Selection: voicebank.SelectionViterbi, ProsodyPitchOnly: true,
	}
	model := &prosodyModelOption{Path: "v8.json", RequiresFeatures: true, FrameContour: true}
	config, err := configuredTTSConfig(tts.Config{Text: "別の文", Renderer: "openutau-classic-worldline-faithful"}, model)
	if err != nil {
		t.Fatal(err)
	}
	if config.ProsodyModelPath != "v8.json" || len(config.ProsodyFeatures) != 0 || !config.ProsodyPitchOnly {
		t.Fatalf("v8 config was not applied: %#v", config)
	}
}

func TestConfiguredTTSConfigRejectsUnsupportedV8Renderer(t *testing.T) {
	previous := advancedSettings
	t.Cleanup(func() { advancedSettings = previous })
	advancedSettings = synthesisSettings{}
	model := &prosodyModelOption{Path: "v8.json", FrameContour: true}
	if _, err := configuredTTSConfig(tts.Config{Renderer: "waveform"}, model); err == nil {
		t.Fatal("v8 accepted a renderer that cannot consume its frame contour")
	}
}

func TestProsodyModelAtReservesZeroForNone(t *testing.T) {
	previous := availableProsodyModels
	t.Cleanup(func() { availableProsodyModels = previous })
	availableProsodyModels = []prosodyModelOption{{Path: "v8.json", Label: "v8"}}
	if prosodyModelAt(0) != nil || prosodyModelAt(-1) != nil || prosodyModelAt(2) != nil {
		t.Fatal("none or out-of-range model index resolved to a model")
	}
	if model := prosodyModelAt(1); model == nil || model.Path != "v8.json" {
		t.Fatalf("model index 1 = %#v", model)
	}
}

func TestProsodyModelLabelUsesSelfDescription(t *testing.T) {
	model := &prosody.Model{ID: "stable-id", DisplayName: "表示名", Version: 99, Mode: "future"}
	if got := prosodyModelLabel(model, "ignored.json"); got != "表示名" {
		t.Fatalf("self-described label = %q", got)
	}
	if got := prosodyModelLabel(&prosody.Model{ID: "stable-id"}, "ignored.json"); got != "stable-id" {
		t.Fatalf("ID fallback label = %q", got)
	}
	if got := prosodyModelLabel(&prosody.Model{}, "file-name.json"); got != "" {
		t.Fatalf("identity-free model got a filename label = %q", got)
	}
}
