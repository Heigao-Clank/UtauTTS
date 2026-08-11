package openjtalk

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"utautts/internal/prosody"
)

type Config struct {
	HelperPath     string
	DictionaryPath string
}

type Analysis struct {
	Version  int                    `json:"version"`
	Reading  string                 `json:"reading"`
	Morae    []string               `json:"morae"`
	Features []prosody.FeatureFrame `json:"features"`
}

func Analyze(text string, cfg Config) (*Analysis, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, fmt.Errorf("Open JTalk input is empty")
	}
	helper, err := resolveHelper(cfg.HelperPath)
	if err != nil {
		return nil, err
	}
	dictionary, err := resolveDictionary(cfg.DictionaryPath)
	if err != nil {
		return nil, err
	}
	request, err := json.Marshal(struct {
		Text string `json:"text"`
	}{Text: text})
	if err != nil {
		return nil, err
	}
	command := exec.Command(helper, "--dictionary", dictionary)
	command.Stdin = bytes.NewReader(request)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message != "" {
			return nil, fmt.Errorf("Open JTalk frontend failed: %w: %s", err, message)
		}
		return nil, fmt.Errorf("Open JTalk frontend failed: %w", err)
	}
	var result Analysis
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		return nil, fmt.Errorf("decode Open JTalk response: %w", err)
	}
	if result.Version != 1 || result.Reading == "" || len(result.Morae) == 0 || len(result.Features) != len(result.Morae) {
		return nil, fmt.Errorf("invalid Open JTalk response: version=%d morae=%d features=%d", result.Version, len(result.Morae), len(result.Features))
	}
	return &result, nil
}

func resolveHelper(explicit string) (string, error) {
	name := "utautts-openjtalk-features"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return resolveFile(explicit, name, []string{
		filepath.Join("runtime", name),
		filepath.Join("tools", "openjtalk-feature-bridge", "bin", name),
	})
}

func resolveDictionary(explicit string) (string, error) {
	const name = "open_jtalk_dic_utf_8-1.11"
	if explicit != "" {
		if info, err := os.Stat(explicit); err == nil && info.IsDir() {
			return filepath.Abs(explicit)
		}
		return "", fmt.Errorf("Open JTalk dictionary not found: %s", explicit)
	}
	for _, root := range searchRoots() {
		for _, relative := range []string{
			filepath.Join("runtime", name),
			filepath.Join(".tmp-openjtalk", "pyopenjtalk", name),
		} {
			candidate := filepath.Join(root, relative)
			if info, err := os.Stat(candidate); err == nil && info.IsDir() {
				return filepath.Abs(candidate)
			}
		}
	}
	return "", fmt.Errorf("Open JTalk dictionary not found; expected runtime/%s", name)
}

func resolveFile(explicit, description string, relatives []string) (string, error) {
	if explicit != "" {
		if info, err := os.Stat(explicit); err == nil && !info.IsDir() {
			return filepath.Abs(explicit)
		}
		return "", fmt.Errorf("%s not found: %s", description, explicit)
	}
	for _, root := range searchRoots() {
		for _, relative := range relatives {
			candidate := filepath.Join(root, relative)
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				return filepath.Abs(candidate)
			}
		}
	}
	return "", fmt.Errorf("%s not found", description)
}

func searchRoots() []string {
	var roots []string
	if executable, err := os.Executable(); err == nil {
		roots = append(roots, filepath.Dir(executable), filepath.Dir(filepath.Dir(executable)))
	}
	if current, err := os.Getwd(); err == nil {
		roots = append(roots, current)
	}
	return roots
}
