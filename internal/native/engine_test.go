package native

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"utautts/internal/audio"
)

const openJTalkHelperEnvironment = "UTAUTTS_TEST_OPENJTALK_HELPER"

func TestMain(m *testing.M) {
	if os.Getenv(openJTalkHelperEnvironment) == "1" {
		_, _ = io.Copy(io.Discard, os.Stdin)
		_, _ = fmt.Fprint(os.Stdout, `{"version":1,"reading":"ハロー","morae":["は","ろ","ー"],"features":[{},{},{}]}`)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestEngineListsAnalyzesAndSynthesizes(t *testing.T) {
	root := t.TempDir()
	bankDir := filepath.Join(root, "bank")
	if err := os.Mkdir(bankDir, 0755); err != nil {
		t.Fatal(err)
	}
	samples := make([]int16, 400)
	for index := range samples {
		samples[index] = 4000
	}
	if err := audio.WriteWav(filepath.Join(bankDir, "a.wav"), &audio.PCM{SampleRate: 1000, Channels: 1, Data: samples}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bankDir, "oto.ini"), []byte("a.wav=あ,0,0,0,0,0\n"), 0644); err != nil {
		t.Fatal(err)
	}
	engine, err := New(Config{VoiceDir: root})
	if err != nil {
		t.Fatal(err)
	}
	for _, method := range []string{"health", "voicebanks", "renderers", "models"} {
		if _, err := engine.Call(method, nil); err != nil {
			t.Fatalf("%s: %v", method, err)
		}
	}
	analysis, err := engine.Call("analyze", []byte(`{"text":"あ"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(analysis) {
		t.Fatalf("analysis=%s", analysis)
	}
	preview, err := engine.Call("predictProsody", []byte(`{"kana":"\u3042","mora_duration_ms":100,"apply_pitch":false}`))
	if err != nil {
		t.Fatal(err)
	}
	var previewResult struct {
		MoraDurationsMS []float64 `json:"mora_durations_ms"`
	}
	if err := json.Unmarshal(preview, &previewResult); err != nil {
		t.Fatalf("prosody preview=%s: %v", preview, err)
	}
	if len(previewResult.MoraDurationsMS) != 1 || previewResult.MoraDurationsMS[0] != 100 {
		t.Fatalf("prosody preview=%s", preview)
	}
	dictionaryAnalysis, err := engine.Call("analyze", []byte(`{"text":"UtauTTS","dictionary":[{"surface":"UtauTTS","reading":"あ"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	var dictionaryResult struct {
		Reading string `json:"reading"`
	}
	if err := json.Unmarshal(dictionaryAnalysis, &dictionaryResult); err != nil {
		t.Fatal(err)
	}
	if dictionaryResult.Reading != "ア" {
		t.Fatalf("dictionary analysis=%s", dictionaryAnalysis)
	}
	output := filepath.Join(root, "preview.wav")
	request, _ := json.Marshal(map[string]any{"kana": "あ", "voicebank_id": "bank", "mora_duration_ms": 100, "output_path": output})
	if _, err := engine.Call("synthesize", request); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(output); err != nil || info.Size() < 44 {
		t.Fatalf("output info=%v err=%v", info, err)
	}
	if err := os.WriteFile(filepath.Join(bankDir, "oto.ini"), []byte("a.wav=い,0,0,0,0,0\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Call("reloadVoicebanks", nil); err != nil {
		t.Fatal(err)
	}
	request, _ = json.Marshal(map[string]any{"kana": "い", "voicebank_id": "bank", "mora_duration_ms": 100, "output_path": filepath.Join(root, "reloaded.wav")})
	if _, err := engine.Call("synthesize", request); err != nil {
		t.Fatalf("synthesis after voicebank reload: %v", err)
	}
}

func TestEngineFallsBackToOpenJTalkForEnglish(t *testing.T) {
	root := t.TempDir()
	bankDir := filepath.Join(root, "bank")
	if err := os.Mkdir(bankDir, 0755); err != nil {
		t.Fatal(err)
	}
	samples := make([]int16, 400)
	for index := range samples {
		samples[index] = 4000
	}
	if err := audio.WriteWav(filepath.Join(bankDir, "a.wav"), &audio.PCM{SampleRate: 1000, Channels: 1, Data: samples}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bankDir, "oto.ini"), []byte("a.wav=は,0,0,0,0,0\na.wav=ろ,0,0,0,0,0\na.wav=ー,0,0,0,0,0\n"), 0644); err != nil {
		t.Fatal(err)
	}
	helper, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(openJTalkHelperEnvironment, "1")
	engine, err := New(Config{VoiceDir: root, OpenJTalkPath: helper, OpenJTalkDictionary: root})
	if err != nil {
		t.Fatal(err)
	}

	analysisJSON, err := engine.Call("analyze", []byte(`{"text":"hello"}`))
	if err != nil {
		t.Fatal(err)
	}
	var analysis struct {
		Reading string `json:"reading"`
		Morae   []any  `json:"morae"`
	}
	if err := json.Unmarshal(analysisJSON, &analysis); err != nil {
		t.Fatal(err)
	}
	if analysis.Reading != "ハロー" || len(analysis.Morae) != 3 {
		t.Fatalf("analysis=%s", analysisJSON)
	}

	output := filepath.Join(root, "english.wav")
	request, _ := json.Marshal(map[string]any{
		"text": "hello", "voicebank_id": "bank", "mora_duration_ms": 100, "output_path": output,
	})
	if _, err := engine.Call("synthesize", request); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(output); err != nil || info.Size() < 44 {
		t.Fatalf("output info=%v err=%v", info, err)
	}
}

func TestNewReportsInvalidPlugin(t *testing.T) {
	pluginDirectory := t.TempDir()
	if err := os.WriteFile(filepath.Join(pluginDirectory, "plugin.json"), []byte(`{"kind":"renderer"}`), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := New(Config{VoiceDir: t.TempDir(), RendererDirectories: []string{pluginDirectory}}); err == nil {
		t.Fatal("invalid renderer plugin was silently ignored")
	}
}

func TestNewAllowsMissingVoiceDirectory(t *testing.T) {
	engine, err := New(Config{VoiceDir: filepath.Join(t.TempDir(), "missing")})
	if err != nil {
		t.Fatal(err)
	}
	result, err := engine.Call("voicebanks", nil)
	if err != nil || string(result) != `{"voicebanks":[]}` {
		t.Fatalf("voicebanks=%s err=%v", result, err)
	}
}
