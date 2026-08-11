package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadProsodyFeatures(t *testing.T) {
	path := filepath.Join(t.TempDir(), "features.json")
	data := []byte(`{"version":1,"cases":[{"id":"sample","features":[{"accent_high":1},{"accent_high":0,"word_end":1}]}]}`)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	frames, err := loadProsodyFeatures(path, "sample", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) != 2 || frames[0]["accent_high"] != 1 || frames[1]["word_end"] != 1 {
		t.Fatalf("unexpected frames: %#v", frames)
	}
}
