package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"log"
	"os"
	"path/filepath"
	"strings"

	"utautts/internal/audio"
	"utautts/internal/ceiling"
	"utautts/internal/plan"
	"utautts/internal/render"
	"utautts/internal/voicebank"
)

type report struct {
	Version         int          `json:"version"`
	Voicebank       string       `json:"voicebank"`
	Root            string       `json:"root"`
	MoraDurationMS  float64      `json:"mora_duration_ms"`
	MinimumVowelMS  float64      `json:"minimum_vowel_ms"`
	Discovered      int          `json:"discovered_sequences"`
	Cases           []caseReport `json:"cases"`
	Failures        []string     `json:"failures,omitempty"`
	ClassicFaithful bool         `json:"classic_faithful"`
}

type caseReport struct {
	ID                   string             `json:"id"`
	Aliases              []string           `json:"aliases"`
	Source               string             `json:"source"`
	OtoPath              string             `json:"oto_path"`
	SourceStartMS        float64            `json:"source_start_ms"`
	SourceEndMS          float64            `json:"source_end_ms"`
	OriginalDurationMS   float64            `json:"original_duration_ms"`
	CurrentDurationMS    float64            `json:"current_duration_ms"`
	AnchoredDurationMS   float64            `json:"anchored_duration_ms"`
	ContinuousDurationMS float64            `json:"continuous_anchor_duration_ms"`
	Intervals            []ceiling.Interval `json:"intervals"`
	OriginalWAV          string             `json:"original_wav"`
	ReconstructedWAV     string             `json:"reconstructed_original_wav"`
	CurrentWAV           string             `json:"current_waveform_wav"`
	AnchoredWAV          string             `json:"anchor_preserving_wav"`
	ContinuousWAV        string             `json:"continuous_anchor_wav"`
	ClassicFaithfulWAV   string             `json:"classic_faithful_wav,omitempty"`
	ClassicDurationMS    float64            `json:"classic_faithful_duration_ms,omitempty"`
	WholeWorldlineWAV    string             `json:"whole_worldline_wav,omitempty"`
	WholeDurationMS      float64            `json:"whole_worldline_duration_ms,omitempty"`
	OriginalPlan         string             `json:"original_plan"`
	CurrentPlan          string             `json:"current_plan"`
}

