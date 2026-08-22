package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"log"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"strings"

	"utautts/internal/audio"
	"utautts/internal/evaluation"
	"utautts/internal/plan"
	"utautts/internal/prosody"
	"utautts/internal/render"
	"utautts/internal/rendererplugin"
	"utautts/internal/tts"
	"utautts/internal/voicebank"
)

type textList []string

func (values *textList) String() string         { return strings.Join(*values, " | ") }
func (values *textList) Set(value string) error { *values = append(*values, value); return nil }

type publicTrial struct {
	ID     int    `json:"id"`
	CaseID string `json:"case_id,omitempty"`
	Text   string `json:"text"`
	A      string `json:"a"`
	B      string `json:"b"`
	X      string `json:"x,omitempty"`
}

type answerTrial struct {
	ID      int               `json:"id"`
	CaseID  string            `json:"case_id,omitempty"`
	Text    string            `json:"text"`
	A       systemInfo        `json:"a"`
	B       systemInfo        `json:"b"`
	XAnswer string            `json:"x_answer,omitempty"`
	Changes []selectionChange `json:"selection_changes,omitempty"`
}

type selectionChange struct {
	Position int        `json:"position"`
	Mora     string     `json:"mora"`
	A        unitChoice `json:"a"`
	B        unitChoice `json:"b"`
}

type unitChoice struct {
	Alias           string  `json:"alias"`
	Source          string  `json:"source"`
	OtoLine         int     `json:"oto_line"`
	TargetScore     float64 `json:"target_score"`
	JoinScore       float64 `json:"join_score"`
	JoinProbability float64 `json:"join_probability,omitempty"`
}

type systemInfo struct {
	Renderer                string  `json:"renderer"`
	AliasPolicy             string  `json:"alias_policy,omitempty"`
	JoinModel               bool    `json:"join_model"`
	JoinModelPath           string  `json:"join_model_path,omitempty"`
	ProsodyModel            bool    `json:"prosody_model,omitempty"`
	ProsodyPath             string  `json:"prosody_model_path,omitempty"`
	ProsodyFeaturesPath     string  `json:"prosody_features_path,omitempty"`
	ProsodyPitchOnly        bool    `json:"prosody_pitch_only,omitempty"`
	ProsodyDurationOnly     bool    `json:"prosody_duration_only,omitempty"`
	MoraDurationsPath       string  `json:"mora_durations_path,omitempty"`
	ApplyPitch              bool    `json:"apply_pitch,omitempty"`
	PitchContourPath        string  `json:"pitch_contour_path,omitempty"`
	FramePitchContourPath   string  `json:"frame_pitch_contour_path,omitempty"`
	IntonationStrength      float64 `json:"intonation_strength"`
	BoundaryBridgeMS        float64 `json:"boundary_bridge_ms,omitempty"`
	BoundaryBridgeThreshold float64 `json:"boundary_bridge_threshold,omitempty"`
	CVVCTiming              string  `json:"cvvc_timing,omitempty"`
	CVVCTransitionGain      float64 `json:"cvvc_transition_gain,omitempty"`
	CVVCPreBoundaryFade     bool    `json:"cvvc_pre_boundary_fade,omitempty"`
	LongUnitGroups          int     `json:"long_unit_groups"`
}

type pitchContourCorpus struct {
	Version int                `json:"version"`
	Cases   []pitchContourCase `json:"cases"`
}

type moraDurationCorpus struct {
	Version int                `json:"version"`
	Cases   []moraDurationCase `json:"cases"`
}

type moraDurationCase struct {
	ID              string    `json:"id"`
	MoraDurationsMS []float64 `json:"mora_durations_ms"`
	PauseDurationMS float64   `json:"pause_duration_ms,omitempty"`
}

type pitchContourCase struct {
	ID           string    `json:"id"`
	PitchFactors []float64 `json:"pitch_factors"`
}

type framePitchContourCorpus struct {
	Version int                     `json:"version"`
	Cases   []framePitchContourCase `json:"cases"`
}

type framePitchContourCase struct {
	ID      string    `json:"id"`
	FrameMS float64   `json:"frame_ms"`
	Cents   []float64 `json:"cents"`
}

type prosodyFeatureCorpus struct {
	Version int                  `json:"version"`
	Cases   []prosodyFeatureCase `json:"cases"`
}

