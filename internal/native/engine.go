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
	"utautts/internal/aviutl"
	"utautts/internal/frontend"
	"utautts/internal/openjtalk"
	"utautts/internal/plugin"
	"utautts/internal/prosody"
	"utautts/internal/render"
	"utautts/internal/synth"
	"utautts/internal/tts"
	"utautts/internal/voicebank"
)

type Config struct {
	VoiceDir            string   `json:"voice_dir"`
	Renderer            string   `json:"renderer"`
	WorldlinePath       string   `json:"worldline_path"`
	WorldlineBridgePath string   `json:"worldline_bridge_path"`
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
	synth      *synth.Service
}

func New(config Config) (*Engine, error) {
	config.VoiceDir = voicebank.ResolveDirectory(config.VoiceDir)
	catalog, err := plugin.DiscoverWithDefaults(config.RendererDirectories, config.ModelDirectories, render.IsKnownRenderer)
	if err != nil {
		return nil, fmt.Errorf("discover plugins: %w", err)
	}
	if config.Renderer == "" {
		config.Renderer = catalog.DefaultRenderer()
	}
	engine := &Engine{config: config, voicebanks: make(map[string]voicebank.Summary), catalog: catalog}
	engine.synth = synth.NewService(catalog, config.Renderer, config.WorldlinePath, config.WorldlineBridgePath, config.OpenJTalkPath, config.OpenJTalkDictionary, nativeVoicebankResolver{engine: engine})
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
	case "predictProsody":
		result, err = e.predictProsody(requestJSON)
	case "synthesize":
		result, err = e.synthesize(requestJSON)
	case "writeExo":
		result, err = e.writeExo(requestJSON)
	default:
		err = fmt.Errorf("unknown native method %q", method)
	}
	if err != nil {
		return nil, err
	}
	return json.Marshal(result)
}

type nativeVoicebankResolver struct {
	engine *Engine
}

func (r nativeVoicebankResolver) Resolve(id string) (string, bool) {
	r.engine.mu.RLock()
	defer r.engine.mu.RUnlock()
	if id != "" {
		summary, ok := r.engine.voicebanks[id]
		return summary.Path, ok
	}
	first := voicebank.DefaultSortedKey(r.engine.voicebanks)
	summary, ok := r.engine.voicebanks[first]
	return summary.Path, ok
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
		entry := map[string]any{
			"id":          id,
			"name":        item.Name,
			"path":        item.Path,
			"image_path":  item.ImagePath,
			"readme_path": item.ReadmePath,
			"readme_text": presentation.ReadmeText,
		}
		if bank, err := voicebank.Load(item.Path); err == nil {
			capabilities := bank.AliasCapabilities()
			entry["alias_counts"] = capabilities.Counts
			entry["vcv_contexts"] = capabilities.VCVContexts
			entry["vc_contexts"] = capabilities.VCContexts
			entry["has_vc"] = capabilities.HasVC
			entry["has_initial_vcv"] = capabilities.HasInitialVCV
			entry["has_n_context_vcv"] = capabilities.HasNContextVCV
		}
		list = append(list, entry)
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
		items = append(items, map[string]any{"position": index, "mora": mora.Text, "consonant": mora.Consonant, "vowel": mora.Vowel, "pause": mora.Pause})
	}
	return map[string]any{"reading": reading, "morae": items}, nil
}

type prosodyPreviewRequest struct {
	RequestID string `json:"request_id"`
	Text      string `json:"text"`
	Kana      string `json:"kana"`
	ModelID   string `json:"model_id"`
	Renderer  string `json:"renderer"`

	MoraDurationMS     float64           `json:"mora_duration_ms"`
	PauseDurationMS    float64           `json:"pause_duration_ms"`
	MoraDurationsMS    []float64         `json:"mora_durations_ms"`
	IntonationStrength float64           `json:"intonation_strength"`
	ApplyPitch         bool              `json:"apply_pitch"`
	Dictionary         []dictionaryEntry `json:"dictionary"`
}

