package ceiling

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"

	"utautts/internal/audio"
	"utautts/internal/oto"
	"utautts/internal/plan"
	"utautts/internal/render"
	"utautts/internal/voicebank"
)

type Sequence struct {
	Source  string      `json:"source"`
	OtoPath string      `json:"oto_path"`
	Entries []oto.Entry `json:"entries"`
}

type Config struct {
	MoraDurationMS float64
	MinimumVowelMS float64
	ReleaseMS      float64
}

type Interval struct {
	Index               int     `json:"index"`
	Alias               string  `json:"alias"`
	SourceDurationMS    float64 `json:"source_duration_ms"`
	TargetDurationMS    float64 `json:"target_duration_ms"`
	SourcePrefixMS      float64 `json:"source_prefix_ms"`
	TargetPrefixMS      float64 `json:"target_prefix_ms"`
	SourceSuffixMS      float64 `json:"source_suffix_ms"`
	TargetSuffixMS      float64 `json:"target_suffix_ms"`
	SourceStableVowelMS float64 `json:"source_stable_vowel_ms"`
	TargetStableVowelMS float64 `json:"target_stable_vowel_ms"`
}

type Result struct {
	Original              *audio.PCM
	ReconstructedOriginal *audio.PCM
	Current               *audio.PCM
	Anchored              *audio.PCM
	ContinuousAnchored    *audio.PCM
	OriginalPlan          *plan.Plan
	CurrentPlan           *plan.Plan
	Intervals             []Interval
	SourceStartMS         float64
	SourceEndMS           float64
	RequestedMoraMS       float64
	AnchoredDurationMS    float64
	ContinuousDurationMS  float64
}

// Discoverは同じWAVに含まれる時系列順のVCV原音を探す。
func Discover(bank *voicebank.Bank, minimumEntries, maximumEntries int) []Sequence {
	if bank == nil {
		return nil
	}
	minimumEntries = max(2, minimumEntries)
	if maximumEntries < minimumEntries {
		maximumEntries = minimumEntries
	}
	type key struct {
		otoPath string
		source  string
	}
	groups := map[key][]oto.Entry{}
	for _, entries := range bank.Entries {
		for _, entry := range entries {
			if voicebank.ClassifyAlias(entry.Alias) != voicebank.AliasVCV {
				continue
			}
			group := key{otoPath: entry.OtoPath, source: entry.Filename}
			groups[group] = append(groups[group], entry)
		}
	}
	var sequences []Sequence
	for group, entries := range groups {
		sort.Slice(entries, func(i, j int) bool {
			if entries[i].Line != entries[j].Line {
				return entries[i].Line < entries[j].Line
			}
			return entries[i].Offset < entries[j].Offset
		})
		var run []oto.Entry
		flush := func() {
			if len(run) >= minimumEntries {
				end := min(len(run), maximumEntries)
				sequences = append(sequences, Sequence{
					Source: group.source, OtoPath: group.otoPath,
					Entries: append([]oto.Entry(nil), run[:end]...),
				})
			}
			run = nil
		}
		for _, entry := range entries {
			if len(run) > 0 {
				previous := run[len(run)-1]
				if entry.Offset <= previous.Offset || sourceAnchorMS(entry) <= sourceAnchorMS(previous) {
					flush()
				}
			}
			run = append(run, entry)
		}
		flush()
	}
	sort.Slice(sequences, func(i, j int) bool {
		if len(sequences[i].Entries) != len(sequences[j].Entries) {
			return len(sequences[i].Entries) > len(sequences[j].Entries)
		}
		if sequences[i].Source != sequences[j].Source {
			return sequences[i].Source < sequences[j].Source
		}
		return sequences[i].Entries[0].Line < sequences[j].Entries[0].Line
	})
	return sequences
}

