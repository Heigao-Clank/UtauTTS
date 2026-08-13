package plugin

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"utautts/internal/prosody"
)

const ManifestVersion = 1

type Capabilities struct {
	FramePitch     bool `json:"frame_pitch,omitempty"`
	BoundaryBridge bool `json:"boundary_bridge,omitempty"`
}

type Renderer struct {
	ManifestVersion int               `json:"manifest_version"`
	Kind            string            `json:"kind"`
	ID              string            `json:"id"`
	DisplayName     string            `json:"display_name"`
	Description     string            `json:"description,omitempty"`
	Backend         string            `json:"backend"`
	Version         string            `json:"version,omitempty"`
	Experimental    bool              `json:"experimental,omitempty"`
	Acceleration    string            `json:"acceleration,omitempty"`
	DefaultPriority int               `json:"default_priority,omitempty"`
	Capabilities    Capabilities      `json:"capabilities,omitempty"`
	Assets          map[string]string `json:"assets,omitempty"`
	Directory       string            `json:"-"`
}

type Model struct {
	ID                   string   `json:"id"`
	DisplayName          string   `json:"display_name"`
	Description          string   `json:"description,omitempty"`
	Path                 string   `json:"path"`
	Version              int      `json:"version"`
	Mode                 string   `json:"mode"`
	RecommendedRenderers []string `json:"recommended_renderers,omitempty"`
	DefaultPriority      int      `json:"default_priority,omitempty"`
	RequiresFeatures     bool     `json:"requires_features,omitempty"`
	FrameContour         bool     `json:"frame_contour,omitempty"`
}

type Catalog struct {
	Renderers []Renderer
	Models    []Model
}

func Discover(rendererDirectories, modelDirectories []string, supportsBackend func(string) bool) (*Catalog, error) {
	renderers, rendererErr := DiscoverRenderers(rendererDirectories, supportsBackend)
	models, modelErr := DiscoverModels(modelDirectories)
	return &Catalog{Renderers: renderers, Models: models}, errors.Join(rendererErr, modelErr)
}

func DiscoverRenderers(directories []string, supportsBackend func(string) bool) ([]Renderer, error) {
	seen := map[string]string{}
	var result []Renderer
	var problems []error
	for _, root := range uniqueDirectories(directories) {
		_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return nil
			}
			if entry.IsDir() || !strings.EqualFold(entry.Name(), "plugin.json") {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				problems = append(problems, fmt.Errorf("read renderer manifest %q: %w", path, err))
				return nil
			}
			var renderer Renderer
			if err := json.Unmarshal(data, &renderer); err != nil {
				problems = append(problems, fmt.Errorf("decode renderer manifest %q: %w", path, err))
				return nil
			}
			if err := validateRenderer(renderer, supportsBackend); err != nil {
				problems = append(problems, fmt.Errorf("renderer manifest %q: %w", path, err))
				return nil
			}
			key := strings.ToLower(renderer.ID)
			if previous, exists := seen[key]; exists {
				problems = append(problems, fmt.Errorf("duplicate renderer id %q in %q and %q", renderer.ID, previous, path))
				return nil
			}
			seen[key] = path
			renderer.Directory = filepath.Dir(path)
			result = append(result, renderer)
			return nil
		})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].DefaultPriority != result[j].DefaultPriority {
			return result[i].DefaultPriority > result[j].DefaultPriority
		}
		return result[i].DisplayName < result[j].DisplayName
	})
	return result, errors.Join(problems...)
}

