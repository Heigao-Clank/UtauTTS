package plugin

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRepositoryRendererPluginsAreSelfDescribing(t *testing.T) {
	rendererDirectories, _ := DefaultDirectories()
	items, err := DiscoverRenderers(rendererDirectories, func(string) bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	if len(items) < 2 {
		t.Fatalf("renderer plugins = %d, want multiple independently described plugins", len(items))
	}
	if items[0].ID != "waveform" {
		t.Fatalf("default renderer = %q, want manifest-priority waveform", items[0].ID)
	}
	for _, item := range items {
		if item.ID == "" || item.DisplayName == "" || item.Backend == "" || item.Directory == "" {
			t.Fatalf("incomplete renderer plugin: %#v", item)
		}
	}
}

func TestModelUsesIdentityStoredInsideModel(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "arbitrary-filename.json")
	data := []byte(`{
  "id":"intonation.example", "display_name":"Example model", "description":"self described",
  "recommended_renderers":["waveform"], "version":9, "feature_version":1,
  "mode":"standard_japanese_accent", "duration_weights":{},
  "standard_accent":{"frame_ms":10,"accent_range_cents":100,"declination_cents":20,"question_rise_cents":50,"smoothing_ms":20,"p99_cents":200,"max_cents":250},
  "metrics":{}, "training":{}
}`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	models, err := DiscoverModels([]string{directory})
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 1 || models[0].ID != "intonation.example" || models[0].DisplayName != "Example model" {
		t.Fatalf("models = %#v", models)
	}
}

func TestModelWithoutIdentityIsNotCatalogued(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "filename-must-not-become-an-id.json")
	data := []byte(`{
  "version":9, "feature_version":1, "mode":"standard_japanese_accent",
  "duration_weights":{},
  "standard_accent":{"frame_ms":10,"accent_range_cents":100,"declination_cents":20,"question_rise_cents":50,"smoothing_ms":20,"p99_cents":200,"max_cents":250},
  "metrics":{}, "training":{}
}`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	models, err := DiscoverModels([]string{directory})
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 0 {
		t.Fatalf("identity-free model was catalogued from its filename: %#v", models)
	}
}

func TestInvalidModelIsReported(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "broken.json"), []byte(`{"version":`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := DiscoverModels([]string{directory}); err == nil {
		t.Fatal("invalid model was silently ignored")
	}
}

func TestRepositoryBundlesSelfDescribingModels(t *testing.T) {
	_, modelDirectories := DefaultDirectories()
	models, err := DiscoverModels(modelDirectories)
	if err != nil {
		t.Fatal(err)
	}
	if len(models) == 0 {
		t.Fatal("no bundled self-describing models found")
	}
	if models[0].ID != "frame-intonation-v8" {
		t.Fatalf("default model = %q, want metadata-priority frame-intonation-v8", models[0].ID)
	}
	for _, model := range models {
		if model.ID == "" || model.DisplayName == "" {
			t.Fatalf("incomplete bundled model: %#v", model)
		}
	}
}

func TestClassicRenderersDeclareAcceleration(t *testing.T) {
	rendererDirectories, _ := DefaultDirectories()
	items, err := DiscoverRenderers(rendererDirectories, func(string) bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"openutau-classic-worldline-faithful":     "cpu",
		"openutau-classic-worldline-faithful-gpu": "cuda",
	}
	for _, item := range items {
		if acceleration, ok := want[item.ID]; ok {
			if item.Acceleration != acceleration {
				t.Fatalf("renderer %q acceleration = %q, want %q", item.ID, item.Acceleration, acceleration)
			}
			delete(want, item.ID)
		}
	}
	if len(want) != 0 {
		t.Fatalf("missing classic renderers: %#v", want)
	}
}
