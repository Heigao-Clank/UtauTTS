package main

import (
	"embed"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"utautts/internal/engine"
)

//go:embed index.html
var webFiles embed.FS

type Voicebank struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Path         string `json:"path"`
	PhonemeCount int    `json:"phoneme_count"`
}

type Server struct {
	mu         sync.RWMutex
	voicebanks map[string]Voicebank
	modelPath  string
	python     string
	toolsDir   string
}

func main() {
	var (
		port       int
		host       string
		voiceDir   string
		modelPath  string
		pythonPath string
		toolsDir   string
	)

	exe, _ := os.Executable()
	defaultTools := filepath.Dir(exe)

	flag.IntVar(&port, "port", 8080, "port")
	flag.StringVar(&host, "host", "127.0.0.1", "host")
	flag.StringVar(&voiceDir, "voice-dir", "voice", "voicebank directory")
	flag.StringVar(&modelPath, "model", "", "DNN model path (.pth)")
	flag.StringVar(&pythonPath, "python", "python", "python executable")
	flag.StringVar(&toolsDir, "tools", defaultTools, "tools directory")
	flag.Parse()

	srv := &Server{
		voicebanks: map[string]Voicebank{},
		modelPath:  modelPath,
		python:     pythonPath,
		toolsDir:   toolsDir,
	}

	if info, err := os.Stat(voiceDir); err == nil && info.IsDir() {
		entries, _ := os.ReadDir(voiceDir)
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			vbPath := filepath.Join(voiceDir, e.Name())
			if vb, err := srv.registerVoicebank(vbPath); err == nil {
				srv.voicebanks[vb.ID] = vb
				log.Printf("voicebank: %s (%d entries)", vb.ID, vb.PhonemeCount)
				continue
			}
			subEntries, _ := os.ReadDir(vbPath)
			for _, se := range subEntries {
				if !se.IsDir() {
					continue
				}
				subPath := filepath.Join(vbPath, se.Name())
				if vb, err := srv.registerVoicebank(subPath); err == nil {
					srv.voicebanks[vb.ID] = vb
					log.Printf("voicebank: %s (%d entries)", vb.ID, vb.PhonemeCount)
				}
			}
		}
	}

	if len(srv.voicebanks) == 0 {
		log.Printf("warning: no voicebanks found in %s", voiceDir)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /", srv.handleIndex)
	mux.HandleFunc("POST /synthesize", srv.handleSynthesize)
	mux.HandleFunc("GET /voicebanks", srv.handleListVoicebanks)
	mux.HandleFunc("POST /voicebanks", srv.handleRegisterVoicebank)
	mux.HandleFunc("GET /health", srv.handleHealth)

	addr := fmt.Sprintf("%s:%d", host, port)
	log.Printf("listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleListVoicebanks(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	list := make([]Voicebank, 0, len(s.voicebanks))
	for _, vb := range s.voicebanks {
		list = append(list, vb)
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"voicebanks": list})
}

func (s *Server) handleRegisterVoicebank(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	vb, err := s.registerVoicebank(req.Path)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if req.Name != "" {
		vb.Name = req.Name
	}
	s.mu.Lock()
	s.voicebanks[vb.ID] = vb
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, vb)
}

func (s *Server) handleSynthesize(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Text        string `json:"text"`
		VoicebankID string `json:"voicebank_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	if req.Text == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "text is required"})
		return
	}

	vbPath := s.resolveVoicebank(req.VoicebankID)
	if vbPath == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no voicebank available"})
		return
	}
	if s.modelPath == "" {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "model path not configured"})
		return
	}

	tmpDir, err := os.MkdirTemp("", "utautts-server-*")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer os.RemoveAll(tmpDir)

	cfg := engine.Config{
		OtoPath:   filepath.Join(vbPath, "oto.ini"),
		Text:      req.Text,
		OutPath:   filepath.Join(tmpDir, "out.wav"),
		ModelPath: s.modelPath,
		Python:    s.python,
		ToolsDir:  s.toolsDir,
	}

	if err := engine.Synthesize(cfg); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	wavBytes, err := os.ReadFile(cfg.OutPath)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"wav_base64": base64.StdEncoding.EncodeToString(wavBytes),
	})
}

func (s *Server) resolveVoicebank(id string) string {
	if id != "" {
		s.mu.RLock()
		vb, ok := s.voicebanks[id]
		s.mu.RUnlock()
		if ok {
			return vb.Path
		}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, vb := range s.voicebanks {
		return vb.Path
	}
	return ""
}

func (s *Server) registerVoicebank(vbPath string) (Voicebank, error) {
	info, err := os.Stat(vbPath)
	if err != nil || !info.IsDir() {
		return Voicebank{}, fmt.Errorf("voicebank not found: %s", vbPath)
	}
	otoPath := filepath.Join(vbPath, "oto.ini")
	if _, err := os.Stat(otoPath); err != nil {
		return Voicebank{}, fmt.Errorf("oto.ini not found in %s", vbPath)
	}
	entryCount := countOtoEntries(otoPath)
	id := filepath.Base(vbPath)
	return Voicebank{
		ID:           id,
		Name:         id,
		Path:         vbPath,
		PhonemeCount: entryCount,
	}, nil
}

func countOtoEntries(path string) int {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()
	data, _ := io.ReadAll(f)
	lines := 0
	for _, b := range data {
		if b == '\n' {
			lines++
		}
	}
	return lines
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	data, _ := webFiles.ReadFile("index.html")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(data)
}