type prosodyFeatureCase struct {
	ID       string                 `json:"id"`
	Features []prosody.FeatureFrame `json:"features"`
}

type publicManifest struct {
	Version   int           `json:"version"`
	Mode      string        `json:"mode"`
	Corpus    string        `json:"corpus,omitempty"`
	Criterion string        `json:"criterion,omitempty"`
	Trials    []publicTrial `json:"trials"`
}

type answerKey struct {
	Version   int           `json:"version"`
	Mode      string        `json:"mode"`
	Seed      int64         `json:"seed"`
	Corpus    string        `json:"corpus,omitempty"`
	Criterion string        `json:"criterion,omitempty"`
	Trials    []answerTrial `json:"trials"`
	Failures  []string      `json:"failures,omitempty"`
}

func main() {
	var texts textList
	var cfg tts.Config
	var outputDirectory, mode, criterion, rendererA, rendererB, commonModel, modelA, modelB, commonProsody, prosodyA, prosodyB, commonFeatures, featuresAPath, featuresBPath, contourAPath, contourBPath, frameContourAPath, frameContourBPath, durationsAPath, durationsBPath, corpusPath string
	var rendererDirectories []string
	var seed int64
	var pitchOnlyA, pitchOnlyB, durationOnlyA, durationOnlyB, applyPitchA, applyPitchB bool
	var intonationStrengthA, intonationStrengthB float64
	var boundaryBridgeMSA, boundaryBridgeMSB float64
	var boundaryBridgeThresholdA, boundaryBridgeThresholdB float64
	var cvvcTimingA, cvvcTimingB, aliasPolicyA, aliasPolicyB string
	var cvvcTransitionGainA, cvvcTransitionGainB float64
	var cvvcPreBoundaryFadeA, cvvcPreBoundaryFadeB bool
	flag.StringVar(&cfg.VoicebankPath, "voicebank", "", "path to a UTAU voicebank directory")
	flag.Var(&texts, "text", "Japanese text to evaluate (repeatable)")
	flag.StringVar(&corpusPath, "corpus", "", "versioned evaluation corpus JSON")
	flag.StringVar(&commonModel, "join-model", "", "join-cost model used by both systems")
	flag.StringVar(&modelA, "system-a-join-model", "", "optional join model for system A")
	flag.StringVar(&modelB, "system-b-join-model", "", "optional join model for system B")
	flag.StringVar(&commonProsody, "prosody", "", "prosody model used by both systems")
	flag.StringVar(&prosodyA, "system-a-prosody", "", "optional prosody model for system A")
	flag.StringVar(&prosodyB, "system-b-prosody", "", "optional prosody model for system B")
	flag.StringVar(&commonFeatures, "prosody-features", "", "mora-level accent feature corpus used by both systems")
	flag.StringVar(&featuresAPath, "system-a-prosody-features", "", "optional accent feature corpus for system A")
	flag.StringVar(&featuresBPath, "system-b-prosody-features", "", "optional accent feature corpus for system B")
	flag.BoolVar(&pitchOnlyA, "system-a-prosody-pitch-only", false, "apply only prosody pitch prediction to system A")
	flag.BoolVar(&pitchOnlyB, "system-b-prosody-pitch-only", false, "apply only prosody pitch prediction to system B")
	flag.BoolVar(&durationOnlyA, "system-a-prosody-duration-only", false, "apply only prosody duration prediction to system A")
	flag.BoolVar(&durationOnlyB, "system-b-prosody-duration-only", false, "apply only prosody duration prediction to system B")
	flag.StringVar(&durationsAPath, "system-a-mora-durations", "", "optional per-case mora duration corpus for system A")
	flag.StringVar(&durationsBPath, "system-b-mora-durations", "", "optional per-case mora duration corpus for system B")
	flag.BoolVar(&applyPitchA, "system-a-apply-pitch", false, "enable experimental waveform pitch resampling for system A")
	flag.BoolVar(&applyPitchB, "system-b-apply-pitch", false, "enable experimental waveform pitch resampling for system B")
	flag.StringVar(&contourAPath, "system-a-pitch-contours", "", "optional per-case pitch contour JSON for system A")
	flag.StringVar(&contourBPath, "system-b-pitch-contours", "", "optional per-case pitch contour JSON for system B")
	flag.StringVar(&frameContourAPath, "system-a-frame-pitch-contours", "", "optional per-case frame pitch contour JSON for system A")
	flag.StringVar(&frameContourBPath, "system-b-frame-pitch-contours", "", "optional per-case frame pitch contour JSON for system B")
	flag.Float64Var(&intonationStrengthA, "system-a-intonation-strength", -1, "override intonation strength for system A (-1 uses --intonation-strength)")
	flag.Float64Var(&intonationStrengthB, "system-b-intonation-strength", -1, "override intonation strength for system B (-1 uses --intonation-strength)")
	flag.Float64Var(&boundaryBridgeMSA, "system-a-boundary-bridge-ms", 0, "maximum phase-aligned boundary repair width for system A (0 disables)")
	flag.Float64Var(&boundaryBridgeMSB, "system-b-boundary-bridge-ms", 0, "maximum phase-aligned boundary repair width for system B (0 disables)")
	flag.Float64Var(&boundaryBridgeThresholdA, "system-a-boundary-bridge-threshold", 0, "boundary repair score threshold for system A")
	flag.Float64Var(&boundaryBridgeThresholdB, "system-b-boundary-bridge-threshold", 0, "boundary repair score threshold for system B")
	flag.StringVar(&cvvcTimingA, "system-a-cvvc-timing", render.CVVCTimingLegacy, "system A CVVC timing: legacy or sequential")
	flag.StringVar(&cvvcTimingB, "system-b-cvvc-timing", render.CVVCTimingLegacy, "system B CVVC timing: legacy or sequential")
	flag.StringVar(&aliasPolicyA, "system-a-alias-policy", "auto", "system A alias policy: auto, vcv-prefer, cvvc-prefer, or cv-only")
	flag.StringVar(&aliasPolicyB, "system-b-alias-policy", "auto", "system B alias policy: auto, vcv-prefer, cvvc-prefer, or cv-only")
	flag.Float64Var(&cvvcTransitionGainA, "system-a-cvvc-transition-gain", 1, "system A CVVC transition volume multiplier (0..1)")
	flag.Float64Var(&cvvcTransitionGainB, "system-b-cvvc-transition-gain", 1, "system B CVVC transition volume multiplier (0..1)")
	flag.BoolVar(&cvvcPreBoundaryFadeA, "system-a-cvvc-pre-boundary-fade", false, "fade system A CVVC transitions out before the following CV consonant")
	flag.BoolVar(&cvvcPreBoundaryFadeB, "system-b-cvvc-pre-boundary-fade", false, "fade system B CVVC transitions out before the following CV consonant")
	flag.Float64Var(&cfg.JoinScoreScale, "join-scale", 0, "learned logit score scale")
	flag.StringVar(&rendererA, "system-a-renderer", "", "first renderer plugin ID (default: highest manifest priority)")
	flag.StringVar(&rendererB, "system-b-renderer", "", "second renderer plugin ID (default: next catalog entry)")
	flag.Func("renderer-dir", "renderer plugin directory (repeatable)", func(value string) error { rendererDirectories = append(rendererDirectories, value); return nil })
	flag.StringVar(&mode, "mode", "ab", "test mode: ab or abx")
	flag.StringVar(&criterion, "criterion", "総合的に自然な方を選んでください。", "listening-test question shown to participants")
	flag.StringVar(&outputDirectory, "out", "", "output test directory")
	flag.Int64Var(&seed, "seed", 1, "deterministic randomization seed")
	flag.StringVar(&cfg.Tone, "tone", "C4", "voicebank tone")
	flag.Float64Var(&cfg.MoraDurationMS, "mora-ms", 140, "base mora duration")
	flag.Float64Var(&cfg.PauseDurationMS, "pause-ms", 180, "pause duration")
	flag.Float64Var(&cfg.ReleaseMS, "release-ms", 20, "release envelope")
	flag.Float64Var(&cfg.IntonationStrength, "intonation-strength", 0, "source-pitch stabilization and phrase contour strength (0..1)")
	flag.StringVar(&cfg.WorldlinePath, "worldline", "", "path to worldline library")
	flag.StringVar(&cfg.WorldlineBridgePath, "worldline-bridge", "", "path to worldline bridge")
	flag.StringVar(&cfg.OpenJTalkPath, "openjtalk", "", "path to Open JTalk feature helper")
	flag.StringVar(&cfg.OpenJTalkDictionaryPath, "openjtalk-dictionary", "", "path to Open JTalk dictionary")
	flag.Parse()
	renderers, discoveryErr := rendererplugin.Discover(rendererDirectories)
	if discoveryErr != nil {
		log.Printf("renderer plugin discovery warning: %v", discoveryErr)
	}
	rendererPluginA, err := rendererplugin.Resolve(renderers, rendererA)
	if err != nil {
		log.Fatal(err)
	}
	if rendererB == "" {
		for _, candidate := range renderers {
			if candidate.ID != rendererPluginA.ID {
				rendererB = candidate.ID
				break
			}
		}
	}
	rendererPluginB, err := rendererplugin.Resolve(renderers, rendererB)
	if err != nil {
		log.Fatal(err)
	}
	rendererA, rendererB = rendererPluginA.ID, rendererPluginB.ID
	if cfg.VoicebankPath == "" || outputDirectory == "" || (len(texts) == 0 && corpusPath == "") || (mode != "ab" && mode != "abx") {
		flag.Usage()
		log.Fatal("--voicebank, --out, --text or --corpus, and mode ab/abx are required")
	}
	if intonationStrengthA < 0 {
		intonationStrengthA = cfg.IntonationStrength
	}
	if intonationStrengthB < 0 {
		intonationStrengthB = cfg.IntonationStrength
	}
	type evaluationCase struct{ id, text, reading string }
	cases := make([]evaluationCase, 0, len(texts))
	corpusName := ""
	if corpusPath != "" {
		corpus, err := evaluation.LoadCorpus(corpusPath)
		if err != nil {
			log.Fatal(err)
		}
		corpusName = corpus.Name
		for _, item := range corpus.Cases {
			cases = append(cases, evaluationCase{id: item.ID, text: item.Text, reading: item.Reading})
		}
	}
	for index, text := range texts {
		cases = append(cases, evaluationCase{id: fmt.Sprintf("custom-%03d", index+1), text: text})
	}
	if modelA == "" {
		modelA = commonModel
	}
	if modelB == "" {
		modelB = commonModel
	}
	if prosodyA == "" {
		prosodyA = commonProsody
	}
	if prosodyB == "" {
		prosodyB = commonProsody
	}
	if pitchOnlyA && durationOnlyA || pitchOnlyB && durationOnlyB {
		log.Fatal("a system cannot be both prosody-pitch-only and prosody-duration-only")
	}
	durationModelA, err := loadDurationOnlyModel(prosodyA, durationOnlyA)
	if err != nil {
		log.Fatal(err)
	}
	durationModelB, err := loadDurationOnlyModel(prosodyB, durationOnlyB)
	if err != nil {
		log.Fatal(err)
	}
	if featuresAPath == "" {
		featuresAPath = commonFeatures
	}
	if featuresBPath == "" {
		featuresBPath = commonFeatures
	}
	featuresA, err := loadProsodyFeatureCorpus(featuresAPath)
	if err != nil {
		log.Fatal(err)
	}
	featuresB, err := loadProsodyFeatureCorpus(featuresBPath)
	if err != nil {
		log.Fatal(err)
	}
	contoursA, err := loadPitchContours(contourAPath)
	if err != nil {
		log.Fatal(err)
	}
	contoursB, err := loadPitchContours(contourBPath)
	if err != nil {
		log.Fatal(err)
	}
	frameContoursA, err := loadFramePitchContours(frameContourAPath)
	if err != nil {
		log.Fatal(err)
	}
	frameContoursB, err := loadFramePitchContours(frameContourBPath)
	if err != nil {
		log.Fatal(err)
	}
	durationsA, err := loadMoraDurations(durationsAPath)
	if err != nil {
		log.Fatal(err)
	}
	durationsB, err := loadMoraDurations(durationsBPath)
	if err != nil {
		log.Fatal(err)
	}
	publicDirectory := filepath.Join(outputDirectory, "public")
	if err := os.MkdirAll(publicDirectory, 0o755); err != nil {
		log.Fatal(err)
	}
	random := rand.New(rand.NewSource(seed))
	manifest := publicManifest{Version: 2, Mode: mode, Corpus: corpusName, Criterion: criterion}
	key := answerKey{Version: 3, Mode: mode, Seed: seed, Corpus: corpusName, Criterion: criterion}
	for _, item := range cases {
		text := item.text
		if missing := missingCaseInput(
			caseInput{"system A prosody features", featuresAPath, hasKey(featuresA, item.id)},
			caseInput{"system B prosody features", featuresBPath, hasKey(featuresB, item.id)},
			caseInput{"system A pitch contour", contourAPath, hasKey(contoursA, item.id)},
			caseInput{"system B pitch contour", contourBPath, hasKey(contoursB, item.id)},
			caseInput{"system A frame pitch contour", frameContourAPath, hasKey(frameContoursA, item.id)},
			caseInput{"system B frame pitch contour", frameContourBPath, hasKey(frameContoursB, item.id)},
			caseInput{"system A mora durations", durationsAPath, hasKey(durationsA, item.id)},
			caseInput{"system B mora durations", durationsBPath, hasKey(durationsB, item.id)},
		); missing != "" {
			key.Failures = append(key.Failures, fmt.Sprintf("%s: %s has no case %q", text, missing, item.id))
			continue
		}
		cfgA := cfg
		cfgA.Text = text
		cfgA.Reading = item.reading
		cfgA.ProsodyFeatures = featuresA[item.id]
		cfgA.PitchFactors = contoursA[item.id]
		cfgA.PitchCurve = frameContoursA[item.id]
		cfgA.MoraDurationsMS = durationsA[item.id].MoraDurationsMS
		if durationsA[item.id].PauseDurationMS > 0 {
			cfgA.PauseDurationMS = durationsA[item.id].PauseDurationMS
		}
		cfgA.JoinModelPath, cfgA.ProsodyModelPath, cfgA.ProsodyModel, cfgA.ProsodyPitchOnly, cfgA.ApplyPitch, cfgA.IntonationStrength = modelA, prosodyA, durationModelA, pitchOnlyA, applyPitchA || contourAPath != "" || frameContourAPath != "", intonationStrengthA
		if durationModelA != nil {
			cfgA.ProsodyModelPath = ""
		}
		cfgA.BoundaryBridgeMS, cfgA.BoundaryBridgeThreshold = boundaryBridgeMSA, boundaryBridgeThresholdA
		cfgA.CVVCTiming = cvvcTimingA
		cfgA.CVVCTransitionGain = cvvcTransitionGainA
		cfgA.CVVCPreBoundaryFade = cvvcPreBoundaryFadeA
		cfgA.AliasPolicy = voicebank.AliasPolicy(aliasPolicyA)
		rendererplugin.Apply(rendererPluginA, &cfgA)
		first, err := tts.Synthesize(cfgA)
		if err != nil {
			key.Failures = append(key.Failures, fmt.Sprintf("%s: system A: %v", text, err))
			continue
		}
		cfgB := cfg
		cfgB.Text = text
		cfgB.Reading = item.reading
		cfgB.PitchFactors = contoursB[item.id]
		cfgB.PitchCurve = frameContoursB[item.id]
		cfgB.ProsodyFeatures = featuresB[item.id]
		cfgB.MoraDurationsMS = durationsB[item.id].MoraDurationsMS
		if durationsB[item.id].PauseDurationMS > 0 {
			cfgB.PauseDurationMS = durationsB[item.id].PauseDurationMS
		}
		cfgB.JoinModelPath, cfgB.ProsodyModelPath, cfgB.ProsodyModel, cfgB.ProsodyPitchOnly, cfgB.ApplyPitch, cfgB.IntonationStrength = modelB, prosodyB, durationModelB, pitchOnlyB, applyPitchB || contourBPath != "" || frameContourBPath != "", intonationStrengthB
		if durationModelB != nil {
			cfgB.ProsodyModelPath = ""
		}
		cfgB.BoundaryBridgeMS, cfgB.BoundaryBridgeThreshold = boundaryBridgeMSB, boundaryBridgeThresholdB
		cfgB.CVVCTiming = cvvcTimingB
		cfgB.CVVCTransitionGain = cvvcTransitionGainB
		cfgB.CVVCPreBoundaryFade = cvvcPreBoundaryFadeB
		cfgB.AliasPolicy = voicebank.AliasPolicy(aliasPolicyB)
		rendererplugin.Apply(rendererPluginB, &cfgB)
		second, err := tts.Synthesize(cfgB)
		if err != nil {
			key.Failures = append(key.Failures, fmt.Sprintf("%s: system B: %v", text, err))
			continue
		}
		if sameAudio(first.Audio, second.Audio) {
			key.Failures = append(key.Failures, fmt.Sprintf("%s: systems produced identical audio", text))
			continue
		}
		trialID := len(manifest.Trials) + 1
		left, right := first, second
		leftInfo := systemInfo{Renderer: rendererA, AliasPolicy: aliasPolicyA, JoinModel: modelA != "", JoinModelPath: modelA, ProsodyModel: prosodyA != "", ProsodyPath: prosodyA, ProsodyFeaturesPath: featuresAPath, ProsodyPitchOnly: pitchOnlyA, ProsodyDurationOnly: durationOnlyA, MoraDurationsPath: durationsAPath, ApplyPitch: applyPitchA || pitchOnlyA || contourAPath != "" || frameContourAPath != "", PitchContourPath: contourAPath, FramePitchContourPath: frameContourAPath, IntonationStrength: intonationStrengthA, BoundaryBridgeMS: boundaryBridgeMSA, BoundaryBridgeThreshold: boundaryBridgeThresholdA, CVVCTiming: cvvcTimingA, CVVCTransitionGain: cvvcTransitionGainA, CVVCPreBoundaryFade: cvvcPreBoundaryFadeA, LongUnitGroups: longUnitGroups(first.Plan.Units)}
		rightInfo := systemInfo{Renderer: rendererB, AliasPolicy: aliasPolicyB, JoinModel: modelB != "", JoinModelPath: modelB, ProsodyModel: prosodyB != "", ProsodyPath: prosodyB, ProsodyFeaturesPath: featuresBPath, ProsodyPitchOnly: pitchOnlyB, ProsodyDurationOnly: durationOnlyB, MoraDurationsPath: durationsBPath, ApplyPitch: applyPitchB || pitchOnlyB || contourBPath != "" || frameContourBPath != "", PitchContourPath: contourBPath, FramePitchContourPath: frameContourBPath, IntonationStrength: intonationStrengthB, BoundaryBridgeMS: boundaryBridgeMSB, BoundaryBridgeThreshold: boundaryBridgeThresholdB, CVVCTiming: cvvcTimingB, CVVCTransitionGain: cvvcTransitionGainB, CVVCPreBoundaryFade: cvvcPreBoundaryFadeB, LongUnitGroups: longUnitGroups(second.Plan.Units)}
		if random.Intn(2) == 1 {
			left, right, leftInfo, rightInfo = right, left, rightInfo, leftInfo
		}
		prefix := fmt.Sprintf("trial-%03d", trialID)
		aName, bName := prefix+"-A.wav", prefix+"-B.wav"
		if err := audio.WriteWav(filepath.Join(publicDirectory, aName), left.Audio); err != nil {
			log.Fatal(err)
		}
		if err := audio.WriteWav(filepath.Join(publicDirectory, bName), right.Audio); err != nil {
			log.Fatal(err)
		}
		public := publicTrial{ID: trialID, CaseID: item.id, Text: text, A: aName, B: bName}
		answer := answerTrial{ID: trialID, CaseID: item.id, Text: text, A: leftInfo, B: rightInfo}
		answer.Changes = selectionChanges(left.Plan, right.Plan)
		if mode == "abx" {
			xName := prefix + "-X.wav"
			xAudio := left.Audio
			answer.XAnswer = "A"
			if random.Intn(2) == 1 {
				xAudio, answer.XAnswer = right.Audio, "B"
			}
			if err := audio.WriteWav(filepath.Join(publicDirectory, xName), xAudio); err != nil {
				log.Fatal(err)
			}
			public.X = xName
		}
		manifest.Trials = append(manifest.Trials, public)
		key.Trials = append(key.Trials, answer)
	}
	if len(manifest.Trials) == 0 {
		log.Fatal("no listening trials could be generated")
	}
	if err := writeJSON(filepath.Join(publicDirectory, "trials.json"), manifest); err != nil {
		log.Fatal(err)
	}
	if err := writeJSON(filepath.Join(outputDirectory, "answer-key.json"), key); err != nil {
		log.Fatal(err)
	}
	if err := writeHTML(filepath.Join(publicDirectory, "index.html"), manifest); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("wrote %d %s trials to %s (keep answer-key.json private)\n", len(manifest.Trials), mode, publicDirectory)
}

