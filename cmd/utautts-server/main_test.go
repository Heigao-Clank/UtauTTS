package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"utautts/internal/audio"
)

func TestSynthesizeEndpointReportsWaveformRenderer(t *testing.T) {
	root := t.TempDir()
	wavPath := filepath.Join(root, "a.wav")
	samples := make([]int16, 400)
	for i := range samples {
		samples[i] = 8000
	}
	if err := audio.WriteWav(wavPath, &audio.PCM{SampleRate: 1000, Channels: 1, Data: samples}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "oto.ini"), []byte("a.wav=あ,0,0,0,0,0\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	server := &Server{voicebanks: map[string]Voicebank{
		"test": {ID: "test", Name: "test", Path: root},
	}}
	body := bytes.NewBufferString(`{"kana":"あ","voicebank_id":"test","mora_duration_ms":100}`)
	request := httptest.NewRequest(http.MethodPost, "/synthesize", body)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var payload struct {
		WAV       string `json:"wav_base64"`
		Engine    string `json:"engine"`
		UnitCount int    `json:"unit_count"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	wav, err := base64.StdEncoding.DecodeString(payload.WAV)
	if err != nil {
		t.Fatal(err)
	}
	if string(wav[:4]) != "RIFF" || payload.Engine != "waveform" || payload.UnitCount != 1 {
		t.Fatalf("unexpected response: %+v", payload)
	}
}

func TestSynthesizeEndpointRejectsUnknownText(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "oto.ini"), []byte("a.wav=あ,0,0,0,0,0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	server := &Server{voicebanks: map[string]Voicebank{
		"test": {ID: "test", Path: root},
	}}
	request := httptest.NewRequest(http.MethodPost, "/synthesize", bytes.NewBufferString(`{"text":"UtauTTS","voicebank_id":"test"}`))
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, request)
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestHealthReportsConfiguredRenderer(t *testing.T) {
	server := &Server{renderer: "worldline-hybrid"}
	request := httptest.NewRequest(http.MethodGet, "/health", nil)
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var payload map[string]string
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["status"] != "ok" || payload["engine"] != "worldline-hybrid" {
		t.Fatalf("unexpected response: %#v", payload)
	}
}

func TestVoicebankEndpointsDiscoverAndReloadDirectory(t *testing.T) {
	root := t.TempDir()
	makeBank := func(directory, name string) {
		dir := filepath.Join(root, directory)
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "oto.ini"), []byte("a.wav=あ,0,0,0,0,0\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "character.txt"), []byte("name="+name+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	makeBank("alpha", "アルファ")
	server := &Server{voicebanks: map[string]Voicebank{}, voiceDir: root}
	if err := server.loadVoiceDirectory(); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodGet, "/voicebanks", nil)
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, request)
	var first struct {
		Voicebanks []Voicebank `json:"voicebanks"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &first); err != nil {
		t.Fatal(err)
	}
	if len(first.Voicebanks) != 1 || first.Voicebanks[0].ID != "alpha" || first.Voicebanks[0].Name != "アルファ" {
		t.Fatalf("voicebanks = %#v", first.Voicebanks)
	}

	makeBank("beta", "ベータ")
	request = httptest.NewRequest(http.MethodPost, "/voicebanks/reload", nil)
	response = httptest.NewRecorder()
	server.routes().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("reload status = %d, body = %s", response.Code, response.Body.String())
	}
	var second struct {
		Voicebanks []Voicebank `json:"voicebanks"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &second); err != nil {
		t.Fatal(err)
	}
	if len(second.Voicebanks) != 2 || second.Voicebanks[1].ID != "beta" {
		t.Fatalf("reloaded voicebanks = %#v", second.Voicebanks)
	}
}
