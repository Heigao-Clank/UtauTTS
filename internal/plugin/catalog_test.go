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
		t.Fatalf("legacy model was catalogued from its filename: %#v", models)
	}
}