func DiscoverModels(directories []string) ([]Model, error) {
	seenIDs, seenPaths := map[string]string{}, map[string]bool{}
	var result []Model
	var problems []error
	for _, directory := range uniqueDirectories(directories) {
		entries, err := os.ReadDir(directory)
		if err != nil {
			if !os.IsNotExist(err) {
				problems = append(problems, fmt.Errorf("read model directory %q: %w", directory, err))
			}
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".json") {
				continue
			}
			path, err := filepath.Abs(filepath.Join(directory, entry.Name()))
			if err != nil || seenPaths[strings.ToLower(path)] {
				continue
			}
			loaded, err := prosody.LoadModel(path)
			if err != nil {
				continue
			}
			id := strings.TrimSpace(loaded.ID)
			name := strings.TrimSpace(loaded.DisplayName)
			if id == "" || name == "" {
				continue
			}
			key := strings.ToLower(id)
			if previous, exists := seenIDs[key]; exists {
				problems = append(problems, fmt.Errorf("duplicate model id %q in %q and %q", id, previous, path))
				continue
			}
			seenIDs[key], seenPaths[strings.ToLower(path)] = path, true
			result = append(result, Model{
				ID: id, DisplayName: name, Description: loaded.Description, Path: path,
				Version: loaded.Version, Mode: loaded.Mode,
				RecommendedRenderers: append([]string(nil), loaded.RecommendedRenderers...),
				DefaultPriority:      loaded.DefaultPriority,
				RequiresFeatures:     loaded.RequiresExternalFeatures(), FrameContour: loaded.HasFrameContour(),
			})
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].DefaultPriority != result[j].DefaultPriority {
			return result[i].DefaultPriority > result[j].DefaultPriority
		}
		return result[i].DisplayName < result[j].DisplayName
	})
	return result, errors.Join(problems...)
}

func (catalog *Catalog) DefaultRenderer() string {
	if len(catalog.Renderers) == 0 {
		return ""
	}
	return catalog.Renderers[0].ID
}

func (catalog *Catalog) Renderer(id string) (Renderer, bool) {
	for _, item := range catalog.Renderers {
		if item.ID == id {
			return item, true
		}
	}
	return Renderer{}, false
}

func (renderer Renderer) Asset(name string) string {
	value := strings.TrimSpace(renderer.Assets[name])
	if value == "" || filepath.IsAbs(value) {
		return value
	}
	return filepath.Clean(filepath.Join(renderer.Directory, value))
}

func (catalog *Catalog) Model(id string) (Model, bool) {
	for _, item := range catalog.Models {
		if item.ID == id || item.Path == id {
			return item, true
		}
	}
	return Model{}, false
}

func DefaultDirectories() (rendererDirectories, modelDirectories []string) {
	if executable, err := os.Executable(); err == nil {
		root := filepath.Dir(executable)
		if strings.EqualFold(filepath.Base(root), "tools") || strings.EqualFold(filepath.Base(root), "app") {
			root = filepath.Dir(root)
		}
		rendererDirectories = append(rendererDirectories, filepath.Join(root, "plugins", "renderers"))
		modelDirectories = append(modelDirectories, filepath.Join(root, "models"))
	}
	if current, err := os.Getwd(); err == nil {
		root := workspaceRoot(current)
		rendererDirectories = append(rendererDirectories, filepath.Join(root, "plugins", "renderers"))
		modelDirectories = append(modelDirectories, filepath.Join(root, "models"))
	}
	return
}

func workspaceRoot(start string) string {
	current := filepath.Clean(start)
	for {
		if info, err := os.Stat(filepath.Join(current, "go.mod")); err == nil && !info.IsDir() {
			return current
		}
		parent := filepath.Dir(current)
		if parent == current {
			return start
		}
		current = parent
	}
}

func validateRenderer(renderer Renderer, supportsBackend func(string) bool) error {
	if renderer.ManifestVersion != ManifestVersion {
		return fmt.Errorf("unsupported manifest_version %d", renderer.ManifestVersion)
	}
	if renderer.Kind != "renderer" || renderer.ID == "" || renderer.DisplayName == "" || renderer.Backend == "" {
		return errors.New("kind=renderer, id, display_name, and backend are required")
	}
	if renderer.Acceleration != "" && renderer.Acceleration != "cpu" && renderer.Acceleration != "cuda" && renderer.Acceleration != "directml" {
		return fmt.Errorf("unsupported acceleration %q", renderer.Acceleration)
	}
	if supportsBackend != nil && !supportsBackend(renderer.Backend) {
		return fmt.Errorf("backend %q is not installed", renderer.Backend)
	}
	return nil
}

func uniqueDirectories(values []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		absolute, err := filepath.Abs(value)
		if err != nil {
			continue
		}
		key := strings.ToLower(filepath.Clean(absolute))
		if !seen[key] {
			seen[key] = true
			result = append(result, absolute)
		}
	}
	return result
}
