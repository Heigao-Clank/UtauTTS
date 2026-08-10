package main

import (
	"embed"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"sort"
	"sync"

	"utautts/internal/audio"
	"utautts/internal/tts"
	"utautts/internal/voicebank"
)

//go:embed index.html
var webFiles embed.FS

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
	prosodyModelPath    string
	renderer            string
	worldlinePath       string
	worldlineBridgePath string
	voiceDir            string
}

func main() {
	var port int
	var host, voiceDir, prosodyPath, renderer, worldlinePath, worldlineBridgePath string
	flag.IntVar(&port, "port", 8080, "port")
	flag.StringVar(&host, "host", "127.0.0.1", "host")
	flag.StringVar(&voiceDir, "voice-dir", "voice", "directory containing voicebanks")
	flag.StringVar(&prosodyPath, "prosody", "", "optional learned prosody model JSON")
	flag.StringVar(&renderer, "renderer", "waveform", "renderer (default: waveform; other backends are experimental)")
	flag.StringVar(&worldlinePath, "worldline", "", "path to worldline library (default: next to executable)")
	flag.StringVar(&worldlineBridgePath, "worldline-bridge", "", "path to worldline bridge (default: next to executable)")
	flag.Parse()

	voiceDir = voicebank.ResolveDirectory(voiceDir)
	srv := &Server{
		voicebanks: map[string]Voicebank{}, prosodyModelPath: prosodyPath,
		renderer: renderer, worldlinePath: worldlinePath, worldlineBridgePath: worldlineBridgePath, voiceDir: voiceDir,
	}
	if err := srv.loadVoiceDirectory(); err != nil {
		log.Printf("warning: load voicebanks from %s: %v", voiceDir, err)
	}
	if len(srv.voicebanks) == 0 {
		log.Printf("warning: no voicebanks found in %s", voiceDir)
	}

	addr := fmt.Sprintf("%s:%d", host, port)
	log.Printf("listening on http://%s", addr)
	if err := http.ListenAndServe(addr, srv.routes()); err != nil {
		log.Fatal(err)
	}
}

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", s.handleIndex)
	mux.HandleFunc("POST /synthesize", s.handleSynthesize)
	mux.HandleFunc("GET /voicebanks", s.handleListVoicebanks)
	mux.HandleFunc("POST /voicebanks", s.handleRegisterVoicebank)
	mux.HandleFunc("POST /voicebanks/reload", s.handleReloadVoicebanks)
	mux.HandleFunc("GET /health", s.handleHealth)
	return mux
}

func (s *Server) loadVoiceDirectory() error {
	summaries, err := voicebank.Discover(s.voiceDir)
	if err != nil {
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
	return nil
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
		engine = "waveform"
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "engine": engine})
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
	var request struct {
		Name string `json:"name"`
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	vb, err := inspectVoicebank(request.Path)
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

func (s *Server) handleSynthesize(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Kana               string  `json:"kana"`
		Text               string  `json:"text"`
		VoicebankID        string  `json:"voicebank_id"`
		Tone               string  `json:"tone"`
		MoraDurationMS     float64 `json:"mora_duration_ms"`
		PauseDurationMS    float64 `json:"pause_duration_ms"`
		IntonationStrength float64 `json:"intonation_strength"`
		ApplyPitch         bool    `json:"apply_pitch"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	if request.Kana == "" && request.Text == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "text or kana is required"})
		return
	}
	vb, ok := s.resolveVoicebank(request.VoicebankID)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "voicebank not found"})
		return
	}

	result, err := tts.Synthesize(tts.Config{
		VoicebankPath:       vb.Path,
		Text:                request.Text,
		Reading:             request.Kana,
		Tone:                request.Tone,
		MoraDurationMS:      request.MoraDurationMS,
		PauseDurationMS:     request.PauseDurationMS,
		ProsodyModelPath:    s.prosodyModelPath,
		IntonationStrength:  request.IntonationStrength,
		ApplyPitch:          request.ApplyPitch,
		Renderer:            s.renderer,
		WorldlinePath:       s.worldlinePath,
		WorldlineBridgePath: s.worldlineBridgePath,
	})
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	wav := audio.PCMToWavBytes(result.Audio)
	engine := s.renderer
	if engine == "" {
		engine = "waveform"
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"wav_base64":  base64.StdEncoding.EncodeToString(wav),
		"sample_rate": result.Audio.SampleRate,
		"duration_ms": float64(len(result.Audio.Data)) * 1000 / float64(result.Audio.SampleRate),
		"unit_count":  len(result.Plan.Units),
		"engine":      engine,
		"reading":     result.Plan.Reading,
	})
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

func (s *Server) handleIndex(w http.ResponseWriter, _ *http.Request) {
	data, err := webFiles.ReadFile("index.html")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(data)
}
