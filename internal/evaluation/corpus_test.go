package evaluation

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadCorpusValidatesIDs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "corpus.json")
	if err := os.WriteFile(path, []byte(`{"version":1,"name":"test","cases":[{"id":"a","text":"あ"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	corpus, err := LoadCorpus(path)
	if err != nil {
		t.Fatal(err)
	}
	if corpus.Name != "test" || len(corpus.Cases) != 1 {
		t.Fatalf("unexpected corpus: %+v", corpus)
	}
}

func TestLoadCorpusRejectsDuplicateIDs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "corpus.json")
	if err := os.WriteFile(path, []byte(`{"version":1,"name":"test","cases":[{"id":"a","text":"あ"},{"id":"a","text":"い"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadCorpus(path); err == nil {
		t.Fatal("expected duplicate ID error")
	}
}
