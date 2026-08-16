// Package synth is the transport-agnostic synthesis orchestrator shared by the
// Qt GUI backend and the HTTP server. It resolves a voicebank, prosody model,
// and renderer plugin, then dispatches to the tts package so callers only need
// to handle their own request decoding, validation, and response encoding.
package synth

import (
	"errors"
	"fmt"

	"utautts/internal/plugin"
	"utautts/internal/prosody"
	"utautts/internal/tts"
)

// ErrUnavailable marks a resolution failure (voicebank, prosody model, or
// renderer plugin not found) so HTTP callers can return a client error that is
// distinct from a synthesis failure.
var ErrUnavailable = errors.New("unavailable")

// Request is the shared, transport-agnostic synthesis and preview request.
type Request struct {
	Text               string
	Kana               string
	VoicebankID        string
	Tone               string
	ModelID            string
	Renderer           string
	Dictionary         map[string]string
	MoraDurationMS     float64
	PauseDurationMS    float64
	MoraDurationsMS    []float64
	IntonationStrength float64
	ApplyPitch         bool
	ManualPitch        *prosody.ManualPitchFile
}

// VoicebankResolver maps a voicebank ID to its root path. An empty ID selects
// the default voicebank.
type VoicebankResolver interface {
	Resolve(id string) (path string, ok bool)
}

// Service resolves request inputs against a plugin catalog and voicebank
// resolver, then dispatches to the tts package.
type Service struct {
	catalog              *plugin.Catalog
	renderer             string
	worldlinePath        string
	worldlineBridgePath  string
	openJTalkPath        string
	openJTalkDictionary  string
	voicebanks           VoicebankResolver
}

func NewService(catalog *plugin.Catalog, renderer, worldlinePath, worldlineBridgePath, openJTalkPath, openJTalkDictionary string, voicebanks VoicebankResolver) *Service {
	return &Service{
		catalog: catalog, renderer: renderer,
		worldlinePath: worldlinePath, worldlineBridgePath: worldlineBridgePath,
		openJTalkPath: openJTalkPath, openJTalkDictionary: openJTalkDictionary,
		voicebanks: voicebanks,
	}
}

// Synthesize resolves the request and renders audio, returning the renderer ID
// that was actually used.
func (s *Service) Synthesize(request Request) (*tts.Result, string, error) {
	cfg, rendererID, err := s.config(request, true)
	if err != nil {
		return nil, "", err
	}
	result, err := tts.Synthesize(cfg)
	if err != nil {
		return nil, "", err
	}
	return result, rendererID, nil
}

// PredictProsody resolves the request and previews prosody without synthesizing
// audio or loading a voicebank.
func (s *Service) PredictProsody(request Request) (*tts.ProsodyPreview, string, error) {
	cfg, rendererID, err := s.config(request, false)
	if err != nil {
		return nil, "", err
	}
	preview, err := tts.PredictProsody(cfg)
	if err != nil {
		return nil, "", err
	}
	return preview, rendererID, nil
}

func (s *Service) config(request Request, requireVoicebank bool) (tts.Config, string, error) {
	modelPath, err := s.modelPath(request.ModelID)
	if err != nil {
		return tts.Config{}, "", err
	}
	cfg := tts.Config{
		Text:                    request.Text,
		Reading:                 request.Kana,
		Dictionary:              request.Dictionary,
		Tone:                    request.Tone,
		MoraDurationMS:          request.MoraDurationMS,
		PauseDurationMS:         request.PauseDurationMS,
		MoraDurationsMS:         request.MoraDurationsMS,
		ProsodyModelPath:        modelPath,
		ManualPitch:             request.ManualPitch,
		IntonationStrength:      request.IntonationStrength,
		ApplyPitch:              request.ApplyPitch,
		OpenJTalkPath:           s.openJTalkPath,
		OpenJTalkDictionaryPath: s.openJTalkDictionary,
	}
	if requireVoicebank {
		if s.voicebanks == nil {
			return tts.Config{}, "", fmt.Errorf("%w: voicebank resolver is not configured", ErrUnavailable)
		}
		voicebankPath, ok := s.voicebanks.Resolve(request.VoicebankID)
		if !ok {
			return tts.Config{}, "", fmt.Errorf("%w: voicebank not found", ErrUnavailable)
		}
		cfg.VoicebankPath = voicebankPath
	}
	rendererID := s.rendererID(request.Renderer)
	if err := tts.ApplyRenderer(&cfg, s.catalog, rendererID, s.worldlinePath, s.worldlineBridgePath); err != nil {
		return tts.Config{}, "", fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	return cfg, rendererID, nil
}

func (s *Service) rendererID(requested string) string {
	if requested != "" {
		return requested
	}
	return s.renderer
}

// modelPath resolves a prosody model ID to its file path. An empty or "none"
// ID selects no model.
func (s *Service) modelPath(id string) (string, error) {
	if id == "" || id == "none" {
		return "", nil
	}
	model, found := s.catalog.Model(id)
	if !found {
		return "", fmt.Errorf("%w: prosody model %q not found", ErrUnavailable, id)
	}
	return model.Path, nil
}

// ModelAvailable reports whether the request selects a prosody model, matching
// the "prosody_model_applied" field the callers return.
func (s *Service) ModelAvailable(id string) bool {
	if id == "" || id == "none" {
		return false
	}
	_, found := s.catalog.Model(id)
	return found
}
