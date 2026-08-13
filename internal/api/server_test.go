package api

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

func TestProtectedHandlerRequiresTokenAndExchangesCookie(t *testing.T) {
	server := New(Config{AuthToken: "secret", VoiceDir: t.TempDir()})
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/health", nil))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", response.Code)
	}
	response = httptest.NewRecorder()
	server.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/?token=secret", nil))
	if response.Code != http.StatusSeeOther || len(response.Result().Cookies()) == 0 {
		t.Fatalf("bootstrap = %d, cookies=%v", response.Code, response.Result().Cookies())
	}
	cookie := response.Result().Cookies()[0]
	for _, path := range []string{"/", "/ui/app.js", "/api/health"} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.AddCookie(cookie)
		response = httptest.NewRecorder()
		server.Handler().ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("authenticated %s status = %d, body = %s", path, response.Code, response.Body.String())
		}
	}
}

func TestUnknownPathIsNotIndex(t *testing.T) {
	response := httptest.NewRecorder()
	New(Config{VoiceDir: t.TempDir()}).Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/missing", nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d", response.Code)
	}
}

func TestVoicebankRegistrationDisabledByDefault(t *testing.T) {
	response := httptest.NewRecorder()
	New(Config{VoiceDir: t.TempDir()}).Handler().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/voicebanks", bytes.NewBufferString(`{"path":"."}`)))
	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d", response.Code)
	}
}

func TestRendererMetadataIncludesConfiguredDefault(t *testing.T) {
	response := httptest.NewRecorder()
	New(Config{VoiceDir: t.TempDir(), Renderer: "worldline-hybrid"}).Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/renderers", nil))
	var payload struct {
		Default string `json:"default_renderer"`
	}
	_ = json.Unmarshal(response.Body.Bytes(), &payload)
	if payload.Default != "worldline-hybrid" {
		t.Fatalf("default = %q", payload.Default)
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

	server := &Server{voicebanks: map[string]Voicebank{
		"test": {ID: "test", Name: "test", Path: root},
	}}
	body := bytes.NewBufferString(`{"kana":"あ","voicebank_id":"test","mora_duration_ms":100}`)
	request := httptest.NewRequest(http.MethodPost, "/synthesize", body)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)

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
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestHealthReportsConfiguredRenderer(t *testing.T) {
	server := &Server{renderer: "worldline-hybrid"}
	request := httptest.NewRequest(http.MethodGet, "/health", nil)
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

func TestAPIAliasesExposeMetadata(t *testing.T) {
	server := &Server{renderer: "waveform", voicebanks: map[string]Voicebank{}}
	for _, path := range []string{"/health", "/api/health"} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		response := httptest.NewRecorder()
		server.Handler().ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("%s status = %d, body = %s", path, response.Code, response.Body.String())
		}
	}

	request := httptest.NewRequest(http.MethodGet, "/api/renderers", nil)
	response := httptest.NewRecorder()
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

func TestResolveProsodyModelPathOnlyAllowsConfiguredModel(t *testing.T) {
	server := &Server{prosodyModelPath: filepath.Join("models", "v8.json")}
	for _, id := range []string{"", server.prosodyModelPath, "v8.json"} {
		path, err := server.resolveProsodyModelPath(id)
		if err != nil || path != server.prosodyModelPath {
			t.Fatalf("id %q resolved to %q, err=%v", id, path, err)
		}
	}
	if _, err := server.resolveProsodyModelPath("other.json"); err == nil {
		t.Fatal("unconfigured model was accepted")
	}
	if path, err := server.resolveProsodyModelPath("none"); err != nil || path != "" {
		t.Fatalf("none resolved to %q, err=%v", path, err)
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

	request := httptest.NewRequest(http.MethodGet, "/voicebanks", nil)
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
	request = httptest.NewRequest(http.MethodPost, "/voicebanks/reload", nil)
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
