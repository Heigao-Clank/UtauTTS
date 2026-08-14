package api

import (
	"archive/zip"
	"bytes"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"utautts/internal/audio"
	"utautts/internal/frontend"
	"utautts/internal/plugin"
	"utautts/internal/prosody"
	"utautts/internal/render"
	"utautts/internal/tts"
	"utautts/internal/voicebank"
)

const (
	maxJSONRequestBytes   = 1 << 20
	maxSynthesisTextRunes = 500
	maxBatchItems         = 16
	maxBatchWAVBytes      = 256 << 20
	maxManualPitchPoints  = 1000
)

type Config struct {
	VoiceDir, Renderer                            string
	WorldlinePath, WorldlineBridgePath            string
	WorldlineR2MelPath, WorldlineR2VocoderPath    string
	OnnxDeviceID                                  int
	OpenJTalkPath, OpenJTalkDictionary, AuthToken string
	AllowVoicebankRegistration                    bool
	RendererDirectories, ModelDirectories         []string
}

type Voicebank struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Path            string `json:"path"`
	OtoFileCount    int    `json:"oto_file_count"`
	PhonemeCount    int    `json:"phoneme_count"`
	DiagnosticCount int    `json:"diagnostic_count"`
}

type Server struct {
	mu                  sync.RWMutex
	voicebanks          map[string]Voicebank
	renderer            string
	worldlinePath       string
	worldlineBridgePath string
	worldlineR2MelPath  string
	worldlineR2Vocoder  string
	onnxDeviceID        int
	openJTalkPath       string
	openJTalkDictionary string
	voiceDir            string
	cacheMu             sync.Mutex
	loadedVoicebanks    map[string]*voicebank.Bank
	modelMu             sync.Mutex
	loadedProsody       cachedProsodyModel
	authToken           string
	allowRegistration   bool
	catalog             *plugin.Catalog
}

type cachedProsodyModel struct {
	path    string
	modTime time.Time
	size    int64
	model   *prosody.Model
}

func New(config Config) (*Server, error) {
	voiceDir := voicebank.ResolveDirectory(config.VoiceDir)
	defaultRendererDirs, defaultModelDirs := plugin.DefaultDirectories()
	if len(config.RendererDirectories) == 0 {
		config.RendererDirectories = defaultRendererDirs
	}
	if len(config.ModelDirectories) == 0 {
		config.ModelDirectories = defaultModelDirs
	}
	catalog, err := plugin.Discover(config.RendererDirectories, config.ModelDirectories, render.IsKnownRenderer)
	if err != nil {
		return nil, fmt.Errorf("discover plugins: %w", err)
	}
	if config.Renderer == "" {
		config.Renderer = catalog.DefaultRenderer()
	}
	srv := &Server{
		voicebanks: map[string]Voicebank{},
		renderer:   config.Renderer, worldlinePath: config.WorldlinePath, worldlineBridgePath: config.WorldlineBridgePath, voiceDir: voiceDir,
		worldlineR2MelPath: config.WorldlineR2MelPath, worldlineR2Vocoder: config.WorldlineR2VocoderPath, onnxDeviceID: config.OnnxDeviceID,
		openJTalkPath: config.OpenJTalkPath, openJTalkDictionary: config.OpenJTalkDictionary,
		loadedVoicebanks: map[string]*voicebank.Bank{},
		authToken:        config.AuthToken, allowRegistration: config.AllowVoicebankRegistration,
		catalog: catalog,
	}
	if err := srv.loadVoiceDirectory(); err != nil {
		return nil, fmt.Errorf("load voicebanks from %s: %w", voiceDir, err)
	}
	if len(srv.voicebanks) == 0 {
		log.Printf("warning: no voicebanks found in %s", voiceDir)
	}

	return srv, nil
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", s.handleHealth)
	mux.HandleFunc("GET /api/voicebanks", s.handleListVoicebanks)
	mux.HandleFunc("POST /api/voicebanks", s.handleRegisterVoicebank)
	mux.HandleFunc("POST /api/voicebanks/reload", s.handleReloadVoicebanks)
	mux.HandleFunc("POST /api/synthesize/audio", s.handleSynthesizeAudio)
	mux.HandleFunc("POST /api/synthesize/batch", s.handleSynthesizeBatch)
	mux.HandleFunc("GET /api/models", s.handleModels)
	mux.HandleFunc("GET /api/renderers", s.handleRenderers)
	mux.HandleFunc("POST /api/analyze", s.handleAnalyze)
	return s.authenticate(mux)
}

