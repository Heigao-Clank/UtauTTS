package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"utautts/internal/audio"
	"utautts/internal/evaluation"
	"utautts/internal/frontend"
	"utautts/internal/openutau"
	"utautts/internal/plan"
	"utautts/internal/render"
	"utautts/internal/rendererplugin"
	"utautts/internal/tts"
)

type fileIdentity struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

type renderedCase struct {
	ID          string      `json:"id"`
	Description string      `json:"description"`
	Cents       []float64   `json:"cents"`
	FrameMS     float64     `json:"frame_ms"`
	File        string      `json:"file"`
	Units       []plan.Unit `json:"units"`
}

type sweepManifest struct {
	Version           int                    `json:"version"`
	Text              string                 `json:"text"`
	Reading           string                 `json:"reading"`
	Morae             []string               `json:"morae"`
	Voicebank         string                 `json:"voicebank"`
	Tone              string                 `json:"tone"`
	TargetRenderer    string                 `json:"target_renderer"`
	ReferenceRenderer string                 `json:"reference_renderer"`
	Mixing            string                 `json:"mixing"`
	UTAUResampler     *fileIdentity          `json:"utau_resampler,omitempty"`
	OpenUtauProject   *openutau.ProjectAudit `json:"openutau_project,omitempty"`
	Warnings          []string               `json:"warnings,omitempty"`
	FlatExact         bool                   `json:"flat_exact"`
	ReferenceFile     string                 `json:"reference_file"`
	ReferenceUnits    []plan.Unit            `json:"reference_units"`
	Cases             []renderedCase         `json:"cases"`
}

func main() {
	var cfg tts.Config
	var outputDirectory, openUtauProject, rendererID, referenceRendererID string
	var rendererDirectories []string
	flag.StringVar(&cfg.VoicebankPath, "voicebank", "", "path to a UTAU voicebank directory")
	flag.StringVar(&cfg.Text, "text", "あいうえお", "Japanese test text")
	flag.StringVar(&cfg.Reading, "reading", "", "optional kana reading")
	flag.StringVar(&cfg.Tone, "tone", "C4", "voicebank tone")
	flag.Float64Var(&cfg.MoraDurationMS, "mora-ms", 240, "mora duration for renderer diagnostics")
	flag.Float64Var(&cfg.PauseDurationMS, "pause-ms", 180, "pause duration")
	flag.Float64Var(&cfg.ReleaseMS, "release-ms", 20, "release envelope")
	flag.StringVar(&rendererID, "renderer", "", "renderer plugin ID under test (required)")
	flag.StringVar(&referenceRendererID, "reference-renderer", "", "reference renderer plugin ID (default: highest manifest priority)")
	flag.Func("renderer-dir", "renderer plugin directory (repeatable)", func(value string) error { rendererDirectories = append(rendererDirectories, value); return nil })
	flag.StringVar(&cfg.WorldlinePath, "worldline", "", "path to worldline library")
	flag.StringVar(&cfg.WorldlineBridgePath, "worldline-bridge", "", "path to worldline bridge")
	flag.StringVar(&cfg.UTAUResamplerPath, "utau-resampler", "", "path to UTAU-compatible resampler")
	flag.StringVar(&openUtauProject, "openutau-project", "", "optional .ustx provenance source")
	flag.StringVar(&outputDirectory, "out", "", "output directory")
	flag.Parse()
	if cfg.VoicebankPath == "" || outputDirectory == "" || rendererID == "" {
		flag.Usage()
		log.Fatal("--voicebank, --renderer and --out are required")
	}
	renderers, discoveryErr := rendererplugin.Discover(rendererDirectories)
	if discoveryErr != nil {
		log.Printf("renderer plugin discovery warning: %v", discoveryErr)
	}
	targetRenderer, err := rendererplugin.Resolve(renderers, rendererID)
	if err != nil {
		log.Fatal(err)
	}
	referenceRenderer, err := rendererplugin.Resolve(renderers, referenceRendererID)
	if err != nil {
		log.Fatal(err)
	}
	rendererplugin.Apply(targetRenderer, &cfg)
	if err := os.MkdirAll(outputDirectory, 0o755); err != nil {
		log.Fatal(err)
	}

	reading := cfg.Reading
	if reading == "" {
		var err error
		reading, err = frontend.ToKana(cfg.Text)
		if err != nil {
			log.Fatal(err)
		}
	}
	morae, err := frontend.ParseKana(reading)
	if err != nil {
		log.Fatal(err)
	}
	voiced := 0
	moraNames := make([]string, len(morae))
	for index, mora := range morae {
		moraNames[index] = mora.Text
		if mora.Pause {
			moraNames[index] = "<pause>"
		} else {
			voiced++
		}
	}
	if voiced < 1 {
		log.Fatal("renderer sweep needs at least one non-pause mora")
	}

	manifest := sweepManifest{
		Version: 1, Text: cfg.Text, Reading: reading, Morae: moraNames,
		Voicebank: cfg.VoicebankPath, Tone: cfg.Tone,
		TargetRenderer: targetRenderer.ID, ReferenceRenderer: referenceRenderer.ID,
		Mixing: "UtauTTS internal overlap/envelope mixer",
	}
	if cfg.UTAUResamplerPath != "" {
		identity, identityErr := identifyFile(cfg.UTAUResamplerPath)
		if identityErr != nil {
			log.Fatal(identityErr)
		}
		manifest.UTAUResampler = identity
	}
	if openUtauProject != "" {
		manifest.OpenUtauProject, err = openutau.InspectProject(openUtauProject)
		if err != nil {
			log.Fatal(err)
		}
		manifest.Warnings = append(manifest.Warnings, provenanceWarnings(cfg, manifest.OpenUtauProject)...)
	}

	referenceCfg := cfg
	rendererplugin.Apply(referenceRenderer, &referenceCfg)
	referenceCfg.PitchFactors = nil
	referenceCfg.ApplyPitch = false
	reference, err := tts.Synthesize(referenceCfg)
	if err != nil {
		log.Fatalf("render waveform reference: %v", err)
	}
	manifest.ReferenceFile = "reference-waveform.wav"
	manifest.ReferenceUnits = reference.Plan.Units
	if err := audio.WriteWav(filepath.Join(outputDirectory, manifest.ReferenceFile), reference.Audio); err != nil {
		log.Fatal(err)
	}

	const openUtauPitchFrameMS = 60000.0 / 120.0 * 5.0 / 480.0
	for _, contour := range evaluation.DeterministicFramePitchSweep(reference.Plan.DurationMS+cfg.ReleaseMS, openUtauPitchFrameMS) {
		caseCfg := cfg
		caseCfg.PitchFactors = nil
		caseCfg.PitchCurve = &render.PitchCurve{FrameMS: contour.FrameMS, Cents: contour.Cents}
		result, synthErr := tts.Synthesize(caseCfg)
		if synthErr != nil {
			log.Fatalf("render %s: %v", contour.ID, synthErr)
		}
		if contour.ID == "flat" {
			manifest.FlatExact = equalPCM(reference.Audio, result.Audio)
		}
		filename := contour.ID + ".wav"
		if err := audio.WriteWav(filepath.Join(outputDirectory, filename), result.Audio); err != nil {
			log.Fatal(err)
		}
		manifest.Cases = append(manifest.Cases, renderedCase{
			ID: contour.ID, Description: contour.Description,
			Cents: contour.Cents, FrameMS: contour.FrameMS,
			File: filename, Units: result.Plan.Units,
		})
	}
	if err := writeJSON(filepath.Join(outputDirectory, "manifest.json"), manifest); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("wrote waveform reference and %d renderer cases to %s\n", len(manifest.Cases), outputDirectory)
}

