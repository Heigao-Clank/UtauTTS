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
	"strings"
	"sync"

	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/transform"

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
			found := srv.scanVoicebanks(vbPath, 0)
			if !found {
				log.Printf("warning: no oto.ini found in %s", vbPath)
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

func (s *Server) scanVoicebanks(dir string, depth int) bool {
	if depth > 3 {
		return false
	}
	foundAny := false
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	hasOto := false
	for _, e := range entries {
		if e.Name() == "oto.ini" && !e.IsDir() {
			hasOto = true
			break
		}
	}
	if hasOto {
		vb, err := s.registerVoicebank(dir)
		if err == nil {
			vb.ID = filepath.Base(dir)
			vb.Name = vb.ID
			s.voicebanks[vb.ID] = vb
			log.Printf("voicebank: %s (%d entries)", vb.ID, vb.PhonemeCount)
			foundAny = true
		}
	}
	for _, e := range entries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			if s.scanVoicebanks(filepath.Join(dir, e.Name()), depth+1) {
				foundAny = true
			}
		}
	}
	return foundAny
}

func countOtoEntries(path string) int {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()
	decoder := japanese.ShiftJIS.NewDecoder()
	reader := transform.NewReader(f, decoder)
	data, _ := io.ReadAll(reader)
	lines := 0
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		fields := strings.Split(parts[1], ",")
		if len(fields) >= 6 {
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