func (s *Server) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			if origin := r.Header.Get("Origin"); origin != "" && origin != "http://"+r.Host && origin != "https://"+r.Host {
				http.Error(w, "cross-origin request rejected", http.StatusForbidden)
				return
			}
		}
		if s.authToken == "" {
			next.ServeHTTP(w, r)
			return
		}
		const prefix = "Bearer "
		authorization := r.Header.Get("Authorization")
		if len(authorization) <= len(prefix) || authorization[:len(prefix)] != prefix || !tokenEqual(authorization[len(prefix):], s.authToken) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func tokenEqual(left, right string) bool {
	return len(left) == len(right) && subtle.ConstantTimeCompare([]byte(left), []byte(right)) == 1
}

func (s *Server) loadVoiceDirectory() error {
	summaries, err := voicebank.Discover(s.voiceDir)
	if err != nil && !errors.Is(err, voicebank.ErrNoOto) && !os.IsNotExist(err) {
		s.mu.Lock()
		s.voicebanks = map[string]Voicebank{}
		s.mu.Unlock()
		return err
	}
	next := make(map[string]Voicebank, len(summaries))
	for _, summary := range summaries {
		id := filepath.Base(summary.Path)
		next[id] = Voicebank{ID: id, Name: summary.Name, Path: summary.Path}
		log.Printf("voicebank: %s (%s)", summary.Name, id)
	}
	s.mu.Lock()
	s.voicebanks = next
	s.mu.Unlock()
	s.cacheMu.Lock()
	s.loadedVoicebanks = make(map[string]*voicebank.Bank)
	s.cacheMu.Unlock()
	tts.ClearCaches()
	return nil
}

func (s *Server) cachedVoicebank(path string) (*voicebank.Bank, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	if s.loadedVoicebanks == nil {
		s.loadedVoicebanks = make(map[string]*voicebank.Bank)
	}
	if bank := s.loadedVoicebanks[absPath]; bank != nil {
		return bank, nil
	}
	bank, err := voicebank.Load(absPath)
	if err != nil {
		return nil, err
	}
	s.loadedVoicebanks[absPath] = bank
	return bank, nil
}

func (s *Server) cachedProsodyModelAt(path string) (*prosody.Model, error) {
	if path == "" {
		return nil, nil
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	s.modelMu.Lock()
	defer s.modelMu.Unlock()
	if cached := s.loadedProsody; cached.model != nil && cached.path == path && cached.size == info.Size() && cached.modTime.Equal(info.ModTime()) {
		return cached.model, nil
	}
	model, err := prosody.LoadModel(path)
	if err != nil {
		return nil, err
	}
	s.loadedProsody = cachedProsodyModel{path: path, size: info.Size(), modTime: info.ModTime(), model: model}
	return model, nil
}

func inspectVoicebank(path string) (Voicebank, error) {
	bank, err := voicebank.Load(path)
	if err != nil {
		return Voicebank{}, err
	}
	return Voicebank{
		ID:              filepath.Base(bank.Root),
		Name:            bank.Name,
		Path:            bank.Root,
		OtoFileCount:    len(bank.OtoFiles),
		PhonemeCount:    bank.EntryCount(),
		DiagnosticCount: len(bank.Diagnostics),
	}, nil
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	engine := s.renderer
	if engine == "" {
		engine = s.pluginCatalog().DefaultRenderer()
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "engine": engine})
}