func main() {
	var voicebankPath, outputDirectory, worldlinePath, worldlineBridgePath string
	var caseLimit, minimumEntries, maximumEntries int
	var moraMS, minimumVowelMS float64
	flag.StringVar(&voicebankPath, "voicebank", "", "path to a UTAU voicebank directory")
	flag.StringVar(&outputDirectory, "out", "", "output directory")
	flag.IntVar(&caseLimit, "cases", 12, "maximum number of sequences to export")
	flag.IntVar(&minimumEntries, "min-units", 3, "minimum VCV entries in one source recording")
	flag.IntVar(&maximumEntries, "max-units", 8, "maximum VCV entries per exported sequence")
	flag.Float64Var(&moraMS, "mora-ms", 140, "target duration per mora in milliseconds")
	flag.Float64Var(&minimumVowelMS, "min-vowel-ms", 25, "minimum stretchable vowel duration in milliseconds")
	flag.StringVar(&worldlinePath, "worldline", "", "path to worldline library for Classic faithful comparison")
	flag.StringVar(&worldlineBridgePath, "worldline-bridge", "", "path to worldline bridge for Classic faithful comparison")
	flag.Parse()
	if voicebankPath == "" || outputDirectory == "" {
		flag.Usage()
		log.Fatal("--voicebank and --out are required")
	}
	if caseLimit < 1 || minimumEntries < 2 || maximumEntries < minimumEntries || moraMS <= 0 || minimumVowelMS < 0 {
		log.Fatal("invalid experiment limits")
	}
	if (worldlinePath == "") != (worldlineBridgePath == "") {
		log.Fatal("--worldline and --worldline-bridge must be specified together")
	}
	bank, err := voicebank.Load(voicebankPath)
	if err != nil {
		log.Fatal(err)
	}
	if err := os.MkdirAll(outputDirectory, 0o755); err != nil {
		log.Fatal(err)
	}
	sequences := ceiling.Discover(bank, minimumEntries, maximumEntries)
	resultReport := report{
		Version: 1, Voicebank: bank.Name, Root: bank.Root,
		MoraDurationMS: moraMS, MinimumVowelMS: minimumVowelMS, Discovered: len(sequences),
		ClassicFaithful: worldlinePath != "",
	}
	for _, sequence := range sequences {
		if len(resultReport.Cases) >= caseLimit {
			break
		}
		id := fmt.Sprintf("case-%03d", len(resultReport.Cases)+1)
		generated, err := ceiling.Generate(sequence, ceiling.Config{MoraDurationMS: moraMS, MinimumVowelMS: minimumVowelMS})
		if err != nil {
			resultReport.Failures = append(resultReport.Failures, fmt.Sprintf("%s: %v", relative(bank.Root, sequence.Source), err))
			continue
		}
		caseDirectory := filepath.Join(outputDirectory, id)
		if err := os.MkdirAll(caseDirectory, 0o755); err != nil {
			log.Fatal(err)
		}
		files := map[string]*audio.PCM{
			"01-original.wav":               generated.Original,
			"02-reconstructed-original.wav": generated.ReconstructedOriginal,
			"03-current-tts.wav":            generated.Current,
			"04-anchor-preserving.wav":      generated.Anchored,
			"05-continuous-anchor.wav":      generated.ContinuousAnchored,
		}
		var classic, wholeWorldline *audio.PCM
		if worldlinePath != "" {
			classic, err = render.Render(generated.CurrentPlan, render.Config{
				Backend:       "openutau-classic-worldline-faithful",
				WorldlinePath: worldlinePath, WorldlineBridgePath: worldlineBridgePath,
				ReleaseMS: 0, ReleaseSet: true,
			})
			if err != nil {
				resultReport.Failures = append(resultReport.Failures, fmt.Sprintf("%s Classic faithful: %v", relative(bank.Root, sequence.Source), err))
				continue
			}
			files["06-openutau-classic-faithful.wav"] = classic
			continuousPath := filepath.Join(caseDirectory, "05-continuous-anchor.wav")
			if err := audio.WriteWav(continuousPath, generated.ContinuousAnchored); err != nil {
				log.Fatalf("write continuous anchor source: %v", err)
			}
			wholePlan := wholeRunPlan(generated.CurrentPlan, continuousPath, generated.ContinuousAnchored)
			wholeWorldline, err = render.Render(wholePlan, render.Config{
				Backend:       "openutau-classic-worldline-faithful",
				WorldlinePath: worldlinePath, WorldlineBridgePath: worldlineBridgePath,
				WorldlineExactLength: true, ReleaseMS: 0, ReleaseSet: true,
			})
			if err != nil {
				resultReport.Failures = append(resultReport.Failures, fmt.Sprintf("%s whole Worldline: %v", relative(bank.Root, sequence.Source), err))
				continue
			}
			files["07-continuous-anchor-worldline.wav"] = wholeWorldline
		}
		for name, pcm := range files {
			if err := audio.WriteWav(filepath.Join(caseDirectory, name), pcm); err != nil {
				log.Fatalf("write %s: %v", name, err)
			}
		}
		if err := writeJSON(filepath.Join(caseDirectory, "original-plan.json"), generated.OriginalPlan); err != nil {
			log.Fatal(err)
		}
		if err := writeJSON(filepath.Join(caseDirectory, "current-plan.json"), generated.CurrentPlan); err != nil {
			log.Fatal(err)
		}
		aliases := make([]string, len(sequence.Entries))
		for index, entry := range sequence.Entries {
			aliases[index] = entry.Alias
		}
		currentDurationMS := float64(len(generated.Current.Data)) * 1000 / float64(generated.Current.SampleRate)
		originalDurationMS := float64(len(generated.Original.Data)) * 1000 / float64(generated.Original.SampleRate)
		currentCase := caseReport{
			ID: id, Aliases: aliases, Source: relative(bank.Root, sequence.Source), OtoPath: relative(bank.Root, sequence.OtoPath),
			SourceStartMS: generated.SourceStartMS, SourceEndMS: generated.SourceEndMS,
			OriginalDurationMS: originalDurationMS, CurrentDurationMS: currentDurationMS,
			AnchoredDurationMS: generated.AnchoredDurationMS, Intervals: generated.Intervals,
			ContinuousDurationMS: generated.ContinuousDurationMS,
			OriginalWAV:          filepath.ToSlash(filepath.Join(id, "01-original.wav")),
			ReconstructedWAV:     filepath.ToSlash(filepath.Join(id, "02-reconstructed-original.wav")),
			CurrentWAV:           filepath.ToSlash(filepath.Join(id, "03-current-tts.wav")),
			AnchoredWAV:          filepath.ToSlash(filepath.Join(id, "04-anchor-preserving.wav")),
			ContinuousWAV:        filepath.ToSlash(filepath.Join(id, "05-continuous-anchor.wav")),
			OriginalPlan:         filepath.ToSlash(filepath.Join(id, "original-plan.json")),
			CurrentPlan:          filepath.ToSlash(filepath.Join(id, "current-plan.json")),
		}
		if classic != nil {
			currentCase.ClassicFaithfulWAV = filepath.ToSlash(filepath.Join(id, "06-openutau-classic-faithful.wav"))
			currentCase.ClassicDurationMS = float64(len(classic.Data)) * 1000 / float64(classic.SampleRate)
			currentCase.WholeWorldlineWAV = filepath.ToSlash(filepath.Join(id, "07-continuous-anchor-worldline.wav"))
			currentCase.WholeDurationMS = pcmDurationMS(wholeWorldline)
		}
		resultReport.Cases = append(resultReport.Cases, currentCase)
		fmt.Printf("%s units=%d source=%.0fms current=%.0fms anchored=%.0fms %s\n",
			id, len(aliases), originalDurationMS, currentDurationMS, generated.AnchoredDurationMS, strings.Join(aliases, " / "))
	}
	if len(resultReport.Cases) == 0 {
		log.Fatalf("no usable VCV sequences found (%d discovered, %d failed)", len(sequences), len(resultReport.Failures))
	}
	if err := writeJSON(filepath.Join(outputDirectory, "report.json"), resultReport); err != nil {
		log.Fatal(err)
	}
	if err := writeHTML(filepath.Join(outputDirectory, "index.html"), resultReport); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("wrote %s\n", filepath.Join(outputDirectory, "index.html"))
}

