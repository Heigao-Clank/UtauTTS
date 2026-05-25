package main

import (
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"utautts/internal/audio"
	"utautts/internal/oto"
	"utautts/internal/synth"
)

type Voicebank struct {
    ID           string `json:"id"`
    Name         string `json:"name"`
    Path         string `json:"path"`
    PhonemeCount int    `json:"phoneme_count"`
}

type Server struct {
    mu          sync.RWMutex
    voicebanks  map[string]Voicebank
    baseCfg     synth.Config
}

func main() {
    var (
        port       int
        host       string
        voicebankPath string
        modelPath  string
    )

    flag.IntVar(&port, "port", 8080, "port")
    flag.StringVar(&host, "host", "127.0.0.1", "host")
    flag.StringVar(&voicebankPath, "voicebank", "", "default voicebank path")
    flag.StringVar(&modelPath, "model", "", "JSUT model path for prediction")
    flag.Parse()

    srv := &Server{
        voicebanks: map[string]Voicebank{},
        baseCfg: synth.Config{
            GapMs:   40,
            CrossMs: 10,
            Pitch:   1.0,
            NoCurve: true,
        },
    }

    if voicebankPath != "" {
        if _, err := srv.registerVoicebank(voicebankPath); err != nil {
            log.Printf("register voicebank: %v", err)
        }
    }

    _ = modelPath

    mux := http.NewServeMux()
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
        Pitch       float64 `json:"pitch"`
    }
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
        return
    }
    if req.Text == "" {
        writeJSON(w, http.StatusBadRequest, map[string]string{"error": "text is required"})
        return
    }

    vbPath := ""
    if req.VoicebankID != "" {
        s.mu.RLock()
        vb, ok := s.voicebanks[req.VoicebankID]
        s.mu.RUnlock()
        if !ok {
            writeJSON(w, http.StatusNotFound, map[string]string{"error": "voicebank not found"})
            return
        }
        vbPath = vb.Path
    } else {
        s.mu.RLock()
        for _, vb := range s.voicebanks {
            vbPath = vb.Path
            break
        }
        s.mu.RUnlock()
    }

    if vbPath == "" {
        writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no voicebank available"})
        return
    }

    cfg := s.baseCfg
    cfg.OtoPath = filepath.Join(vbPath, "oto.ini")
    cfg.Text = req.Text
    if req.Pitch > 0 {
        cfg.Pitch = req.Pitch
    }

    pcm, err := synth.Synthesize(cfg)
    if err != nil {
        writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
        return
    }

    wavBytes := pcmToWavBytes(pcm)
    writeJSON(w, http.StatusOK, map[string]interface{}{
        "wav_base64":  base64.StdEncoding.EncodeToString(wavBytes),
        "sample_rate": pcm.SampleRate,
        "duration_ms": float64(len(pcm.Data)) / float64(pcm.Channels) / float64(pcm.SampleRate) * 1000.0,
    })
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
    otoIni, err := oto.ReadIni(otoPath)
    if err != nil {
        return Voicebank{}, err
    }
    entryCount := 0
    for _, entries := range otoIni.Entries {
        entryCount += len(entries)
    }
    id := filepath.Base(vbPath)
    return Voicebank{
        ID:           id,
        Name:         id,
        Path:         vbPath,
        PhonemeCount: entryCount,
    }, nil
}

func pcmToWavBytes(pcm *audio.PCM) []byte {
    buf := audio.PCMToWavBytes(pcm)
    return buf
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(status)
    json.NewEncoder(w).Encode(data)
}