// Generateは原録音、現行再構成、短縮現行、anchor保持短縮を生成する。
func Generate(sequence Sequence, cfg Config) (*Result, error) {
	if len(sequence.Entries) < 2 {
		return nil, errors.New("sequence needs at least two entries")
	}
	if cfg.MoraDurationMS <= 0 {
		cfg.MoraDurationMS = 140
	}
	if cfg.MinimumVowelMS < 0 {
		return nil, errors.New("minimum vowel duration must not be negative")
	}
	if cfg.MinimumVowelMS == 0 {
		cfg.MinimumVowelMS = 25
	}
	pcm, err := audio.ReadWav(sequence.Source)
	if err != nil {
		return nil, fmt.Errorf("read source: %w", err)
	}
	mono := monoFloats(pcm)
	if len(mono) == 0 {
		return nil, errors.New("source has no samples")
	}
	startMS := math.Max(0, sequence.Entries[0].Offset)
	endMS := usableEndMS(sequence.Entries[len(sequence.Entries)-1], len(mono), pcm.SampleRate)
	startFrame := msToFrames(startMS, pcm.SampleRate)
	endFrame := min(len(mono), msToFrames(endMS, pcm.SampleRate))
	if endFrame <= startFrame {
		return nil, errors.New("invalid source span")
	}
	anchors := make([]int, len(sequence.Entries))
	for index, entry := range sequence.Entries {
		anchors[index] = msToFrames(sourceAnchorMS(entry), pcm.SampleRate) - startFrame
		if anchors[index] < 0 || anchors[index] >= endFrame-startFrame {
			return nil, fmt.Errorf("entry %q anchor is outside source span", entry.Alias)
		}
		if index > 0 && anchors[index] <= anchors[index-1] {
			return nil, fmt.Errorf("entry %q has a non-increasing anchor", entry.Alias)
		}
	}
	originalWave := append([]float64(nil), mono[startFrame:endFrame]...)
	originalStarts := make([]float64, len(anchors))
	originalDurations := make([]float64, len(anchors))
	for index := range anchors {
		originalStarts[index] = framesToMS(anchors[index], pcm.SampleRate)
		if index+1 < len(anchors) {
			originalDurations[index] = framesToMS(anchors[index+1]-anchors[index], pcm.SampleRate)
		} else {
			originalDurations[index] = framesToMS(len(originalWave)-anchors[index], pcm.SampleRate)
		}
	}
	originalPlan := buildPlan(sequence, originalStarts, originalDurations)
	reconstructed, err := render.Render(originalPlan, render.Config{ReleaseMS: cfg.ReleaseMS, ReleaseSet: true})
	if err != nil {
		return nil, fmt.Errorf("render original timing: %w", err)
	}
	currentStarts := make([]float64, len(anchors))
	currentDurations := make([]float64, len(anchors))
	for index := range anchors {
		currentStarts[index] = float64(index) * cfg.MoraDurationMS
		currentDurations[index] = cfg.MoraDurationMS
	}
	currentPlan := buildPlan(sequence, currentStarts, currentDurations)
	current, err := render.Render(currentPlan, render.Config{ReleaseMS: cfg.ReleaseMS, ReleaseSet: true})
	if err != nil {
		return nil, fmt.Errorf("render current timing: %w", err)
	}
	anchoredWave, intervals := anchoredRetime(originalWave, sequence.Entries, anchors, cfg, pcm.SampleRate)
	continuousWave := continuousAnchoredRetime(originalWave, sequence.Entries, anchors, cfg, pcm.SampleRate)
	return &Result{
		Original:              floatsToPCM(originalWave, pcm.SampleRate),
		ReconstructedOriginal: reconstructed,
		Current:               current,
		Anchored:              floatsToPCM(anchoredWave, pcm.SampleRate),
		ContinuousAnchored:    floatsToPCM(continuousWave, pcm.SampleRate),
		OriginalPlan:          originalPlan,
		CurrentPlan:           currentPlan,
		Intervals:             intervals,
		SourceStartMS:         startMS,
		SourceEndMS:           endMS,
		RequestedMoraMS:       cfg.MoraDurationMS,
		AnchoredDurationMS:    framesToMS(len(anchoredWave), pcm.SampleRate),
		ContinuousDurationMS:  framesToMS(len(continuousWave), pcm.SampleRate),
	}, nil
}

type intervalLayout struct {
	start, end                 int
	middleStart, middleEnd     int
	targetPrefix, targetMiddle int
	targetSuffix               int
}

