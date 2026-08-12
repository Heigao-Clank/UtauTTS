package voicebank

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"utautts/internal/connection"
	"utautts/internal/oto"
)

var ErrNoOto = errors.New("voicebank contains no oto.ini")

type Diagnostic struct {
	Path    string
	Line    int
	Message string
}

type Bank struct {
	Root        string
	Name        string
	OtoFiles    []string
	Entries     map[string][]oto.Entry
	PrefixMap   map[string]Affix
	Diagnostics []Diagnostic
	extractor   *connection.Extractor
}

func Load(root string) (*Bank, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(absRoot)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		if !strings.EqualFold(filepath.Base(absRoot), "oto.ini") {
			return nil, fmt.Errorf("voicebank path must be a directory or oto.ini: %s", root)
		}
		absRoot = filepath.Dir(absRoot)
	}

	var otoFiles []string
	err = filepath.WalkDir(absRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if strings.EqualFold(entry.Name(), "oto.ini") {
			otoFiles = append(otoFiles, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(otoFiles) == 0 {
		return nil, ErrNoOto
	}
	sort.Strings(otoFiles)

	bank := &Bank{
		Root:      absRoot,
		Name:      filepath.Base(absRoot),
		OtoFiles:  otoFiles,
		Entries:   map[string][]oto.Entry{},
		PrefixMap: map[string]Affix{},
		extractor: connection.NewExtractor(),
	}
	bank.loadMetadata()
	for _, path := range otoFiles {
		ini, err := oto.ReadIni(path)
		if err != nil {
			return nil, err
		}
		for alias, entries := range ini.Entries {
			bank.Entries[alias] = append(bank.Entries[alias], entries...)
		}
		for _, diagnostic := range ini.Diagnostics {
			bank.Diagnostics = append(bank.Diagnostics, Diagnostic{
				Path:    path,
				Line:    diagnostic.Line,
				Message: diagnostic.Message,
			})
		}
	}
	return bank, nil
}

func (b *Bank) Aliases() []string {
	aliases := make([]string, 0, len(b.Entries))
	for alias := range b.Entries {
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)
	return aliases
}

func (b *Bank) EntryCount() int {
	count := 0
	for _, entries := range b.Entries {
		count += len(entries)
	}
	return count
}
