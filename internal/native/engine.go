package native

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"utautts/internal/appinfo"
	"utautts/internal/audio"
	"utautts/internal/frontend"
	"utautts/internal/openjtalk"
	"utautts/internal/plugin"
	"utautts/internal/prosody"
	"utautts/internal/render"
	"utautts/internal/tts"
	"utautts/internal/voicebank"
)

type Config struct {
	VoiceDir            string   `json:"voice_dir"`
	Renderer            string   `json:"renderer"`
	WorldlinePath       string   `json:"worldline_path"`
	WorldlineBridgePath string   `json:"worldline_bridge_path"`
	WorldlineR2MelPath  string   `json:"worldline_r2_mel_path"`
	WorldlineR2Vocoder  string   `json:"worldline_r2_vocoder_path"`
	OnnxDeviceID        int      `json:"onnx_device_id"`
	OpenJTalkPath       string   `json:"openjtalk_path"`
	OpenJTalkDictionary string   `json:"openjtalk_dictionary"`
	RendererDirectories []string `json:"renderer_directories,omitempty"`
	ModelDirectories    []string `json:"model_directories,omitempty"`
}

type Engine struct {
	config     Config
	mu         sync.RWMutex
	voicebanks map[string]voicebank.Summary
	catalog    *plugin.Catalog
}

func New(config Config) (*Engine, error) {
	config.VoiceDir = voicebank.ResolveDirectory(config.VoiceDir)
	defaultRendererDirs, defaultModelDirs := plugin.DefaultDirectories()
	if len(config.RendererDirectories) == 0 {
		config.RendererDirectories = defaultRendererDirs
	}
	if len(config.ModelDirectories) == 0 {
		config.ModelDirectories = defaultModelDirs
	}
	catalog, err := plugin.Discover(config.RendererDirectories, config.ModelDirectories, render.IsKnownRenderer)
	if err != nil {
		return nil, fmt.Errorf("discover plugins: %w", err)
	}
	if config.Renderer == "" {
		config.Renderer = catalog.DefaultRenderer()
	}
	engine := &Engine{config: config, voicebanks: make(map[string]voicebank.Summary), catalog: catalog}
	if err := engine.reload(); err != nil {
		return nil, fmt.Errorf("load voicebanks: %w", err)
	}
	return engine, nil
}

func NewJSON(data []byte) (*Engine, error) {
	var config Config
	if len(data) != 0 {
		if err := json.Unmarshal(data, &config); err != nil {
			return nil, fmt.Errorf("decode native config: %w", err)
		}
	}
	return New(config)
}

func (e *Engine) Call(method string, requestJSON []byte) ([]byte, error) {
	var result any
	var err error
	switch method {
	case "health":
		result = map[string]any{"status": "ok", "engine": e.config.Renderer, "version": appinfo.Version()}
	case "voicebanks":
		result = map[string]any{"voicebanks": e.voicebankList()}
	case "reloadVoicebanks":
		err = e.reload()
		result = map[string]any{"voicebanks": e.voicebankList()}
	case "models":
		result = map[string]any{"models": e.models()}
	case "renderers":
		result = map[string]any{"default_renderer": e.config.Renderer, "renderers": e.catalog.Renderers}
	case "hardware":
		result = map[string]any{"cuda_available": render.CUDAAvailable()}
	case "analyze":
		result, err = e.analyze(requestJSON)
	case "synthesize":
		result, err = e.synthesize(requestJSON)
	default:
		err = fmt.Errorf("unknown native method %q", method)
	}
	if err != nil {
		return nil, err
	}
	return json.Marshal(result)
}

type Voicebank struct {
	ID, Name, Path, ImagePath string
}

func (e *Engine) reload() error {
	summaries, err := voicebank.Discover(e.config.VoiceDir)
	if err != nil && !errors.Is(err, voicebank.ErrNoOto) && !os.IsNotExist(err) {
		return err
	}
	next := make(map[string]voicebank.Summary, len(summaries))
	for _, summary := range summaries {
		next[filepath.Base(summary.Path)] = summary
	}
	e.mu.Lock()
	e.voicebanks = next
	e.mu.Unlock()
	tts.ClearCaches()
	return nil
}