func (s *Server) handleModels(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"models": s.pluginCatalog().Models})
}

func (s *Server) resolveProsodyModelPath(id string) (string, error) {
	if id == "none" {
		return "", nil
	}
	if id == "" {
		return "", nil
	}
	model, found := s.pluginCatalog().Model(id)
	if !found {
		return "", fmt.Errorf("prosody model not found")
	}
	return model.Path, nil
}

func (s *Server) handleRenderers(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"default_renderer": s.renderer, "renderers": s.pluginCatalog().Renderers})
}

func (s *Server) handleAnalyze(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Text string `json:"text"`
	}
	if err := decodeJSONBody(w, r, &request); err != nil {
		writeJSON(w, jsonDecodeStatus(err), map[string]string{"error": err.Error()})
		return
	}
	if request.Text == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "text is required"})
		return
	}
	reading, err := frontend.ToKana(request.Text)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	morae, err := frontend.ParseKana(reading)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	items := make([]map[string]any, 0, len(morae))
	for index, mora := range morae {
		items = append(items, map[string]any{"position": index, "mora": mora.Text, "pause": mora.Pause})
	}
	writeJSON(w, http.StatusOK, map[string]any{"reading": reading, "morae": items})
}

func (s *Server) handleListVoicebanks(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"voicebanks": s.voicebankList()})
}

func (s *Server) voicebankList() []Voicebank {
	s.mu.RLock()
	list := make([]Voicebank, 0, len(s.voicebanks))
	for _, vb := range s.voicebanks {
		list = append(list, vb)
	}
	s.mu.RUnlock()
	sort.Slice(list, func(i, j int) bool { return list[i].ID < list[j].ID })
	return list
}

func (s *Server) handleReloadVoicebanks(w http.ResponseWriter, _ *http.Request) {
	if err := s.loadVoiceDirectory(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"voicebanks": s.voicebankList()})
}

func (s *Server) handleRegisterVoicebank(w http.ResponseWriter, r *http.Request) {
	if !s.allowRegistration {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "voicebank registration is disabled"})
		return
	}
	var request struct {
		Name string `json:"name"`
		Path string `json:"path"`
	}
	if err := decodeJSONBody(w, r, &request); err != nil {
		writeJSON(w, jsonDecodeStatus(err), map[string]string{"error": err.Error()})
		return
	}
	path, err := pathWithin(s.voiceDir, request.Path)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	vb, err := inspectVoicebank(path)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if request.Name != "" {
		vb.Name = request.Name
	}
	s.mu.Lock()
	s.voicebanks[vb.ID] = vb
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, vb)
}

func pathWithin(root, candidate string) (string, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	candidate, err = filepath.Abs(candidate)
	if err != nil {
		return "", err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return "", fmt.Errorf("resolve voicebank root: %w", err)
	}
	candidate, err = filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", fmt.Errorf("resolve voicebank path: %w", err)
	}
	relative, err := filepath.Rel(root, candidate)
	if err != nil || relative == ".." || filepath.IsAbs(relative) || len(relative) >= 3 && relative[:3] == ".."+string(filepath.Separator) {
		return "", fmt.Errorf("voicebank path must be inside %s", root)
	}
	return candidate, nil
}

type SynthesisRequest struct {
	Kana               string                   `json:"kana"`
	Text               string                   `json:"text"`
	VoicebankID        string                   `json:"voicebank_id"`
	Tone               string                   `json:"tone"`
	MoraDurationMS     float64                  `json:"mora_duration_ms"`
	PauseDurationMS    float64                  `json:"pause_duration_ms"`
	IntonationStrength float64                  `json:"intonation_strength"`
	ApplyPitch         bool                     `json:"apply_pitch"`
	ManualPitch        *prosody.ManualPitchFile `json:"manual_pitch"`
	ModelID            string                   `json:"model_id"`
	Renderer           string                   `json:"renderer"`
}

