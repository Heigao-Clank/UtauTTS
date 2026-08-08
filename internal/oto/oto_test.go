package oto

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/text/encoding/japanese"
)

func TestReadIniUTF8DecimalsAndDiagnostics(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "oto.ini")
	content := "\ufeffa.wav=あ,12.5,100.25,-20.5,30.75,10.5\n" +
		"plain.wav=,,,,,\n" +
		"broken line\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	ini, err := ReadIni(path)
	if err != nil {
		t.Fatal(err)
	}
	if ini.Encoding != "UTF-8" {
		t.Fatalf("encoding = %q", ini.Encoding)
	}
	entry := ini.Entries["あ"][0]
	if entry.Offset != 12.5 || entry.Fixed != 100.25 || entry.Blank != -20.5 || entry.Preutterance != 30.75 || entry.Overlap != 10.5 {
		t.Fatalf("unexpected parameters: %+v", entry)
	}
	if got := ini.Entries["plain"][0].Alias; got != "plain" {
		t.Fatalf("blank alias fallback = %q", got)
	}
	if got := ini.Entries["plain"][0].Offset; got != 0 {
		t.Fatalf("blank numeric value = %v", got)
	}
	if len(ini.Diagnostics) != 1 || ini.Diagnostics[0].Line != 3 {
		t.Fatalf("diagnostics = %+v", ini.Diagnostics)
	}
}

func TestReadIniShiftJIS(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "oto.ini")
	encoded, err := japanese.ShiftJIS.NewEncoder().Bytes([]byte("voice.wav=こんにちは,0,100,-20,30,10\n"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, encoded, 0o644); err != nil {
		t.Fatal(err)
	}

	ini, err := ReadIni(path)
	if err != nil {
		t.Fatal(err)
	}
	if ini.Encoding != "Shift_JIS" {
		t.Fatalf("encoding = %q", ini.Encoding)
	}
	if len(ini.Entries["こんにちは"]) != 1 {
		t.Fatalf("entries = %#v", ini.Entries)
	}
}

func TestReadIniNormalizesWindowsRelativePath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "oto.ini")
	if err := os.WriteFile(path, []byte("sub\\a.wav=あ,0,0,0,0,0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ini, err := ReadIni(path)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, "sub", "a.wav")
	if got := ini.Entries["あ"][0].Filename; got != want {
		t.Fatalf("filename = %q, want %q", got, want)
	}
}