func (e *Engine) predictProsody(data []byte) (any, error) {
	var request prosodyPreviewRequest
	if err := json.Unmarshal(data, &request); err != nil {
		return nil, fmt.Errorf("decode prosody preview request: %w", err)
	}
	if request.Text == "" && request.Kana == "" {
		return nil, fmt.Errorf("text or kana is required")
	}
	preview, _, err := e.synth.PredictProsody(synth.Request{
		Text: request.Text, Kana: request.Kana, Dictionary: dictionaryMap(request.Dictionary),
		ModelID: request.ModelID, Renderer: request.Renderer,
		MoraDurationMS: request.MoraDurationMS, PauseDurationMS: request.PauseDurationMS,
		MoraDurationsMS: request.MoraDurationsMS, IntonationStrength: request.IntonationStrength,
		ApplyPitch: request.ApplyPitch,
	})
	if err != nil {
		return nil, err
	}
	morae := make([]map[string]any, len(preview.Morae))
	for index, mora := range preview.Morae {
		morae[index] = map[string]any{"position": index, "mora": mora.Text, "pause": mora.Pause}
	}
	return map[string]any{
		"request_id":            request.RequestID,
		"reading":               preview.Reading,
		"morae":                 morae,
		"mora_durations_ms":     preview.MoraDurationsMS,
		"mora_positions_ms":     preview.MoraPositionsMS,
		"pitch_points":          preview.PitchPoints,
		"prosody_model_applied": e.synth.ModelAvailable(request.ModelID),
	}, nil
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
	return tts.ConvertToReading(text, dictionary, openjtalk.Config{
		HelperPath:     e.config.OpenJTalkPath,
		DictionaryPath: e.config.OpenJTalkDictionary,
	})
}

type synthesizeRequest struct {
	Text, Kana, VoicebankID, Tone, ModelID, Renderer, OutputPath string
	AliasPolicy                                                  voicebank.AliasPolicy
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
		AliasPolicy        voicebank.AliasPolicy    `json:"alias_policy"`
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
	*r = synthesizeRequest{Text: value.Text, Kana: value.Kana, VoicebankID: value.VoicebankID, Tone: value.Tone, ModelID: value.ModelID, Renderer: value.Renderer, AliasPolicy: value.AliasPolicy, OutputPath: value.OutputPath, MoraDurationMS: value.MoraDurationMS, PauseDurationMS: value.PauseDurationMS, MoraDurationsMS: value.MoraDurationsMS, IntonationStrength: value.IntonationStrength, ApplyPitch: value.ApplyPitch, ManualPitch: value.ManualPitch, Dictionary: value.Dictionary}
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
	result, rendererID, err := e.synth.Synthesize(synth.Request{
		Text: request.Text, Kana: request.Kana, VoicebankID: request.VoicebankID,
		Tone: request.Tone, ModelID: request.ModelID, Renderer: request.Renderer, AliasPolicy: request.AliasPolicy,
		Dictionary:     dictionaryMap(request.Dictionary),
		MoraDurationMS: request.MoraDurationMS, PauseDurationMS: request.PauseDurationMS,
		MoraDurationsMS: request.MoraDurationsMS, IntonationStrength: request.IntonationStrength,
		ApplyPitch: request.ApplyPitch, ManualPitch: request.ManualPitch,
	})
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
	return map[string]any{
		"output_path":           outputPath,
		"reading":               result.Plan.Reading,
		"duration_ms":           float64(len(result.Audio.Data)) * 1000 / float64(result.Audio.SampleRate),
		"unit_count":            len(result.Plan.Units),
		"engine":                rendererID,
		"mora_durations_ms":     result.MoraDurationsMS,
		"mora_positions_ms":     result.MoraPositionsMS,
		"pitch_points":          result.PitchPoints,
		"prosody_model_applied": e.synth.ModelAvailable(request.ModelID),
	}, nil
}

func (e *Engine) writeExo(data []byte) (any, error) {
	var request struct {
		OutputPath string   `json:"output_path"`
		Files      []string `json:"files"`
		FrameRate  int      `json:"frame_rate"`
	}
	if err := json.Unmarshal(data, &request); err != nil {
		return nil, fmt.Errorf("decode write exo request: %w", err)
	}
	if request.OutputPath == "" {
		return nil, fmt.Errorf("output_path is required")
	}
	if len(request.Files) == 0 {
		return nil, fmt.Errorf("files are required")
	}
	if request.FrameRate <= 0 {
		request.FrameRate = 60
	}
	for _, file := range request.Files {
		info, err := os.Stat(file)
		if err != nil || info.IsDir() {
			return nil, fmt.Errorf("WAV file not found: %s", file)
		}
	}
	outputPath, err := filepath.Abs(request.OutputPath)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return nil, err
	}
	if err := aviutl.WriteExo(outputPath, request.Files, request.FrameRate); err != nil {
		return nil, err
	}
	return map[string]any{"exo_path": outputPath}, nil
}