func (e *Engine) voicebankList() []map[string]any {
	e.mu.RLock()
	list := make([]map[string]any, 0, len(e.voicebanks))
	for id, item := range e.voicebanks {
		presentation, _ := voicebank.LoadPresentation(item)
		list = append(list, map[string]any{
			"id":          id,
			"name":        item.Name,
			"path":        item.Path,
			"image_path":  item.ImagePath,
			"readme_path": item.ReadmePath,
			"readme_text": presentation.ReadmeText,
		})
	}
	e.mu.RUnlock()
	sort.Slice(list, func(i, j int) bool { return list[i]["name"].(string) < list[j]["name"].(string) })
	return list
}

func (e *Engine) models() []plugin.Model {
	return append([]plugin.Model(nil), e.catalog.Models...)
}

func (e *Engine) analyze(data []byte) (any, error) {
	var request struct {
		Text       string            `json:"text"`
		Dictionary []dictionaryEntry `json:"dictionary"`
	}
	if err := json.Unmarshal(data, &request); err != nil || request.Text == "" {
		return nil, fmt.Errorf("text is required")
	}
	reading, err := e.reading(request.Text, dictionaryMap(request.Dictionary))
	if err != nil {
		return nil, err
	}
	morae, err := frontend.ParseKana(reading)
	if err != nil {
		return nil, err
	}
	items := make([]map[string]any, 0, len(morae))
	for index, mora := range morae {
		items = append(items, map[string]any{"position": index, "mora": mora.Text, "pause": mora.Pause})
	}
	return map[string]any{"reading": reading, "morae": items}, nil
}

type dictionaryEntry struct {
	Surface string `json:"surface"`
	Reading string `json:"reading"`
}

func dictionaryMap(entries []dictionaryEntry) map[string]string {
	result := make(map[string]string, len(entries))
	for _, entry := range entries {
		if entry.Surface == "" || entry.Reading == "" {
			continue
		}
		result[entry.Surface] = entry.Reading
	}
	return result
}

func (e *Engine) reading(text string, dictionary map[string]string) (string, error) {
	reading, frontendErr := frontend.ToKanaWithDictionary(text, dictionary)
	if frontendErr == nil {
		return reading, nil
	}
	analysis, openJTalkErr := openjtalk.Analyze(frontend.ApplyDictionary(text, dictionary), openjtalk.Config{
		HelperPath:     e.config.OpenJTalkPath,
		DictionaryPath: e.config.OpenJTalkDictionary,
	})
	if openJTalkErr != nil {
		return "", fmt.Errorf("convert text to reading: %v; Open JTalk fallback: %w", frontendErr, openJTalkErr)
	}
	return analysis.Reading, nil
}

type synthesizeRequest struct {
	Text, Kana, VoicebankID, Tone, ModelID, Renderer, OutputPath string
	MoraDurationMS, PauseDurationMS, IntonationStrength          float64
	MoraDurationsMS                                              []float64
	ApplyPitch                                                   bool
	ManualPitch                                                  *prosody.ManualPitchFile
	Dictionary                                                   []dictionaryEntry
}

func (r *synthesizeRequest) UnmarshalJSON(data []byte) error {
	type wire struct {
		Text               string                   `json:"text"`
		Kana               string                   `json:"kana"`
		VoicebankID        string                   `json:"voicebank_id"`
		Tone               string                   `json:"tone"`
		ModelID            string                   `json:"model_id"`
		Renderer           string                   `json:"renderer"`
		OutputPath         string                   `json:"output_path"`
		MoraDurationMS     float64                  `json:"mora_duration_ms"`
		PauseDurationMS    float64                  `json:"pause_duration_ms"`
		MoraDurationsMS    []float64                `json:"mora_durations_ms"`
		IntonationStrength float64                  `json:"intonation_strength"`
		ApplyPitch         bool                     `json:"apply_pitch"`
		ManualPitch        *prosody.ManualPitchFile `json:"manual_pitch"`
		Dictionary         []dictionaryEntry        `json:"dictionary"`
	}
	var value wire
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*r = synthesizeRequest{Text: value.Text, Kana: value.Kana, VoicebankID: value.VoicebankID, Tone: value.Tone, ModelID: value.ModelID, Renderer: value.Renderer, OutputPath: value.OutputPath, MoraDurationMS: value.MoraDurationMS, PauseDurationMS: value.PauseDurationMS, MoraDurationsMS: value.MoraDurationsMS, IntonationStrength: value.IntonationStrength, ApplyPitch: value.ApplyPitch, ManualPitch: value.ManualPitch, Dictionary: value.Dictionary}
	return nil
}