type caseInput struct {
	name    string
	path    string
	present bool
}

func missingCaseInput(inputs ...caseInput) string {
	for _, input := range inputs {
		if input.path != "" && !input.present {
			return input.name
		}
	}
	return ""
}

func hasKey[K comparable, V any](values map[K]V, key K) bool {
	_, ok := values[key]
	return ok
}

func loadPitchContours(path string) (map[string][]float64, error) {
	result := map[string][]float64{}
	if path == "" {
		return result, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read pitch contours: %w", err)
	}
	var corpus pitchContourCorpus
	if err := json.Unmarshal(data, &corpus); err != nil {
		return nil, fmt.Errorf("decode pitch contours: %w", err)
	}
	if corpus.Version != 1 {
		return nil, fmt.Errorf("unsupported pitch contour version %d", corpus.Version)
	}
	for _, item := range corpus.Cases {
		result[item.ID] = item.PitchFactors
	}
	return result, nil
}

func loadFramePitchContours(path string) (map[string]*render.PitchCurve, error) {
	result := map[string]*render.PitchCurve{}
	if path == "" {
		return result, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read frame pitch contours: %w", err)
	}
	var corpus framePitchContourCorpus
	if err := json.Unmarshal(data, &corpus); err != nil {
		return nil, fmt.Errorf("decode frame pitch contours: %w", err)
	}
	if corpus.Version != 1 {
		return nil, fmt.Errorf("unsupported frame pitch contour version %d", corpus.Version)
	}
	for _, item := range corpus.Cases {
		if item.ID == "" || item.FrameMS < 0.1 || len(item.Cents) == 0 {
			return nil, fmt.Errorf("invalid frame pitch contour %q", item.ID)
		}
		result[item.ID] = &render.PitchCurve{FrameMS: item.FrameMS, Cents: append([]float64(nil), item.Cents...)}
	}
	return result, nil
}

