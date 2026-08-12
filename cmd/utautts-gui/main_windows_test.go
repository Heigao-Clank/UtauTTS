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
	tests := []struct {
		index int
		want  string
	}{
		{index: 0, want: "openutau-classic-worldline-faithful"},
		{index: 1, want: "waveform"},
		{index: 2, want: "waveform-long"},
		{index: 3, want: "worldline-hybrid"},
		{index: 4, want: "worldline-hybrid-cv-gentle"},
		{index: 5, want: "worldline-hybrid-cv-balanced"},
		{index: -1, want: "openutau-classic-worldline-faithful"},
		{index: 99, want: "openutau-classic-worldline-faithful"},
	}
	for _, test := range tests {
		if got := rendererBackend(test.index); got != test.want {
			t.Errorf("rendererBackend(%d) = %q, want %q", test.index, got, test.want)
		}
	}
}

func TestGUIDefaultsUseFaithfulRendererAndV8(t *testing.T) {
	if got := defaultRendererIndex(); got != 0 {
		t.Fatalf("default renderer index = %d, want faithful index 0", got)
	}
	previous := availableProsodyModels
	t.Cleanup(func() { availableProsodyModels = previous })
	availableProsodyModels = []prosodyModelOption{
		{Version: 9, Mode: "intonation_phrase_anchor_v9_1"},
		{Version: prosody.FramePitchModelVersion, Mode: "intonation_frame_tcn_accent_bounded"},
	}
	if got := defaultProsodyModelIndex(); got != 2 {
		t.Fatalf("default prosody combo index = %d, want v8 index 2", got)
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

func TestProsodyModelLabelKeepsVersionVisible(t *testing.T) {
	v8 := &prosody.Model{Version: prosody.FramePitchModelVersion, Mode: "intonation_frame_tcn_accent_bounded"}
	if got := prosodyModelLabel(v8, "model.json"); !strings.Contains(got, "v8") {
		t.Fatalf("v8 label = %q", got)
	}
	future := &prosody.Model{Version: 10, Mode: "future_mode"}
	if got := prosodyModelLabel(future, "future.json"); !strings.Contains(got, "v10") || !strings.Contains(got, "future") {
		t.Fatalf("future label = %q", got)
	}
	v9 := &prosody.Model{Version: prosody.StandardAccentModelVersion, Mode: "intonation_phrase_anchor_v9"}
	if got := prosodyModelLabel(v9, "phrase-anchor-v9.json"); !strings.Contains(got, "v9") {
		t.Fatalf("v9 label = %q", got)
	}
	v91 := &prosody.Model{Version: prosody.StandardAccentModelVersion, Mode: "intonation_phrase_anchor_v9_1"}
	if got := prosodyModelLabel(v91, "phrase-anchor-v9-1.json"); !strings.Contains(got, "v9.1") {
		t.Fatalf("v9.1 label = %q", got)
	}
}