func (e *Engine) synthesize(data []byte) (any, error) {
	var request synthesizeRequest
	if err := json.Unmarshal(data, &request); err != nil {
		return nil, fmt.Errorf("decode synthesis request: %w", err)
	}
	if request.Text == "" && request.Kana == "" {
		return nil, fmt.Errorf("text or kana is required")
	}
	if request.OutputPath == "" {
		return nil, fmt.Errorf("output_path is required")
	}
	dictionary := dictionaryMap(request.Dictionary)
	e.mu.RLock()
	summary, ok := e.voicebanks[request.VoicebankID]
	if !ok && request.VoicebankID == "" {
		for _, item := range e.voicebanks {
			summary = item
			ok = true
			break
		}
	}
	e.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("voicebank not found")
	}
	modelPath := ""
	if request.ModelID != "" && request.ModelID != "none" {
		model, found := e.catalog.Model(request.ModelID)
		if !found {
			return nil, fmt.Errorf("prosody model not found")
		}
		modelPath = model.Path
	}
	rendererID := request.Renderer
	if rendererID == "" {
		rendererID = e.config.Renderer
	}
	rendererPlugin, pluginFound := e.catalog.Renderer(rendererID)
	if !pluginFound {
		return nil, fmt.Errorf("renderer plugin %q is not installed", rendererID)
	}
	if !render.IsKnownRenderer(rendererPlugin.Backend) {
		return nil, fmt.Errorf("renderer plugin %q requires unavailable backend %q", rendererID, rendererPlugin.Backend)
	}
	worldlinePath := firstConfigured(e.config.WorldlinePath, rendererPlugin.Asset("worldline"))
	worldlineBridgePath := firstConfigured(e.config.WorldlineBridgePath, rendererPlugin.Asset("worldline_bridge"))
	r2MelPath := firstConfigured(e.config.WorldlineR2MelPath, rendererPlugin.Asset("worldline_r2_mel"))
	r2VocoderPath := firstConfigured(e.config.WorldlineR2Vocoder, rendererPlugin.Asset("worldline_r2_vocoder"))
	reading := request.Kana
	if reading == "" && modelPath == "" {
		resolvedReading, err := e.reading(request.Text, dictionary)
		if err != nil {
			return nil, err
		}
		reading = resolvedReading
	}
	result, err := tts.Synthesize(tts.Config{VoicebankPath: summary.Path, Text: request.Text, Reading: reading, Dictionary: dictionary, Tone: request.Tone, MoraDurationMS: request.MoraDurationMS, PauseDurationMS: request.PauseDurationMS, MoraDurationsMS: request.MoraDurationsMS, ProsodyModelPath: modelPath, ManualPitch: request.ManualPitch, IntonationStrength: request.IntonationStrength, ApplyPitch: request.ApplyPitch, Renderer: rendererPlugin.Backend, RendererCapabilities: &rendererPlugin.Capabilities, WorldlinePath: worldlinePath, WorldlineBridgePath: worldlineBridgePath, WorldlineR2MelPath: r2MelPath, WorldlineR2VocoderPath: r2VocoderPath, OnnxDeviceID: e.config.OnnxDeviceID, OpenJTalkPath: e.config.OpenJTalkPath, OpenJTalkDictionaryPath: e.config.OpenJTalkDictionary})
	if err != nil {
		return nil, err
	}
	outputPath, err := filepath.Abs(request.OutputPath)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return nil, err
	}
	if err := audio.WriteWav(outputPath, result.Audio); err != nil {
		return nil, err
	}
	return map[string]any{"output_path": outputPath, "reading": result.Plan.Reading, "duration_ms": float64(len(result.Audio.Data)) * 1000 / float64(result.Audio.SampleRate), "unit_count": len(result.Plan.Units), "engine": rendererID}, nil
}

func firstConfigured(explicit, fromPlugin string) string {
	if explicit != "" {
		return explicit
	}
	return fromPlugin
}
