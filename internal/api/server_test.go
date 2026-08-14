package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"utautts/internal/audio"
	"utautts/internal/plugin"
)

func mustNewServer(t *testing.T, config Config) *Server {
	t.Helper()
	server, err := New(config)
	if err != nil {
		t.Fatal(err)
	}
	return server
}

func TestProtectedHandlerRequiresBearerToken(t *testing.T) {
	server := mustNewServer(t, Config{AuthToken: "secret", VoiceDir: t.TempDir()})
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/health", nil))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", response.Code)
	}
	request := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	request.Header.Set("Authorization", "Bearer secret")
	response = httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("authenticated status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestUnknownPathIsNotIndex(t *testing.T) {
	response := httptest.NewRecorder()
	mustNewServer(t, Config{VoiceDir: t.TempDir()}).Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/missing", nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d", response.Code)
	}
}

func TestVoicebankRegistrationDisabledByDefault(t *testing.T) {
	response := httptest.NewRecorder()
	mustNewServer(t, Config{VoiceDir: t.TempDir()}).Handler().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/voicebanks", bytes.NewBufferString(`{"path":"."}`)))
	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d", response.Code)
	}
}

func TestRendererMetadataIncludesConfiguredDefault(t *testing.T) {
	response := httptest.NewRecorder()
	mustNewServer(t, Config{VoiceDir: t.TempDir(), Renderer: "worldline-hybrid"}).Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/renderers", nil))
	var payload struct {
		Default string `json:"default_renderer"`
	}
	_ = json.Unmarshal(response.Body.Bytes(), &payload)
	if payload.Default != "worldline-hybrid" {
		t.Fatalf("default = %q", payload.Default)
	}
}

func TestNewReportsInvalidPlugin(t *testing.T) {
	pluginDirectory := t.TempDir()
	if err := os.WriteFile(filepath.Join(pluginDirectory, "plugin.json"), []byte(`{"kind":"renderer"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := New(Config{VoiceDir: t.TempDir(), RendererDirectories: []string{pluginDirectory}}); err == nil {
		t.Fatal("invalid renderer plugin was silently ignored")
	}
}

func TestNewAllowsMissingVoiceDirectory(t *testing.T) {
	server, err := New(Config{VoiceDir: filepath.Join(t.TempDir(), "missing")})
	if err != nil {
		t.Fatal(err)
	}
	if len(server.voicebankList()) != 0 {
		t.Fatalf("voicebanks=%#v", server.voicebankList())
	}
}

func TestJSONAndBatchLimits(t *testing.T) {
	server := (&Server{voicebanks: map[string]Voicebank{}}).Handler()

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/analyze", strings.NewReader(`{"text":"あ","unknown":true}`))
	server.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("unknown field status = %d", response.Code)
	}

	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/api/synthesize/audio",
		strings.NewReader(`{"text":"`+strings.Repeat("あ", maxSynthesisTextRunes+1)+`"}`))
	server.ServeHTTP(response, request)
	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("long synthesis status = %d, body = %s", response.Code, response.Body.String())
	}

	items := strings.Repeat(`{"request":{"kana":"あ"}},`, maxBatchItems) + `{"request":{"kana":"あ"}}`
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/api/synthesize/batch", strings.NewReader(`{"items":[`+items+`]}`))
	server.ServeHTTP(response, request)
	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("large batch status = %d, body = %s", response.Code, response.Body.String())
	}

	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/api/analyze", strings.NewReader(strings.Repeat(" ", maxJSONRequestBytes+1)))
	server.ServeHTTP(response, request)
	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("large JSON status = %d", response.Code)
	}
}

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

	server := &Server{renderer: "waveform", catalog: &plugin.Catalog{Renderers: []plugin.Renderer{{ID: "waveform", Backend: "waveform"}}}, voicebanks: map[string]Voicebank{
		"test": {ID: "test", Name: "test", Path: root},
	}}
	body := bytes.NewBufferString(`{"kana":"あ","voicebank_id":"test","mora_duration_ms":100}`)
	request := httptest.NewRequest(http.MethodPost, "/api/synthesize/audio", body)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if wav := response.Body.Bytes(); len(wav) < 4 || string(wav[:4]) != "RIFF" || response.Header().Get("X-UtauTTS-Engine") != "waveform" {
		t.Fatalf("unexpected audio response: headers=%v bytes=%d", response.Header(), len(wav))
	}
}

func TestSynthesizeEndpointRejectsUnknownText(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "oto.ini"), []byte("a.wav=あ,0,0,0,0,0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	server := &Server{renderer: "waveform", catalog: &plugin.Catalog{Renderers: []plugin.Renderer{{ID: "waveform", Backend: "waveform"}}}, voicebanks: map[string]Voicebank{
		"test": {ID: "test", Path: root},
	}}
	request := httptest.NewRequest(http.MethodPost, "/api/synthesize/audio", bytes.NewBufferString(`{"text":"UtauTTS","voicebank_id":"test"}`))
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestHealthReportsConfiguredRenderer(t *testing.T) {
	server := &Server{renderer: "worldline-hybrid"}
	request := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)

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

func TestAPIMetadata(t *testing.T) {
	server := mustNewServer(t, Config{Renderer: "waveform", VoiceDir: t.TempDir()})
	request := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("health status = %d, body = %s", response.Code, response.Body.String())
	}

	request = httptest.NewRequest(http.MethodGet, "/api/renderers", nil)
	response = httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("renderers status = %d, body = %s", response.Code, response.Body.String())
	}
	var renderers struct {
		Renderers []struct {
			ID string `json:"id"`
		} `json:"renderers"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &renderers); err != nil {
		t.Fatal(err)
	}
	if len(renderers.Renderers) < 2 {
		t.Fatalf("renderers = %#v", renderers.Renderers)
	}
}

func TestAnalyzeEndpointReturnsMoraes(t *testing.T) {
	server := &Server{}
	request := httptest.NewRequest(http.MethodPost, "/api/analyze", bytes.NewBufferString(`{"text":"あいうえお"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var payload struct {
		Reading string `json:"reading"`
		Morae   []struct {
			Mora string `json:"mora"`
		} `json:"morae"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Reading == "" || len(payload.Morae) != 5 {
		t.Fatalf("unexpected analysis: %#v", payload)
	}
}

func TestServerCachesVoicebankLoads(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "oto.ini"), []byte("a.wav=縺・0,0,0,0,0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	server := &Server{}
	first, err := server.cachedVoicebank(root)
	if err != nil {
		t.Fatal(err)
	}
	second, err := server.cachedVoicebank(root)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatal("voicebank was loaded more than once")
	}
}

func TestServerReloadInvalidatesVoicebankCache(t *testing.T) {
	root := t.TempDir()
	bankDir := filepath.Join(root, "bank")
	if err := os.Mkdir(bankDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bankDir, "oto.ini"), []byte("a.wav=縺・0,0,0,0,0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	server := &Server{voicebanks: map[string]Voicebank{}, voiceDir: root}
	if err := server.loadVoiceDirectory(); err != nil {
		t.Fatal(err)
	}
	path := server.voicebankList()[0].Path
	first, err := server.cachedVoicebank(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := server.loadVoiceDirectory(); err != nil {
		t.Fatal(err)
	}
	second, err := server.cachedVoicebank(path)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("voicebank cache survived an explicit directory reload")
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

	request := httptest.NewRequest(http.MethodGet, "/api/voicebanks", nil)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
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
	request = httptest.NewRequest(http.MethodPost, "/api/voicebanks/reload", nil)
	response = httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
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