func (s *Server) handleSynthesizeAudio(w http.ResponseWriter, r *http.Request) {
	var request SynthesisRequest
	if err := decodeJSONBody(w, r, &request); err != nil {
		writeJSON(w, jsonDecodeStatus(err), map[string]string{"error": err.Error()})
		return
	}
	result, engine, status, err := s.synthesize(request)
	if err != nil {
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "audio/wav")
	w.Header().Set("X-UtauTTS-Engine", engine)
	w.Header().Set("X-UtauTTS-Reading", result.Plan.Reading)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(audio.PCMToWavBytes(result.Audio))
}

func (s *Server) handleSynthesizeBatch(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Items []struct {
			Name    string           `json:"name"`
			Request SynthesisRequest `json:"request"`
		} `json:"items"`
	}
	if err := decodeJSONBody(w, r, &request); err != nil {
		writeJSON(w, jsonDecodeStatus(err), map[string]string{"error": err.Error()})
		return
	}
	if len(request.Items) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "items are required"})
		return
	}
	if len(request.Items) > maxBatchItems {
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": fmt.Sprintf("batch supports at most %d items", maxBatchItems)})
		return
	}
	var output bytes.Buffer
	archive := zip.NewWriter(&output)
	totalWAVBytes := 0
	for index, item := range request.Items {
		result, _, status, err := s.synthesize(item.Request)
		if err != nil {
			_ = archive.Close()
			writeJSON(w, status, map[string]string{"error": fmt.Sprintf("item %d: %v", index+1, err)})
			return
		}
		name := filepath.Base(item.Name)
		if name == "." || name == "" {
			name = fmt.Sprintf("utterance-%d.wav", index+1)
		}
		wav := audio.PCMToWavBytes(result.Audio)
		totalWAVBytes += len(wav)
		if totalWAVBytes > maxBatchWAVBytes {
			_ = archive.Close()
			writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "batch audio exceeds 256 MiB"})
			return
		}
		entry, err := archive.Create(name)
		if err != nil {
			_ = archive.Close()
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if _, err := entry.Write(wav); err != nil {
			_ = archive.Close()
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
	}
	if err := archive.Close(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="utautts-audio.zip"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(output.Bytes())
}

func (s *Server) synthesize(request SynthesisRequest) (*tts.Result, string, int, error) {
	if request.Kana == "" && request.Text == "" {
		return nil, "", http.StatusBadRequest, fmt.Errorf("text or kana is required")
	}
	if len([]rune(request.Text)) > maxSynthesisTextRunes || len([]rune(request.Kana)) > maxSynthesisTextRunes {
		return nil, "", http.StatusRequestEntityTooLarge, fmt.Errorf("text and kana are limited to %d characters", maxSynthesisTextRunes)
	}
	if request.MoraDurationMS < 0 || request.MoraDurationMS > 1000 || request.PauseDurationMS < 0 || request.PauseDurationMS > 3000 {
		return nil, "", http.StatusBadRequest, fmt.Errorf("duration settings are outside the supported range")
	}
	if request.IntonationStrength < 0 || request.IntonationStrength > 1 {
		return nil, "", http.StatusBadRequest, fmt.Errorf("intonation_strength must be between 0 and 1")
	}
	if request.ManualPitch != nil && len(request.ManualPitch.Points) > maxManualPitchPoints {
		return nil, "", http.StatusRequestEntityTooLarge, fmt.Errorf("manual pitch supports at most %d points", maxManualPitchPoints)
	}
	vb, ok := s.resolveVoicebank(request.VoicebankID)
	if !ok {
		return nil, "", http.StatusBadRequest, fmt.Errorf("voicebank not found")
	}
	bank, err := s.cachedVoicebank(vb.Path)
	if err != nil {
		return nil, "", http.StatusUnprocessableEntity, fmt.Errorf("load voicebank: %v", err)
	}
	modelPath, err := s.resolveProsodyModelPath(request.ModelID)
	if err != nil {
		return nil, "", http.StatusBadRequest, err
	}
	model, err := s.cachedProsodyModelAt(modelPath)
	if err != nil {
		return nil, "", http.StatusUnprocessableEntity, fmt.Errorf("load prosody model: %v", err)
	}

	rendererID := s.renderer
	if request.Renderer != "" {
		rendererID = request.Renderer
	}
	if rendererID == "" {
		rendererID = s.pluginCatalog().DefaultRenderer()
	}
	rendererPlugin, found := s.pluginCatalog().Renderer(rendererID)
	if !found {
		return nil, "", http.StatusBadRequest, fmt.Errorf("renderer plugin %q is not installed", rendererID)
	}
	if !render.IsKnownRenderer(rendererPlugin.Backend) {
		return nil, "", http.StatusBadRequest, fmt.Errorf("renderer plugin %q requires unavailable backend %q", rendererID, rendererPlugin.Backend)
	}
	result, err := tts.Synthesize(tts.Config{
		VoicebankPath:           vb.Path,
		Voicebank:               bank,
		Text:                    request.Text,
		Reading:                 request.Kana,
		Tone:                    request.Tone,
		MoraDurationMS:          request.MoraDurationMS,
		PauseDurationMS:         request.PauseDurationMS,
		ProsodyModelPath:        modelPath,
		ProsodyModel:            model,
		ManualPitch:             request.ManualPitch,
		IntonationStrength:      request.IntonationStrength,
		ApplyPitch:              request.ApplyPitch,
		Renderer:                rendererPlugin.Backend,
		RendererCapabilities:    &rendererPlugin.Capabilities,
		WorldlinePath:           configuredPluginAsset(s.worldlinePath, rendererPlugin, "worldline"),
		WorldlineBridgePath:     configuredPluginAsset(s.worldlineBridgePath, rendererPlugin, "worldline_bridge"),
		WorldlineR2MelPath:      configuredPluginAsset(s.worldlineR2MelPath, rendererPlugin, "worldline_r2_mel"),
		WorldlineR2VocoderPath:  configuredPluginAsset(s.worldlineR2Vocoder, rendererPlugin, "worldline_r2_vocoder"),
		OnnxDeviceID:            s.onnxDeviceID,
		OpenJTalkPath:           s.openJTalkPath,
		OpenJTalkDictionaryPath: s.openJTalkDictionary,
	})
	if err != nil {
		return nil, "", http.StatusUnprocessableEntity, err
	}
	return result, rendererID, http.StatusOK, nil
}

func configuredPluginAsset(explicit string, renderer plugin.Renderer, name string) string {
	if explicit != "" {
		return explicit
	}
	return renderer.Asset(name)
}

func (s *Server) pluginCatalog() *plugin.Catalog {
	if s.catalog == nil {
		return &plugin.Catalog{}
	}
	return s.catalog
}

func (s *Server) resolveVoicebank(id string) (Voicebank, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if id != "" {
		vb, ok := s.voicebanks[id]
		return vb, ok
	}
	ids := make([]string, 0, len(s.voicebanks))
	for candidate := range s.voicebanks {
		ids = append(ids, candidate)
	}
	if len(ids) == 0 {
		return Voicebank{}, false
	}
	sort.Strings(ids)
	return s.voicebanks[ids[0]], true
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func decodeJSONBody(w http.ResponseWriter, r *http.Request, destination any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONRequestBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return fmt.Errorf("request must contain one JSON object")
		}
		return err
	}
	return nil
}

func jsonDecodeStatus(err error) int {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		return http.StatusRequestEntityTooLarge
	}
	return http.StatusBadRequest
}
