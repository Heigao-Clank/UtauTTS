package voicebank

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Summary is the lightweight information needed to present a voicebank picker.
type Summary struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// Discover finds voicebanks directly below root. If root itself is a
// voicebank, it is returned as the only result.
func Discover(root string) ([]Summary, error) {
	if root == "" {
		root = "voice"
	}
	if looksLikeVoicebankRoot(root) {
		if summary, err := Inspect(root); err == nil {
			return []Summary{summary}, nil
		}
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var result []Summary
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		summary, inspectErr := Inspect(filepath.Join(root, entry.Name()))
		if inspectErr != nil {
			continue
		}
		result = append(result, summary)
	}
	if len(result) == 0 {
		return nil, ErrNoOto
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Name == result[j].Name {
			return result[i].Path < result[j].Path
		}
		return result[i].Name < result[j].Name
	})
	return result, nil
}

// ResolveDirectory makes a configured voice directory independent from the
// process working directory. An existing working-directory path wins for
// development; packaged applications otherwise use a path beside the binary.
func ResolveDirectory(configured string) string {
	if configured == "" {
		configured = "voice"
	}
	if filepath.IsAbs(configured) {
		return filepath.Clean(configured)
	}
	if absolute, err := filepath.Abs(configured); err == nil {
		if info, statErr := os.Stat(absolute); statErr == nil && info.IsDir() {
			return absolute
		}
	}
	if executable, err := os.Executable(); err == nil {
		return filepath.Join(filepath.Dir(executable), configured)
	}
	return configured
}

var errOtoFound = errors.New("oto.ini found")

// Inspect reads only enough metadata to show a voicebank in a picker. Unlike
// Load, it does not parse every oto.ini entry.
func Inspect(root string) (Summary, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return Summary{}, err
	}
	info, err := os.Stat(absRoot)
	if err != nil {
		return Summary{}, err
	}
	if !info.IsDir() {
		if !strings.EqualFold(filepath.Base(absRoot), "oto.ini") {
			return Summary{}, ErrNoOto
		}
		absRoot = filepath.Dir(absRoot)
	}
	found := false
	err = filepath.WalkDir(absRoot, func(_ string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() && strings.EqualFold(entry.Name(), "oto.ini") {
			found = true
			return errOtoFound
		}
		return nil
	})
	if err != nil && !errors.Is(err, errOtoFound) {
		return Summary{}, err
	}
	if !found {
		return Summary{}, ErrNoOto
	}
	name := filepath.Base(absRoot)
	if path := findRootFile(absRoot, "character.txt"); path != "" {
		if text, readErr := readMetadata(path); readErr == nil {
			for _, line := range strings.Split(text, "\n") {
				parts := strings.SplitN(strings.TrimSpace(line), "=", 2)
				if len(parts) == 2 && strings.EqualFold(strings.TrimSpace(parts[0]), "name") {
					if configured := strings.TrimSpace(parts[1]); configured != "" {
						name = configured
					}
				}
			}
		}
	}
	return Summary{Name: name, Path: absRoot}, nil
}

func looksLikeVoicebankRoot(root string) bool {
	entries, err := os.ReadDir(root)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		switch {
		case strings.EqualFold(entry.Name(), "oto.ini"), strings.EqualFold(entry.Name(), "character.txt"), strings.EqualFold(entry.Name(), "prefix.map"):
			return true
		}
	}
	return false
}
