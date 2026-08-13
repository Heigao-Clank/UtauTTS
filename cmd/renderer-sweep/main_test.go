package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"utautts/internal/openutau"
	"utautts/internal/tts"
)

func TestProvenanceWarningsExposeRendererMismatch(t *testing.T) {
	project := &openutau.ProjectAudit{Tracks: []openutau.TrackRendererInfo{{Renderer: "CLASSIC", Resampler: "worldline", Wavtool: "convergence"}}}
	warnings := provenanceWarnings(tts.Config{Renderer: "utau-classic", UTAUResamplerPath: `C:\UTAU\resampler.exe`}, project)
	joined := strings.Join(warnings, "\n")
	if !strings.Contains(joined, "does not match OpenUtau resampler") || !strings.Contains(joined, "selected renderer manifest") {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
}

func TestProvenanceRecognizesFaithfulOpenUtauPath(t *testing.T) {
	project := &openutau.ProjectAudit{Tracks: []openutau.TrackRendererInfo{{Renderer: "CLASSIC", Resampler: "worldline", Wavtool: "convergence"}}}
	warnings := provenanceWarnings(tts.Config{Renderer: "openutau-classic-worldline-faithful"}, project)
	joined := strings.Join(warnings, "\n")
	if strings.Contains(joined, "does not match") || !strings.Contains(joined, "selected renderer manifest") {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
}

func TestIdentifyFileHashesExecutable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "resampler.exe")
	if err := os.WriteFile(path, []byte("test"), 0o600); err != nil {
		t.Fatal(err)
	}
	identity, err := identifyFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if identity.Size != 4 || len(identity.SHA256) != 64 {
		t.Fatalf("unexpected identity: %+v", identity)
	}
}