func equalPCM(left, right *audio.PCM) bool {
	if left == nil || right == nil || left.SampleRate != right.SampleRate || left.Channels != right.Channels || len(left.Data) != len(right.Data) {
		return false
	}
	for index := range left.Data {
		if left.Data[index] != right.Data[index] {
			return false
		}
	}
	return true
}

func identifyFile(path string) (*fileIdentity, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read renderer executable: %w", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	sum := sha256.Sum256(data)
	return &fileIdentity{Path: abs, Size: info.Size(), SHA256: hex.EncodeToString(sum[:])}, nil
}

func provenanceWarnings(cfg tts.Config, project *openutau.ProjectAudit) []string {
	if project == nil || len(project.Tracks) == 0 {
		return nil
	}
	track := project.Tracks[0]
	var warnings []string
	classicRenderer := cfg.Renderer == "utau-classic" || strings.HasPrefix(cfg.Renderer, "openutau-classic-worldline-faithful")
	if classicRenderer && track.Renderer != "" && !strings.EqualFold(track.Renderer, "CLASSIC") {
		warnings = append(warnings, fmt.Sprintf("target renderer utau-classic does not match OpenUtau renderer %s", track.Renderer))
	}
	if track.Resampler != "" {
		configured := ""
		if strings.HasPrefix(cfg.Renderer, "openutau-classic-worldline-faithful") {
			configured = "worldline"
		} else if cfg.UTAUResamplerPath != "" {
			configured = strings.TrimSuffix(filepath.Base(cfg.UTAUResamplerPath), filepath.Ext(cfg.UTAUResamplerPath))
		}
		if configured == "" || !strings.EqualFold(configured, track.Resampler) {
			warnings = append(warnings, fmt.Sprintf("configured resampler %q does not match OpenUtau resampler %q", configured, track.Resampler))
		}
	}
	if track.Wavtool != "" {
		warnings = append(warnings, fmt.Sprintf("OpenUtau wavtool %q is recorded; compare it with the selected renderer manifest", track.Wavtool))
	}
	return warnings
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}
