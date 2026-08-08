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

func TestSynthesizeEndpointUsesDeterministicEngine(t *testing.T) {
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
	if string(wav[:4]) != "RIFF" || payload.Engine != "deterministic-v1" || payload.UnitCount != 1 {
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
