package main

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func makeZip(t *testing.T, files map[string]string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.zip")
	archive, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(archive)
	for name, content := range files {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestExtractZip(t *testing.T) {
	zipPath := makeZip(t, map[string]string{
		"app/utautts-gui.exe": "binary",
		"app/qml/Main.qml":    "main",
		"models/prosody.json": "model",
		"README.md":           "readme",
	})
	dest := filepath.Join(t.TempDir(), "stage")
	if err := extractZip(zipPath, dest); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		"app/utautts-gui.exe",
		"app/qml/Main.qml",
		"models/prosody.json",
		"README.md",
	} {
		if _, err := os.Stat(filepath.Join(dest, path)); err != nil {
			t.Errorf("missing extracted entry %s: %v", path, err)
		}
	}
}

func TestExtractZipRejectsPathTraversal(t *testing.T) {
	zipPath := makeZip(t, map[string]string{
		"../escape.txt": "boom",
	})
	if err := extractZip(zipPath, filepath.Join(t.TempDir(), "stage")); err == nil {
		t.Fatal("expected path traversal rejection")
	}
}

func TestNormalizeStageMovesTopLevelFolder(t *testing.T) {
	root := t.TempDir()
	stage := filepath.Join(root, "stage")
	writeTestFile(t, filepath.Join(stage, "UtauTTS", "app", "utautts-gui.exe"), "binary")
	writeTestFile(t, filepath.Join(stage, "UtauTTS", "voice", "bundle", "oto.ini"), "oto")
	if err := normalizeStage(stage); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(stage, "app", "utautts-gui.exe")); err != nil {
		t.Errorf("top-level folder was not moved up: %v", err)
	}
	if _, err := os.Stat(filepath.Join(stage, "voice", "bundle", "oto.ini")); err != nil {
		t.Errorf("nested content was not preserved: %v", err)
	}
	if _, err := os.Stat(filepath.Join(stage, "UtauTTS")); !os.IsNotExist(err) {
		t.Errorf("top-level folder should have been removed")
	}
}

func TestNormalizeStageFlatLayoutIsValid(t *testing.T) {
	stage := filepath.Join(t.TempDir(), "stage")
	writeTestFile(t, filepath.Join(stage, "utautts.exe"), "launcher")
	if err := normalizeStage(stage); err != nil {
		t.Fatalf("flat package layout should be accepted: %v", err)
	}
}

func TestNormalizeStageRejectsUnknownLayout(t *testing.T) {
	stage := filepath.Join(t.TempDir(), "stage")
	writeTestFile(t, filepath.Join(stage, "random.txt"), "junk")
	if err := normalizeStage(stage); err == nil {
		t.Fatal("expected unknown layout to be rejected")
	}
}

func TestCopyTree(t *testing.T) {
	source := filepath.Join(t.TempDir(), "source")
	destination := filepath.Join(t.TempDir(), "destination")
	writeTestFile(t, filepath.Join(source, "voice", "bank-a", "oto.ini"), "oto")
	writeTestFile(t, filepath.Join(source, "voice", "bank-a", "wav", "a.wav"), "audio")
	writeTestFile(t, filepath.Join(source, "voice", "bank-b", "oto.ini"), "oto2")
	if err := copyTree(source, destination); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		"voice/bank-a/oto.ini",
		"voice/bank-a/wav/a.wav",
		"voice/bank-b/oto.ini",
	} {
		content, err := os.ReadFile(filepath.Join(destination, path))
		if err != nil {
			t.Errorf("missing copied file %s: %v", path, err)
			continue
		}
		if len(content) == 0 {
			t.Errorf("copied file %s is empty", path)
		}
	}
}

func TestSanitizeToken(t *testing.T) {
	cases := map[string]string{
		"v0.0.6": "v0.0.6",
		"":       "latest",
		"v 1.0!": "v_1.0_",
		"rc-1_b": "rc-1_b",
	}
	for input, expected := range cases {
		if got := sanitizeToken(input); got != expected {
			t.Errorf("sanitizeToken(%q) = %q, want %q", input, got, expected)
		}
	}
}
