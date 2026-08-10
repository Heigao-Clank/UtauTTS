package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"strings"

	"utautts/internal/audio"
	"utautts/internal/evaluation"
	"utautts/internal/plan"
	"utautts/internal/tts"
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
	Renderer           string  `json:"renderer"`
	JoinModel          bool    `json:"join_model"`
	JoinModelPath      string  `json:"join_model_path,omitempty"`
	ProsodyModel       bool    `json:"prosody_model,omitempty"`
	ProsodyPath        string  `json:"prosody_model_path,omitempty"`
	IntonationStrength float64 `json:"intonation_strength,omitempty"`
	LongUnitGroups     int     `json:"long_unit_groups"`
}

type publicManifest struct {
	Version int           `json:"version"`
	Mode    string        `json:"mode"`
	Corpus  string        `json:"corpus,omitempty"`
	Trials  []publicTrial `json:"trials"`
}

type answerKey struct {
	Version  int           `json:"version"`
	Mode     string        `json:"mode"`
	Seed     int64         `json:"seed"`
	Corpus   string        `json:"corpus,omitempty"`
	Trials   []answerTrial `json:"trials"`
	Failures []string      `json:"failures,omitempty"`
}

func main() {
	var texts textList
	var cfg tts.Config
	var outputDirectory, mode, rendererA, rendererB, commonModel, modelA, modelB, commonProsody, prosodyA, prosodyB, corpusPath string
	var seed int64
	flag.StringVar(&cfg.VoicebankPath, "voicebank", "", "path to a UTAU voicebank directory")
	flag.Var(&texts, "text", "Japanese text to evaluate (repeatable)")
	flag.StringVar(&corpusPath, "corpus", "", "versioned evaluation corpus JSON")
	flag.StringVar(&commonModel, "join-model", "", "join-cost model used by both systems")
	flag.StringVar(&modelA, "system-a-join-model", "", "optional join model for system A")
	flag.StringVar(&modelB, "system-b-join-model", "", "optional join model for system B")
	flag.StringVar(&commonProsody, "prosody", "", "prosody model used by both systems")
	flag.StringVar(&prosodyA, "system-a-prosody", "", "optional prosody model for system A")
	flag.StringVar(&prosodyB, "system-b-prosody", "", "optional prosody model for system B")
	flag.Float64Var(&cfg.JoinScoreScale, "join-scale", 0, "learned logit score scale")
	flag.StringVar(&rendererA, "system-a-renderer", "waveform", "first renderer")
	flag.StringVar(&rendererB, "system-b-renderer", "waveform-long", "second renderer")
	flag.StringVar(&mode, "mode", "ab", "test mode: ab or abx")
	flag.StringVar(&outputDirectory, "out", "", "output test directory")
	flag.Int64Var(&seed, "seed", 1, "deterministic randomization seed")
	flag.StringVar(&cfg.Tone, "tone", "C4", "voicebank tone")
	flag.Float64Var(&cfg.MoraDurationMS, "mora-ms", 140, "base mora duration")
	flag.Float64Var(&cfg.PauseDurationMS, "pause-ms", 180, "pause duration")
	flag.Float64Var(&cfg.ReleaseMS, "release-ms", 20, "release envelope")
	flag.Float64Var(&cfg.IntonationStrength, "intonation-strength", 0, "source-pitch stabilization and phrase contour strength (0..1)")
	flag.StringVar(&cfg.WorldlinePath, "worldline", "", "path to worldline library")
	flag.StringVar(&cfg.WorldlineBridgePath, "worldline-bridge", "", "path to worldline bridge")
	flag.Parse()
	if cfg.VoicebankPath == "" || outputDirectory == "" || (len(texts) == 0 && corpusPath == "") || (mode != "ab" && mode != "abx") {
		flag.Usage()
		log.Fatal("--voicebank, --out, --text or --corpus, and mode ab/abx are required")
	}
	type evaluationCase struct{ id, text string }
	cases := make([]evaluationCase, 0, len(texts))
	corpusName := ""
	if corpusPath != "" {
		corpus, err := evaluation.LoadCorpus(corpusPath)
		if err != nil {
			log.Fatal(err)
		}
		corpusName = corpus.Name
		for _, item := range corpus.Cases {
			cases = append(cases, evaluationCase{id: item.ID, text: item.Text})
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
	publicDirectory := filepath.Join(outputDirectory, "public")
	if err := os.MkdirAll(publicDirectory, 0o755); err != nil {
		log.Fatal(err)
	}
	random := rand.New(rand.NewSource(seed))
	manifest := publicManifest{Version: 2, Mode: mode, Corpus: corpusName}
	key := answerKey{Version: 3, Mode: mode, Seed: seed, Corpus: corpusName}
	for _, item := range cases {
		text := item.text
		cfg.Text = text
		cfg.Renderer, cfg.JoinModelPath, cfg.ProsodyModelPath = rendererA, modelA, prosodyA
		first, err := tts.Synthesize(cfg)
		if err != nil {
			key.Failures = append(key.Failures, fmt.Sprintf("%s: system A: %v", text, err))
			continue
		}
		cfg.Renderer, cfg.JoinModelPath, cfg.ProsodyModelPath = rendererB, modelB, prosodyB
		second, err := tts.Synthesize(cfg)
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
		leftInfo := systemInfo{Renderer: rendererA, JoinModel: modelA != "", JoinModelPath: modelA, ProsodyModel: prosodyA != "", ProsodyPath: prosodyA, IntonationStrength: cfg.IntonationStrength, LongUnitGroups: longUnitGroups(first.Plan.Units)}
		rightInfo := systemInfo{Renderer: rendererB, JoinModel: modelB != "", JoinModelPath: modelB, ProsodyModel: prosodyB != "", ProsodyPath: prosodyB, IntonationStrength: cfg.IntonationStrength, LongUnitGroups: longUnitGroups(second.Plan.Units)}
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
<form id="test">{{range .Trials}}<fieldset data-id="{{.ID}}"><legend>{{.ID}}. {{.Text}}</legend>
<div>A<audio controls preload="none" src="{{.A}}"></audio></div><div>B<audio controls preload="none" src="{{.B}}"></audio></div>
{{if eq $.Mode "abx"}}<div>X<audio controls preload="none" src="{{.X}}"></audio></div>
<label><input required type="radio" name="q{{.ID}}" value="A">XはA</label><label><input type="radio" name="q{{.ID}}" value="B">XはB</label><label><input type="radio" name="q{{.ID}}" value="unsure">不明</label>
{{else}}<label><input required type="radio" name="q{{.ID}}" value="A">Aが自然</label><label><input type="radio" name="q{{.ID}}" value="B">Bが自然</label><label><input type="radio" name="q{{.ID}}" value="tie">同程度</label>{{end}}</fieldset>{{end}}
<button type="submit">回答JSONを保存</button></form><script>
document.querySelector('#test').addEventListener('submit',e=>{e.preventDefault();const answers=[];document.querySelectorAll('fieldset').forEach(f=>{const x=f.querySelector('input:checked');answers.push({id:Number(f.dataset.id),answer:x?.value||''})});const blob=new Blob([JSON.stringify({version:1,mode:'{{.Mode}}',answers},null,2)],{type:'application/json'});const a=document.createElement('a');a.href=URL.createObjectURL(blob);a.download='listening-results.json';a.click();URL.revokeObjectURL(a.href)});
</script></body></html>`))