func loadMoraDurations(path string) (map[string]moraDurationCase, error) {
	result := map[string]moraDurationCase{}
	if path == "" {
		return result, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read mora durations: %w", err)
	}
	var corpus moraDurationCorpus
	if err := json.Unmarshal(data, &corpus); err != nil {
		return nil, fmt.Errorf("decode mora durations: %w", err)
	}
	if corpus.Version != 1 {
		return nil, fmt.Errorf("unsupported mora duration version %d", corpus.Version)
	}
	for _, item := range corpus.Cases {
		if item.ID == "" || len(item.MoraDurationsMS) == 0 {
			return nil, fmt.Errorf("invalid mora duration case %q", item.ID)
		}
		if _, exists := result[item.ID]; exists {
			return nil, fmt.Errorf("duplicate mora duration case %q", item.ID)
		}
		for index, duration := range item.MoraDurationsMS {
			if math.IsNaN(duration) || math.IsInf(duration, 0) || duration < 0 {
				return nil, fmt.Errorf("invalid mora duration case %q value %d", item.ID, index)
			}
		}
		if math.IsNaN(item.PauseDurationMS) || math.IsInf(item.PauseDurationMS, 0) || item.PauseDurationMS < 0 {
			return nil, fmt.Errorf("invalid pause duration for case %q", item.ID)
		}
		item.MoraDurationsMS = append([]float64(nil), item.MoraDurationsMS...)
		result[item.ID] = item
	}
	return result, nil
}

