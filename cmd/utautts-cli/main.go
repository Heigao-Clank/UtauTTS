package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"

	"utautts/internal/audio"
	"utautts/internal/plugin"
	"utautts/internal/prosody"
	"utautts/internal/render"
	"utautts/internal/tts"
	"utautts/internal/voicebank"
)

func main() {
	var (
		voicebankPath           string
		otoPath                 string
		reading                 string
		text                    string
		tone                    string
		outPath                 string
		planPath                string
		moraMS                  float64
		pauseMS                 float64
		releaseMS               float64
		prosodyPath             string
		manualPitchPath         string
		prosodyFeaturesPath     string
		prosodyFeaturesCase     string
		prosodyPitchOnly        bool
		openJTalkPath           string
		openJTalkDictionaryPath string
		pitchContourPath        string
		pitchContourCase        string
		applyPitch              bool
		intonationStrength      float64
		renderer                string
		worldlinePath           string
		worldlineBridgePath     string
		worldlineR2MelPath      string
		worldlineR2VocoderPath  string
		onnxDeviceID            int
		utauResamplerPath       string
		boundaryBridgeMS        float64
		boundaryBridgeThreshold float64
		selectionMode           string
		joinModelPath           string
		joinScoreScale          float64
		rendererDirectories     []string
		modelDirectories        []string
	)
	flag.StringVar(&voicebankPath, "voicebank", "", "path to a UTAU voicebank directory")
	flag.StringVar(&otoPath, "oto", "", "deprecated alias for --voicebank")
	flag.StringVar(&reading, "kana", "", "kana reading to synthesize")
	flag.StringVar(&text, "text", "", "Japanese text to synthesize")
	flag.StringVar(&tone, "tone", "C4", "voicebank tone used with prefix.map")
	flag.StringVar(&outPath, "out", "", "output WAV path")
	flag.StringVar(&planPath, "plan-out", "", "optional synthesis plan JSON path")
	flag.Float64Var(&moraMS, "mora-ms", 140, "base mora duration in milliseconds")
	flag.Float64Var(&pauseMS, "pause-ms", 180, "punctuation pause in milliseconds")
	flag.Float64Var(&releaseMS, "release-ms", 20, "unit release envelope in milliseconds")
	flag.StringVar(&prosodyPath, "prosody", "", "optional learned prosody model JSON")
	flag.StringVar(&manualPitchPath, "manual-pitch", "", "optional mora pitch edit JSON")
	flag.StringVar(&prosodyFeaturesPath, "prosody-features", "", "optional per-case mora-level accent feature JSON")
	flag.StringVar(&prosodyFeaturesCase, "prosody-feature-case", "", "case ID in --prosody-features")
	flag.BoolVar(&prosodyPitchOnly, "prosody-pitch-only", false, "apply only learned pitch and keep fixed duration/energy")
	flag.StringVar(&openJTalkPath, "openjtalk-features", "", "path to the Open JTalk feature helper (default: runtime directory)")
	flag.StringVar(&openJTalkDictionaryPath, "openjtalk-dictionary", "", "path to the Open JTalk dictionary (default: runtime directory)")
	flag.StringVar(&pitchContourPath, "pitch-contours", "", "optional per-case pitch contour JSON (recorded in the plan; use --apply-pitch for direct waveform processing)")
	flag.StringVar(&pitchContourCase, "pitch-case", "", "case ID in --pitch-contours")
	flag.BoolVar(&applyPitch, "apply-pitch", false, "experimental waveform pitch resampling")
	flag.Float64Var(&intonationStrength, "intonation-strength", 0, "experimental source-pitch stabilization and phrase contour strength (0..1)")
	flag.StringVar(&renderer, "renderer", "", "renderer plugin ID (default: highest manifest priority)")
	flag.StringVar(&worldlinePath, "worldline", "", "path to OpenUtau worldline library (default: next to executable)")
	flag.StringVar(&worldlineBridgePath, "worldline-bridge", "", "path to utautts-worldline-bridge executable")
	flag.StringVar(&worldlineR2MelPath, "worldline-r2-mel", "", "path to OpenUtau WORLDLINE-R2 mel.onnx")
	flag.StringVar(&worldlineR2VocoderPath, "worldline-r2-vocoder", "", "path to the external PC-NSF-HiFiGAN ONNX model")
	flag.IntVar(&onnxDeviceID, "onnx-device", 0, "DirectML GPU device ID")
	flag.StringVar(&utauResamplerPath, "utau-resampler", "", "path to UTAU-compatible resampler.exe")
	flag.Float64Var(&boundaryBridgeMS, "boundary-bridge-ms", 0, "maximum width for phase-aligned waveform boundary repair candidates (0 disables)")
	flag.Float64Var(&boundaryBridgeThreshold, "boundary-bridge-threshold", 0, "apply boundary repair when handcrafted join score is at or below this value")
	flag.StringVar(&selectionMode, "selection", string(voicebank.SelectionViterbi), "unit selection: viterbi, greedy, or target-only")
	flag.StringVar(&joinModelPath, "join-model", "", "optional learned join-cost model JSON")
	flag.Float64Var(&joinScoreScale, "join-scale", 0, "learned logit score scale (default: model or 4)")
	flag.Func("renderer-dir", "renderer plugin directory (repeatable)", func(value string) error { rendererDirectories = append(rendererDirectories, value); return nil })
	flag.Func("model-dir", "prosody model directory (repeatable)", func(value string) error { modelDirectories = append(modelDirectories, value); return nil })
	flag.Parse()
	defaultRendererDirs, defaultModelDirs := plugin.DefaultDirectories()
	rendererDirectories = append(rendererDirectories, defaultRendererDirs...)
	modelDirectories = append(modelDirectories, defaultModelDirs...)
	catalog, catalogErr := plugin.Discover(rendererDirectories, modelDirectories, render.IsKnownRenderer)
	if catalogErr != nil {
		log.Printf("plugin discovery warning: %v", catalogErr)
	}
	if renderer == "" {
		renderer = catalog.DefaultRenderer()
	}
	rendererPlugin, found := catalog.Renderer(renderer)
	if !found {
		log.Fatalf("renderer plugin %q is not installed", renderer)
	}
	if prosodyPath != "" {
		if _, err := os.Stat(prosodyPath); err != nil {
			if model, found := catalog.Model(prosodyPath); found {
				prosodyPath = model.Path
			}
		}
	}
	worldlinePath = preferExplicit(worldlinePath, rendererPlugin.Asset("worldline"))
	worldlineBridgePath = preferExplicit(worldlineBridgePath, rendererPlugin.Asset("worldline_bridge"))
	worldlineR2MelPath = preferExplicit(worldlineR2MelPath, rendererPlugin.Asset("worldline_r2_mel"))
	worldlineR2VocoderPath = preferExplicit(worldlineR2VocoderPath, rendererPlugin.Asset("worldline_r2_vocoder"))

	if voicebankPath == "" {
		voicebankPath = otoPath
	}
	if voicebankPath == "" || (reading == "" && text == "") || outPath == "" {
		flag.Usage()
		log.Fatal("--voicebank, --out, and either --text or --kana are required")
	}

	pitchFactors, err := loadPitchFactors(pitchContourPath, pitchContourCase)
	if err != nil {
		log.Fatal(err)
	}
	prosodyFeatures, err := loadProsodyFeatures(prosodyFeaturesPath, prosodyFeaturesCase, text, reading)
	if err != nil {
		log.Fatal(err)
	}
	result, err := tts.Synthesize(tts.Config{
		VoicebankPath:           voicebankPath,
		Text:                    text,
		Reading:                 reading,
		Tone:                    tone,
		MoraDurationMS:          moraMS,
		PauseDurationMS:         pauseMS,
		ReleaseMS:               releaseMS,
		ProsodyModelPath:        prosodyPath,
		ManualPitchPath:         manualPitchPath,
		ProsodyFeatures:         prosodyFeatures,
		ProsodyPitchOnly:        prosodyPitchOnly,
		OpenJTalkPath:           openJTalkPath,
		OpenJTalkDictionaryPath: openJTalkDictionaryPath,
		PitchFactors:            pitchFactors,
		ApplyPitch:              applyPitch,
		IntonationStrength:      intonationStrength,
		Renderer:                rendererPlugin.Backend,
		RendererCapabilities:    &rendererPlugin.Capabilities,
		WorldlinePath:           worldlinePath,
		WorldlineBridgePath:     worldlineBridgePath,
		WorldlineR2MelPath:      worldlineR2MelPath,
		WorldlineR2VocoderPath:  worldlineR2VocoderPath,
		OnnxDeviceID:            onnxDeviceID,
		UTAUResamplerPath:       utauResamplerPath,
		BoundaryBridgeMS:        boundaryBridgeMS,
		BoundaryBridgeThreshold: boundaryBridgeThreshold,
		SelectionMode:           voicebank.SelectionMode(selectionMode),
		JoinModelPath:           joinModelPath,
		JoinScoreScale:          joinScoreScale,
	})
	if err != nil {
		log.Fatal(err)
	}
	if err := audio.WriteWav(outPath, result.Audio); err != nil {
		log.Fatal(err)
	}
	if planPath != "" {
		data, err := json.MarshalIndent(result.Plan, "", "  ")
		if err != nil {
			log.Fatal(err)
		}
		data = append(data, '\n')
		if err := os.WriteFile(planPath, data, 0o644); err != nil {
			log.Fatal(err)
		}
	}

	duration := float64(len(result.Audio.Data)) / float64(result.Audio.SampleRate)
	fmt.Printf("wrote %s (%.2fs, %d Hz, %d units)\n", outPath, duration, result.Audio.SampleRate, len(result.Plan.Units))
}

func preferExplicit(explicit, pluginValue string) string {
	if explicit != "" {
		return explicit
	}
	return pluginValue
}

func loadProsodyFeatures(path, caseID, text, reading string) ([]prosody.FeatureFrame, error) {
	if path == "" {
		return nil, nil
	}
	corpus, err := prosody.LoadFeatureCorpus(path)
	if err != nil {
		return nil, err
	}
	item, err := corpus.Select(caseID, text, reading)
	if err != nil {
		return nil, fmt.Errorf("select prosody features from %s: %w", path, err)
	}
	return item.Features, nil
}

func loadPitchFactors(path, caseID string) ([]float64, error) {
	if path == "" {
		return nil, nil
	}
	if caseID == "" {
		return nil, fmt.Errorf("--pitch-case is required with --pitch-contours")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var corpus struct {
		Cases []struct {
			ID           string    `json:"id"`
			PitchFactors []float64 `json:"pitch_factors"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(data, &corpus); err != nil {
		return nil, err
	}
	for _, item := range corpus.Cases {
		if item.ID == caseID {
			return item.PitchFactors, nil
		}
	}
	return nil, fmt.Errorf("pitch contour case %q not found in %s", caseID, path)
}
