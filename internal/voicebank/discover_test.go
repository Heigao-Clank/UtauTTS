package voicebank

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverVoicebanksUsesMetadataNameAndSorts(t *testing.T) {
	root := t.TempDir()
	makeBank := func(directory, name string) {
		dir := filepath.Join(root, directory)
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "oto.ini"), []byte("a.wav=あ,0,0,0,0,0\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "character.txt"), []byte("name="+name+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	makeBank("second", "乙")
	makeBank("first", "甲")
	if err := os.Mkdir(filepath.Join(root, "not-a-bank"), 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Name != "乙" || got[1].Name != "甲" {
		t.Fatalf("voicebanks = %#v", got)
	}
}

func TestDiscoverAcceptsVoicebankRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "oto.ini"), []byte("a.wav=あ,0,0,0,0,0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Discover(root)
	if err != nil || len(got) != 1 || got[0].Path != root {
		t.Fatalf("Discover() = %#v, %v", got, err)
	}
}

func TestInspectFindsNestedOtoWithoutParsingIt(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "append")
	if err := os.Mkdir(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "oto.ini"), []byte("this is not a valid oto line\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Inspect(root)
	if err != nil || got.Path != root {
		t.Fatalf("Inspect() = %#v, %v", got, err)
	}
}

func TestResolveDirectoryKeepsAbsolutePath(t *testing.T) {
	root := t.TempDir()
	if got := ResolveDirectory(root); got != root {
		t.Fatalf("ResolveDirectory() = %q, want %q", got, root)
	}
}