func anchoredLayouts(source []float64, entries []oto.Entry, anchors []int, cfg Config, sampleRate int) ([]intervalLayout, []Interval) {
	minimumVowel := msToFrames(cfg.MinimumVowelMS, sampleRate)
	requested := msToFrames(cfg.MoraDurationMS, sampleRate)
	layouts := make([]intervalLayout, 0, len(entries))
	intervals := make([]Interval, 0, len(entries))
	for index := range entries {
		start := anchors[index]
		end := len(source)
		if index+1 < len(anchors) {
			end = anchors[index+1]
		}
		segment := source[start:end]
		sourcePrefix := msToFrames(math.Max(0, entries[index].Fixed-entries[index].Preutterance), sampleRate)
		sourcePrefix = min(sourcePrefix, len(segment))
		sourceSuffix := 0
		if index+1 < len(entries) {
			sourceSuffix = msToFrames(math.Max(0, entries[index+1].Preutterance-entries[index+1].Overlap), sampleRate)
			sourceSuffix = min(sourceSuffix, len(segment)-sourcePrefix)
		}
		middleStart := sourcePrefix
		middleEnd := len(segment) - sourceSuffix
		if middleEnd < middleStart {
			middleEnd = middleStart
		}
		sourceMiddle := middleEnd - middleStart
		targetMiddle := min(requested, minimumVowel)
		if sourceMiddle == 0 {
			targetMiddle = 0
		} else if sourcePrefix+sourceSuffix == 0 {
			targetMiddle = requested
		}
		protectedBudget := requested - targetMiddle
		targetPrefix, targetSuffix := 0, 0
		if protected := sourcePrefix + sourceSuffix; protected > 0 {
			targetPrefix = int(math.Round(float64(protectedBudget) * float64(sourcePrefix) / float64(protected)))
			targetSuffix = protectedBudget - targetPrefix
		}
		layouts = append(layouts, intervalLayout{
			start: start, end: end, middleStart: start + middleStart, middleEnd: start + middleEnd,
			targetPrefix: targetPrefix, targetMiddle: targetMiddle, targetSuffix: targetSuffix,
		})
		intervals = append(intervals, Interval{
			Index: index, Alias: entries[index].Alias,
			SourceDurationMS: framesToMS(len(segment), sampleRate),
			TargetDurationMS: framesToMS(requested, sampleRate),
			SourcePrefixMS:   framesToMS(sourcePrefix, sampleRate), TargetPrefixMS: framesToMS(targetPrefix, sampleRate),
			SourceSuffixMS: framesToMS(sourceSuffix, sampleRate), TargetSuffixMS: framesToMS(targetSuffix, sampleRate),
			SourceStableVowelMS: framesToMS(sourceMiddle, sampleRate), TargetStableVowelMS: framesToMS(targetMiddle, sampleRate),
		})
	}
	return layouts, intervals
}

func anchoredRetime(source []float64, entries []oto.Entry, anchors []int, cfg Config, sampleRate int) ([]float64, []Interval) {
	layouts, intervals := anchoredLayouts(source, entries, anchors, cfg, sampleRate)
	var result []float64
	for _, layout := range layouts {
		result = append(result, stretchInterior(source[layout.start:layout.middleStart], layout.targetPrefix, sampleRate)...)
		result = append(result, stretchInterior(source[layout.middleStart:layout.middleEnd], layout.targetMiddle, sampleRate)...)
		result = append(result, stretchInterior(source[layout.middleEnd:layout.end], layout.targetSuffix, sampleRate)...)
	}
	return result, intervals
}

func continuousAnchoredRetime(source []float64, entries []oto.Entry, anchors []int, cfg Config, sampleRate int) []float64 {
	layouts, _ := anchoredLayouts(source, entries, anchors, cfg, sampleRate)
	if len(layouts) == 0 {
		return nil
	}
	cropStart := layouts[0].start
	cropped := source[cropStart:layouts[len(layouts)-1].end]
	sourceKnots := []int{0}
	targetKnots := []int{0}
	target := 0
	addKnot := func(sourcePosition, targetPosition int) {
		sourcePosition -= cropStart
		if sourcePosition > sourceKnots[len(sourceKnots)-1] && targetPosition > targetKnots[len(targetKnots)-1] {
			sourceKnots = append(sourceKnots, sourcePosition)
			targetKnots = append(targetKnots, targetPosition)
		}
	}
	for _, layout := range layouts {
		target += layout.targetPrefix
		addKnot(layout.middleStart, target)
		target += layout.targetMiddle
		addKnot(layout.middleEnd, target)
		target += layout.targetSuffix
		addKnot(layout.end, target)
	}
	if sourceKnots[len(sourceKnots)-1] != len(cropped) || targetKnots[len(targetKnots)-1] != target {
		sourceKnots = append(sourceKnots, len(cropped))
		targetKnots = append(targetKnots, target)
	}
	return render.StretchWSOLAAnchored(cropped, target, sampleRate, sourceKnots, targetKnots)
}