func loadDurationOnlyModel(path string, enabled bool) (*prosody.Model, error) {
	if !enabled {
		return nil, nil
	}
	if path == "" {
		return nil, fmt.Errorf("prosody-duration-only requires a prosody model")
	}
	model, err := prosody.LoadModel(path)
	if err != nil {
		return nil, fmt.Errorf("load duration-only prosody model: %w", err)
	}
	if model.MoraDuration == nil {
		return nil, fmt.Errorf("prosody model %q has no mora duration head", path)
	}
	clone := *model
	clone.SequencePitch = nil
	clone.FramePitch = nil
	clone.PhrasePitch = nil
	clone.StandardAccent = nil
	return &clone, nil
}

func loadProsodyFeatureCorpus(path string) (map[string][]prosody.FeatureFrame, error) {
	result := map[string][]prosody.FeatureFrame{}
	if path == "" {
		return result, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var corpus prosodyFeatureCorpus
	if err := json.Unmarshal(data, &corpus); err != nil {
		return nil, err
	}
	if corpus.Version != 1 {
		return nil, fmt.Errorf("unsupported prosody feature corpus version %d", corpus.Version)
	}
	for _, item := range corpus.Cases {
		if item.ID == "" || len(item.Features) == 0 {
			return nil, fmt.Errorf("invalid prosody feature case %q", item.ID)
		}
		result[item.ID] = item.Features
	}
	return result, nil
}

func selectionChanges(left, right *plan.Plan) []selectionChange {
	if left == nil || right == nil {
		return nil
	}
	length := min(len(left.Units), len(right.Units))
	var result []selectionChange
	for index := 0; index < length; index++ {
		a, b := left.Units[index], right.Units[index]
		if a.Alias == b.Alias && a.Source == b.Source && a.OtoLine == b.OtoLine {
			continue
		}
		result = append(result, selectionChange{
			Position: a.Position, Mora: a.Mora,
			A: unitChoice{Alias: a.Alias, Source: a.Source, OtoLine: a.OtoLine, TargetScore: a.TargetScore, JoinScore: a.JoinScore, JoinProbability: a.JoinProbability},
			B: unitChoice{Alias: b.Alias, Source: b.Source, OtoLine: b.OtoLine, TargetScore: b.TargetScore, JoinScore: b.JoinScore, JoinProbability: b.JoinProbability},
		})
	}
	return result
}

func sameAudio(left, right *audio.PCM) bool {
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

func longUnitGroups(units []plan.Unit) int {
	seen := map[int]bool{}
	for _, unit := range units {
		if unit.LongUnitGroup > 0 {
			seen[unit.LongUnitGroup] = true
		}
	}
	return len(seen)
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func writeHTML(path string, manifest publicManifest) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	return pageTemplate.Execute(file, manifest)
}

var pageTemplate = template.Must(template.New("page").Parse(`<!doctype html>
<html lang="ja"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width">
<title>UtauTTS {{.Mode}} listening test</title><style>
body{font-family:system-ui,sans-serif;max-width:850px;margin:2rem auto;padding:0 1rem}fieldset{margin:1.5rem 0;padding:1rem}audio{width:100%;margin:.3rem 0}label{margin-right:1.2rem}button{padding:.7rem 1rem}
</style></head><body><h1>{{if eq .Mode "abx"}}ABX{{else}}AB{{end}} listening test</h1>
<p>音量を一定にし、可能ならヘッドホンで回答してください。ページを閉じる前に結果を保存してください。</p>
{{if .Criterion}}<p><strong>評価基準:</strong> {{.Criterion}}</p>{{end}}
<form id="test">{{range .Trials}}<fieldset data-id="{{.ID}}"><legend>{{.ID}}. {{.Text}}</legend>
<div>A<audio controls preload="none" src="{{.A}}"></audio></div><div>B<audio controls preload="none" src="{{.B}}"></audio></div>
{{if eq $.Mode "abx"}}<div>X<audio controls preload="none" src="{{.X}}"></audio></div>
<label><input required type="radio" name="q{{.ID}}" value="A">XはA</label><label><input type="radio" name="q{{.ID}}" value="B">XはB</label><label><input type="radio" name="q{{.ID}}" value="unsure">不明</label>
{{else}}<label><input required type="radio" name="q{{.ID}}" value="A">Aが自然</label><label><input type="radio" name="q{{.ID}}" value="B">Bが自然</label><label><input type="radio" name="q{{.ID}}" value="tie">同程度</label>{{end}}</fieldset>{{end}}
<button type="submit">回答JSONを保存</button></form><script>
document.querySelector('#test').addEventListener('submit',e=>{e.preventDefault();const answers=[];document.querySelectorAll('fieldset').forEach(f=>{const x=f.querySelector('input:checked');answers.push({id:Number(f.dataset.id),answer:x?.value||''})});const blob=new Blob([JSON.stringify({version:1,mode:'{{.Mode}}',answers},null,2)],{type:'application/json'});const a=document.createElement('a');a.href=URL.createObjectURL(blob);a.download='listening-results.json';a.click();URL.revokeObjectURL(a.href)});
</script></body></html>`))
