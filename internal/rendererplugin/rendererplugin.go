package rendererplugin

import (
	"fmt"

	"utautts/internal/plugin"
	"utautts/internal/render"
	"utautts/internal/tts"
)

func Discover(extraDirectories []string) ([]plugin.Renderer, error) {
	defaults, _ := plugin.DefaultDirectories()
	return plugin.DiscoverRenderers(append(extraDirectories, defaults...), render.IsKnownRenderer)
}

func Resolve(renderers []plugin.Renderer, id string) (plugin.Renderer, error) {
	if id == "" && len(renderers) > 0 {
		return renderers[0], nil
	}
	for _, renderer := range renderers {
		if renderer.ID == id {
			return renderer, nil
		}
	}
	return plugin.Renderer{}, fmt.Errorf("renderer plugin %q is not installed", id)
}

func Apply(renderer plugin.Renderer, config *tts.Config) {
	config.Renderer = renderer.Backend
	config.RendererCapabilities = &renderer.Capabilities
	config.WorldlinePath = preferExplicit(config.WorldlinePath, renderer.Asset("worldline"))
	config.WorldlineBridgePath = preferExplicit(config.WorldlineBridgePath, renderer.Asset("worldline_bridge"))
}

func preferExplicit(explicit, manifestValue string) string {
	if explicit != "" {
		return explicit
	}
	return manifestValue
}