func wholeRunPlan(base *plan.Plan, source string, pcm *audio.PCM) *plan.Plan {
	duration := pcmDurationMS(pcm)
	result := &plan.Plan{Version: plan.Version, DurationMS: duration}
	if base != nil {
		result.Voicebank = base.Voicebank
		result.Reading = base.Reading
	}
	result.Units = []plan.Unit{{
		Position: 0, Role: "mora", Mora: "run", Alias: "<continuous-anchor-run>",
		Source: source, NoteStartMS: 0, DurationMS: duration,
		PitchFactor: 1, EnergyFactor: 1,
	}}
	return result
}

func pcmDurationMS(pcm *audio.PCM) float64 {
	if pcm == nil || pcm.SampleRate <= 0 || pcm.Channels <= 0 {
		return 0
	}
	return float64(len(pcm.Data)/pcm.Channels) * 1000 / float64(pcm.SampleRate)
}

func relative(root, path string) string {
	value, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(value)
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func writeHTML(path string, value report) error {
	const page = `<!doctype html>
<html lang="ja"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>UtauTTS 品質上限実験</title>
<style>body{font-family:system-ui,sans-serif;max-width:1100px;margin:24px auto;padding:0 16px;color:#222}article{border:1px solid #ccc;border-radius:10px;padding:16px;margin:18px 0}table{border-collapse:collapse;width:100%}th,td{padding:8px;border-bottom:1px solid #ddd;text-align:left}audio{width:100%;min-width:240px}.meta{color:#555;font-size:.9em}code{overflow-wrap:anywhere}</style>
</head><body><h1>UtauTTS 品質上限実験</h1>
<p>{{.Voicebank}} — 目標{{printf "%.0f" .MoraDurationMS}} ms、母音下限{{printf "%.0f" .MinimumVowelMS}} ms</p>
<ol><li>原録音: 連続WAVの該当範囲</li><li>原間隔再構成: waveform rendererを録音時の発音間隔で使用</li><li>現行waveform: 各原音を目標モーラ長へ短縮して接続</li><li>区間別anchor: 区間ごとにWSOLAを再初期化する初回方式</li><li>連続anchor: 元WAVを分割せず、anchorに沿って読み取り速度を変える方式</li>{{if .ClassicFaithful}}<li>OpenUTAU Classic faithful: 同じunit列をWorldlineと5点envelopeで合成</li><li>一括Worldline: 連続anchorの完成波形を長さを変えず1回だけ再合成</li>{{end}}</ol>
{{range .Cases}}<article><h2>{{.ID}}: {{range $i,$a := .Aliases}}{{if $i}} / {{end}}{{$a}}{{end}}</h2>
<p class="meta"><code>{{.Source}}</code><br>原録音 {{printf "%.0f" .OriginalDurationMS}} ms / 現行 {{printf "%.0f" .CurrentDurationMS}} ms / 区間別anchor {{printf "%.0f" .AnchoredDurationMS}} ms / 連続anchor {{printf "%.0f" .ContinuousDurationMS}} ms{{if .ClassicFaithfulWAV}} / Classic faithful {{printf "%.0f" .ClassicDurationMS}} ms{{end}}</p>
<table><tr><th>条件</th><th>音声</th></tr>
<tr><td>1. 原録音</td><td><audio controls preload="none" src="{{.OriginalWAV}}"></audio></td></tr>
<tr><td>2. 原間隔再構成</td><td><audio controls preload="none" src="{{.ReconstructedWAV}}"></audio></td></tr>
<tr><td>3. 現行waveform</td><td><audio controls preload="none" src="{{.CurrentWAV}}"></audio></td></tr>
<tr><td>4. 区間別anchor</td><td><audio controls preload="none" src="{{.AnchoredWAV}}"></audio></td></tr>
<tr><td>5. 連続anchor</td><td><audio controls preload="none" src="{{.ContinuousWAV}}"></audio></td></tr>
{{if .ClassicFaithfulWAV}}<tr><td>6. OpenUTAU Classic faithful</td><td><audio controls preload="none" src="{{.ClassicFaithfulWAV}}"></audio></td></tr>
<tr><td>7. 連続anchor＋一括Worldline（長さ固定）</td><td><audio controls preload="none" src="{{.WholeWorldlineWAV}}"></audio></td></tr>{{end}}</table></article>{{end}}
</body></html>`
	tmpl, err := template.New("page").Parse(page)
	if err != nil {
		return err
	}
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	return tmpl.Execute(file, value)
}