func stretchInterior(source []float64, targetFrames, sampleRate int) []float64 {
	if targetFrames <= 0 {
		return nil
	}
	if len(source) == 0 {
		return make([]float64, targetFrames)
	}
	stretched := render.StretchWSOLA(source, targetFrames, sampleRate)
	fade := min(msToFrames(3, sampleRate), len(source)/2, len(stretched)/2)
	for index := 0; index < fade; index++ {
		weight := float64(index+1) / float64(fade+1)
		stretched[index] = source[index]*(1-weight) + stretched[index]*weight
		right := len(stretched) - 1 - index
		sourceRight := len(source) - 1 - index
		stretched[right] = source[sourceRight]*(1-weight) + stretched[right]*weight
	}
	return stretched
}

func buildPlan(sequence Sequence, starts, durations []float64) *plan.Plan {
	units := make([]plan.Unit, len(sequence.Entries))
	durationMS := 0.0
	for index, entry := range sequence.Entries {
		units[index] = plan.Unit{
			Position: index, Role: "mora", Mora: moraForAlias(entry.Alias), Alias: entry.Alias,
			Source: entry.Filename, OtoPath: entry.OtoPath, OtoLine: entry.Line,
			NoteStartMS: starts[index], DurationMS: durations[index], OffsetMS: entry.Offset,
			ConsonantMS: entry.Fixed, CutoffMS: entry.Blank, PreutteranceMS: entry.Preutterance,
			OverlapMS: entry.Overlap, PitchFactor: 1, EnergyFactor: 1,
		}
		durationMS = max(durationMS, starts[index]+durations[index])
	}
	return &plan.Plan{Version: plan.Version, Voicebank: sequence.OtoPath, Reading: aliasesReading(sequence.Entries), DurationMS: durationMS, Units: units}
}

func sourceAnchorMS(entry oto.Entry) float64 {
	return entry.Offset + entry.Preutterance
}

func usableEndMS(entry oto.Entry, frames, sampleRate int) float64 {
	if entry.Blank < 0 {
		return entry.Offset - entry.Blank
	}
	return framesToMS(frames, sampleRate) - entry.Blank
}

func aliasesReading(entries []oto.Entry) string {
	parts := make([]string, len(entries))
	for index, entry := range entries {
		parts[index] = moraForAlias(entry.Alias)
	}
	return strings.Join(parts, "")
}

func moraForAlias(alias string) string {
	parts := strings.Fields(alias)
	if len(parts) == 0 {
		return alias
	}
	return parts[len(parts)-1]
}

func monoFloats(pcm *audio.PCM) []float64 {
	if pcm == nil || pcm.Channels <= 0 {
		return nil
	}
	frames := len(pcm.Data) / pcm.Channels
	result := make([]float64, frames)
	for frame := range result {
		sum := 0.0
		for channel := 0; channel < pcm.Channels; channel++ {
			sum += float64(pcm.Data[frame*pcm.Channels+channel]) / 32768
		}
		result[frame] = sum / float64(pcm.Channels)
	}
	return result
}

func floatsToPCM(values []float64, sampleRate int) *audio.PCM {
	data := make([]int16, len(values))
	for index, value := range values {
		value = math.Max(-1, math.Min(1, value))
		data[index] = int16(math.Round(value * 32767))
	}
	return &audio.PCM{SampleRate: sampleRate, Channels: 1, Data: data}
}

func msToFrames(ms float64, sampleRate int) int {
	return int(math.Round(math.Max(0, ms) * float64(sampleRate) / 1000))
}

func framesToMS(frames, sampleRate int) float64 {
	if sampleRate <= 0 {
		return 0
	}
	return float64(frames) * 1000 / float64(sampleRate)
}
