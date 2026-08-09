//go:build windows

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSanitizeDisplayName(t *testing.T) {
	if got := sanitizeDisplayName("  音源\x00名\r\n  "); got != "音源 名" {
		t.Fatalf("sanitizeDisplayName() = %q", got)
	}
	long := strings.Repeat("声", 200)
	if got := []rune(sanitizeDisplayName(long)); len(got) != 121 || got[120] != '…' {
		t.Fatalf("long display name has %d runes", len(got))
	}
}

func TestDiscoverVoicebanksKeepsExplicitInitialBank(t *testing.T) {
	bank := filepath.Join(t.TempDir(), "bank")
	if err := os.Mkdir(bank, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bank, "oto.ini"), []byte("a.wav=あ,0,0,0,0,0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	banks, err := discoverVoicebanks(filepath.Join(t.TempDir(), "missing"), bank)
	if err == nil {
		t.Fatal("missing scan root did not report an error")
	}
	if len(banks) != 1 || !samePath(banks[0].Path, bank) {
		t.Fatalf("voicebanks = %#v", banks)
	}
}
