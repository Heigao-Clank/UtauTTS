package synth

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"

	"utautts/internal/audio"
	"utautts/internal/oto"
)

type Config struct {
    OtoPath  string
    Text     string
    GapMs    float64
    CrossMs  float64
    Pitch    float64
    PredPath string
    PlanPath string
    NoCurve  bool
}

type Sample struct {
    PCM   *audio.PCM
    Name  string
    Entry oto.Entry
    Pred  PredictedParams
}

type PredictedParams struct {
    Preutterance float64
    Overlap      float64
    Duration     float64
    RMSMean      float64
    F0Curve      []float64
    RMSCurve     []float64
}

func Synthesize(cfg Config) (*audio.PCM, error) {
    if cfg.PlanPath != "" {
        return synthesizeFromPlan(cfg)
    }

    otoIni, err := oto.ReadIni(cfg.OtoPath)
    if err != nil {
        return nil, err
    }

    aliases := splitAliases(cfg.Text)
    if len(aliases) == 0 {
        return nil, errors.New("no aliases provided")
    }

    predMap := map[string]PredictedParams{}
    if cfg.PredPath != "" {
        predMap, err = loadPredictions(cfg.PredPath)
        if err != nil {
            return nil, err
        }
    }

    samples, err := loadSamples(otoIni, aliases, cfg.Pitch, predMap, cfg.NoCurve)
    if err != nil {
        return nil, err
    }

    return concatSamples(samples, cfg.GapMs, cfg.CrossMs)
}

func splitAliases(text string) []string {
    text = strings.TrimSpace(text)
    if text == "" {
        return nil
    }
    if strings.Contains(text, " ") || strings.Contains(text, "\t") || strings.Contains(text, "\n") {
        fields := strings.Fields(text)
        out := make([]string, 0, len(fields))
        for _, field := range fields {
            if field != "" {
                out = append(out, field)
            }
        }
        return out
    }
    return splitKanaAliases(text)
}

func splitKanaAliases(text string) []string {
    runes := []rune(text)
    out := make([]string, 0, len(runes))
    for i := 0; i < len(runes); i++ {
        r := runes[i]
        if isIgnorableRune(r) {
            continue
        }
        if i+1 < len(runes) && isSmallKana(runes[i+1]) {
            out = append(out, string([]rune{r, runes[i+1]}))
            i++
            continue
        }
        out = append(out, string(r))
    }
    return out
}

func isIgnorableRune(r rune) bool {
    switch r {
    case ' ', '\t', '\n', '\r':
        return true
    default:
        return false
    }
}

func isSmallKana(r rune) bool {
    switch r {
    case 'ゃ', 'ゅ', 'ょ', 'ぁ', 'ぃ', 'ぅ', 'ぇ', 'ぉ', 'ゎ',
        'ャ', 'ュ', 'ョ', 'ァ', 'ィ', 'ゥ', 'ェ', 'ォ', 'ヮ':
        return true
    default:
        return false
    }
}

func loadSamples(otoIni *oto.Ini, aliases []string, pitch float64, predMap map[string]PredictedParams, noCurve bool) ([]Sample, error) {
    samples := make([]Sample, 0, len(aliases))
    for i, alias := range aliases {
        prev := ""
        if i > 0 {
            prev = aliases[i-1]
        }
        entry, err := selectEntry(otoIni, alias, prev, i == 0)
        if err != nil {
            return nil, err
        }
        pred := findPred(entry, predMap)
        entry = applyPredictions(entry, pred)
        pcm, err := audio.ReadWav(entry.Filename)
        if err != nil {
            return nil, fmt.Errorf("read wav for %s: %w", alias, err)
        }
        trimmed, err := audio.TrimPCM(pcm, entry.Offset, entry.Fixed, entry.Blank)
        if err != nil {
            return nil, fmt.Errorf("trim wav for %s: %w", alias, err)
        }
        if pitch != 1.0 {
            trimmed = pitchShift(trimmed, pitch)
        }
        if !noCurve {
            if len(pred.F0Curve) > 0 {
                trimmed = applyF0Curve(trimmed, pred.F0Curve)
            }
            if pred.RMSMean > 0 {
                trimmed = scaleToRMSLimit(trimmed, pred.RMSMean, 0.5, 2.0)
            }
            if len(pred.RMSCurve) > 0 {
                trimmed = applyRMSCurve(trimmed, pred.RMSCurve)
            }
        }
        samples = append(samples, Sample{PCM: trimmed, Name: entry.Alias, Entry: entry, Pred: pred})
    }
    return samples, nil
}

func selectEntry(otoIni *oto.Ini, alias string, prev string, isFirst bool) (oto.Entry, error) {
    alias = strings.TrimSpace(alias)
    if alias == "" {
        return oto.Entry{}, errors.New("empty alias")
    }
    if strings.HasPrefix(alias, "- ") || strings.HasPrefix(alias, "* ") {
        entry, ok := firstEntry(otoIni, alias)
        if ok {
            return entry, nil
        }
        return oto.Entry{}, fmt.Errorf("alias not found: %s", alias)
    }
    if entry, ok := firstEntry(otoIni, alias); ok {
        return entry, nil
    }
    if !isFirst {
        if isVowelEnding(prev) {
            if entry, ok := firstEntry(otoIni, "- "+alias); ok {
                return entry, nil
            }
        } else {
            if entry, ok := firstEntry(otoIni, "* "+alias); ok {
                return entry, nil
            }
        }
    }
    if entry, ok := firstEntry(otoIni, "- "+alias); ok {
        return entry, nil
    }
    if entry, ok := firstEntry(otoIni, "* "+alias); ok {
        return entry, nil
    }
    return oto.Entry{}, fmt.Errorf("alias not found: %s", alias)
}

func firstEntry(otoIni *oto.Ini, alias string) (oto.Entry, bool) {
    entries := otoIni.Entries[alias]
    if len(entries) == 0 {
        return oto.Entry{}, false
    }
    return entries[0], true
}

func isVowelEnding(alias string) bool {
    if alias == "" {
        return false
    }
    trimmed := strings.TrimSpace(alias)
    if trimmed == "" {
        return false
    }
    if strings.HasSuffix(trimmed, "ー") {
        return true
    }
    runes := []rune(trimmed)
    last := runes[len(runes)-1]
    if last == 'ん' || last == 'ン' {
        return false
    }
    switch last {
    case 'あ', 'い', 'う', 'え', 'お', 'ぁ', 'ぃ', 'ぅ', 'ぇ', 'ぉ',
        'ア', 'イ', 'ウ', 'エ', 'オ', 'ァ', 'ィ', 'ゥ', 'ェ', 'ォ':
        return true
    case 'ゃ', 'ゅ', 'ょ', 'ャ', 'ュ', 'ョ':
        if len(runes) < 2 {
            return false
        }
        prev := runes[len(runes)-2]
        switch prev {
        case 'き', 'ぎ', 'し', 'じ', 'ち', 'ぢ', 'に', 'ひ', 'び', 'ぴ', 'み', 'り',
            'キ', 'ギ', 'シ', 'ジ', 'チ', 'ヂ', 'ニ', 'ヒ', 'ビ', 'ピ', 'ミ', 'リ':
            return true
        default:
            return false
        }
    default:
        return false
    }
}

func loadPredictions(path string) (map[string]PredictedParams, error) {
    file, err := os.Open(path)
    if err != nil {
        return nil, err
    }
    defer file.Close()

    predMap := map[string]PredictedParams{}
    scanner := bufio.NewScanner(file)
    for scanner.Scan() {
        line := strings.TrimSpace(scanner.Text())
        if line == "" {
            continue
        }
        var record struct {
            Source    string `json:"source"`
            Trimmed   string `json:"trimmed"`
            Predicted struct {
                Preutterance float64   `json:"preutterance_ms"`
                Overlap      float64   `json:"overlap_ms"`
                Duration     float64   `json:"duration_ms"`
                RMSMean      float64   `json:"rms_mean"`
                F0Curve      []float64 `json:"f0_curve"`
                RMSCurve     []float64 `json:"rms_curve"`
            } `json:"predicted"`
        }
        if err := json.Unmarshal([]byte(line), &record); err != nil {
            return nil, err
        }
        if record.Trimmed == "" && record.Source == "" {
            continue
        }
        pred := PredictedParams{
            Preutterance: record.Predicted.Preutterance,
            Overlap:      record.Predicted.Overlap,
            Duration:     record.Predicted.Duration,
            RMSMean:      record.Predicted.RMSMean,
            F0Curve:      record.Predicted.F0Curve,
            RMSCurve:     record.Predicted.RMSCurve,
        }
        if record.Trimmed != "" {
            predMap[record.Trimmed] = pred
        }
        if record.Source != "" {
            predMap[filepath.Base(record.Source)] = pred
        }
    }
    if err := scanner.Err(); err != nil {
        return nil, err
    }
    return predMap, nil
}

func findPred(entry oto.Entry, predMap map[string]PredictedParams) PredictedParams {
    if len(predMap) == 0 {
        return PredictedParams{}
    }
    trimmed := filepath.Base(entry.Filename)
    if pred, ok := predMap[trimmed]; ok {
        return pred
    }
    return PredictedParams{}
}

func applyPredictions(entry oto.Entry, pred PredictedParams) oto.Entry {
    if pred.Preutterance > 0 {
        entry.Preutterance = pred.Preutterance
    }
    if pred.Overlap > 0 {
        entry.Overlap = pred.Overlap
    }
    if pred.Duration > 0 {
        entry.Blank = 0
        entry.Fixed = pred.Duration
    }
    return entry
}

func concatSamples(samples []Sample, gapMs float64, crossMs float64) (*audio.PCM, error) {
    if len(samples) == 0 {
        return nil, errors.New("no samples")
    }
    sampleRate := samples[0].PCM.SampleRate
    channels := samples[0].PCM.Channels
    for _, item := range samples[1:] {
        if item.PCM.SampleRate != sampleRate || item.PCM.Channels != channels {
            return nil, errors.New("sample rate or channel mismatch")
        }
    }

    gapFrames := msToFrames(gapMs, sampleRate)
    fallbackOverlapFrames := msToFrames(crossMs, sampleRate)

    var output []int16
    prevEnd := 0
    noteStart := 0
    for _, item := range samples {
        data := applyEnvelope(item.PCM.Data, sampleRate, channels, 5, 8)

        preutterFrames := msToFrames(item.Entry.Preutterance, sampleRate)
        overlapFrames := msToFrames(item.Entry.Overlap, sampleRate)
        if overlapFrames <= 0 {
            overlapFrames = fallbackOverlapFrames
        }
        if preutterFrames > 0 && overlapFrames > preutterFrames {
            overlapFrames = preutterFrames
        }

        sampleFrames := len(data) / channels
        if sampleFrames == 0 {
            continue
        }

        sampleStart := noteStart - preutterFrames
        if sampleStart < 0 {
            sampleStart = 0
        }

        actualOverlap := 0
        if sampleStart < prevEnd {
            actualOverlap = prevEnd - sampleStart
            if overlapFrames > 0 && actualOverlap > overlapFrames {
                shift := actualOverlap - overlapFrames
                sampleStart += shift
                actualOverlap = overlapFrames
            }
        }

        sampleOffset := 0
        if sampleStart < 0 {
            sampleOffset = -sampleStart
            sampleStart = 0
        }
        if sampleOffset >= sampleFrames {
            continue
        }

        sampleFrames -= sampleOffset
        endFrame := sampleStart + sampleFrames
        output = ensureLength(output, endFrame*channels)

        if actualOverlap > sampleFrames {
            actualOverlap = sampleFrames
        }

        for f := 0; f < sampleFrames; f++ {
            outFrame := sampleStart + f
            for ch := 0; ch < channels; ch++ {
                outIdx := outFrame*channels + ch
                inIdx := (sampleOffset+f)*channels + ch
                inVal := float64(data[inIdx])
                if f < actualOverlap {
                    t := float64(f+1) / float64(actualOverlap)
                    prevVal := float64(output[outIdx])
                    blended := (1.0-t)*prevVal + t*inVal
                    output[outIdx] = clampInt16(blended)
                } else {
                    output[outIdx] = clampInt16(inVal)
                }
            }
        }

        if endFrame > prevEnd {
            prevEnd = endFrame
        }

        noteLength := noteLengthFrames(item.Entry, sampleFrames+sampleOffset, sampleRate)
        if noteLength < 1 {
            noteLength = 1
        }
        noteStart += noteLength + gapFrames
    }

    return &audio.PCM{
        SampleRate: sampleRate,
        Channels:   channels,
        Data:       output,
    }, nil
}

func noteLengthFrames(entry oto.Entry, totalFrames int, sampleRate int) int {
    fixedFrames := msToFrames(entry.Fixed, sampleRate)
    offsetFrames := msToFrames(entry.Offset, sampleRate)
    blankFrames := msToFramesAllowNegative(entry.Blank, sampleRate)
    effective := totalFrames - offsetFrames - blankFrames
    if effective < fixedFrames {
        effective = fixedFrames
    }
    if effective < 1 {
        effective = 1
    }
    return effective
}

func applyEnvelope(data []int16, sampleRate int, channels int, attackMs float64, releaseMs float64) []int16 {
    frames := len(data) / channels
    if frames == 0 {
        return data
    }
    attackFrames := msToFrames(attackMs, sampleRate)
    releaseFrames := msToFrames(releaseMs, sampleRate)
    if attackFrames < 1 {
        attackFrames = 1
    }
    if releaseFrames < 1 {
        releaseFrames = 1
    }
    maxEnv := frames / 2
    if attackFrames > maxEnv {
        attackFrames = maxEnv
    }
    if releaseFrames > maxEnv {
        releaseFrames = maxEnv
    }
    if attackFrames == 0 && releaseFrames == 0 {
        return data
    }
    out := make([]int16, len(data))
    copy(out, data)
    for f := 0; f < frames; f++ {
        gain := 1.0
        if f < attackFrames {
            gain = float64(f+1) / float64(attackFrames)
        } else if f >= frames-releaseFrames {
            idx := frames - f
            gain = float64(idx) / float64(releaseFrames)
        }
        if gain >= 0.999 {
            continue
        }
        for ch := 0; ch < channels; ch++ {
            idx := f*channels + ch
            out[idx] = clampInt16(float64(out[idx]) * gain)
        }
    }
    return out
}

func pitchShift(pcm *audio.PCM, factor float64) *audio.PCM {
    if factor == 1.0 {
        return pcm
    }
    frames := len(pcm.Data) / pcm.Channels
    if frames == 0 {
        return pcm
    }
    newFrames := int(math.Round(float64(frames) / factor))
    if newFrames < 1 {
        newFrames = 1
    }
    out := make([]int16, newFrames*pcm.Channels)
    for i := 0; i < newFrames; i++ {
        src := float64(i) * factor
        srcIndex := int(math.Floor(src))
        frac := src - float64(srcIndex)
        if srcIndex >= frames-1 {
            srcIndex = frames - 1
            frac = 0
        }
        nextIndex := srcIndex + 1
        if nextIndex >= frames {
            nextIndex = frames - 1
        }
        for ch := 0; ch < pcm.Channels; ch++ {
            a := float64(pcm.Data[srcIndex*pcm.Channels+ch])
            b := float64(pcm.Data[nextIndex*pcm.Channels+ch])
            out[i*pcm.Channels+ch] = clampInt16(a*(1.0-frac) + b*frac)
        }
    }
    return &audio.PCM{SampleRate: pcm.SampleRate, Channels: pcm.Channels, Data: out}
}

func applyF0Curve(pcm *audio.PCM, curve []float64) *audio.PCM {
    if len(curve) == 0 {
        return pcm
    }
    frames := len(pcm.Data) / pcm.Channels
    if frames == 0 {
        return pcm
    }
    mean := 0.0
    count := 0
    for _, v := range curve {
        if v > 0 {
            mean += v
            count++
        }
    }
    if count == 0 {
        return pcm
    }
    mean /= float64(count)
    if mean <= 0 {
        return pcm
    }
    bins := len(curve)
    base := frames / bins
    extra := frames % bins
    out := make([]int16, 0, len(pcm.Data))
    offset := 0
    for i := 0; i < bins; i++ {
        segFrames := base
        if i < extra {
            segFrames++
        }
        if segFrames <= 0 {
            continue
        }
        if offset+segFrames > frames {
            segFrames = frames - offset
        }
        if segFrames <= 0 {
            break
        }
        seg := pcm.Data[offset*pcm.Channels : (offset+segFrames)*pcm.Channels]
        factor := curve[i] / mean
        if factor <= 0 || factor < 0.5 {
            factor = 1.0
        }
        if factor > 2.0 {
            factor = 1.0
        }
        targetFrames := int(math.Round(float64(segFrames) / factor))
        if targetFrames < 1 {
            targetFrames = 1
        }
        shifted := resampleFrames(seg, pcm.Channels, targetFrames)
        restored := resampleFrames(shifted, pcm.Channels, segFrames)
        out = append(out, restored...)
        offset += segFrames
    }
    if len(out) < len(pcm.Data) {
        out = append(out, make([]int16, len(pcm.Data)-len(out))...)
    }
    return &audio.PCM{SampleRate: pcm.SampleRate, Channels: pcm.Channels, Data: out}
}

func resampleFrames(data []int16, channels int, targetFrames int) []int16 {
    if channels <= 0 {
        return data
    }
    frames := len(data) / channels
    if frames == 0 || targetFrames <= 0 {
        return make([]int16, 0)
    }
    if targetFrames == frames {
        out := make([]int16, len(data))
        copy(out, data)
        return out
    }
    out := make([]int16, targetFrames*channels)
    for i := 0; i < targetFrames; i++ {
        pos := float64(i) * float64(frames-1) / float64(targetFrames-1)
        left := int(math.Floor(pos))
        right := left + 1
        if right >= frames {
            right = frames - 1
        }
        frac := pos - float64(left)
        for ch := 0; ch < channels; ch++ {
            a := float64(data[left*channels+ch])
            b := float64(data[right*channels+ch])
            out[i*channels+ch] = clampInt16(a*(1.0-frac) + b*frac)
        }
    }
    return out
}

func scaleToRMSLimit(pcm *audio.PCM, target float64, minGain float64, maxGain float64) *audio.PCM {
    if target <= 0 {
        return pcm
    }
    current := rmsValue(pcm.Data)
    if current <= 0 {
        return pcm
    }
    gain := target / current
    if gain < minGain {
        gain = minGain
    }
    if gain > maxGain {
        gain = maxGain
    }
    if gain == 1.0 {
        return pcm
    }
    out := make([]int16, len(pcm.Data))
    for i, sample := range pcm.Data {
        out[i] = clampInt16(float64(sample) * gain)
    }
    return &audio.PCM{SampleRate: pcm.SampleRate, Channels: pcm.Channels, Data: out}
}

func rmsValue(data []int16) float64 {
    if len(data) == 0 {
        return 0
    }
    sum := 0.0
    for _, v := range data {
        f := float64(v)
        sum += f * f
    }
    mean := sum / float64(len(data))
    return math.Sqrt(mean)
}

func applyRMSCurve(pcm *audio.PCM, curve []float64) *audio.PCM {
    if len(curve) == 0 {
        return pcm
    }
    frames := len(pcm.Data) / pcm.Channels
    if frames == 0 {
        return pcm
    }
    bins := len(curve)
    out := make([]int16, len(pcm.Data))
    copy(out, pcm.Data)
    for f := 0; f < frames; f++ {
        t := float64(f) / float64(frames-1)
        idx := t * float64(bins-1)
        left := int(math.Floor(idx))
        right := left + 1
        if right >= bins {
            right = bins - 1
        }
        frac := idx - float64(left)
        gain := (1.0-frac)*curve[left] + frac*curve[right]
        if gain < 0.25 {
            gain = 0.25
        }
        if gain > 4.0 {
            gain = 4.0
        }
        for ch := 0; ch < pcm.Channels; ch++ {
            i := f*pcm.Channels + ch
            out[i] = clampInt16(float64(out[i]) * gain)
        }
    }
    return &audio.PCM{SampleRate: pcm.SampleRate, Channels: pcm.Channels, Data: out}
}

func scaleToRMS(pcm *audio.PCM, target float64) *audio.PCM {
    return scaleToRMSLimit(pcm, target, 0.25, 4.0)
}

func ensureLength(data []int16, length int) []int16 {
    if len(data) >= length {
        return data
    }
    extended := make([]int16, length)
    copy(extended, data)
    return extended
}

func msToFrames(ms float64, sampleRate int) int {
    if ms <= 0 {
        return 0
    }
    return int(math.Round((ms / 1000.0) * float64(sampleRate)))
}

func msToFramesAllowNegative(ms float64, sampleRate int) int {
    if ms == 0 {
        return 0
    }
    return int(math.Round((ms / 1000.0) * float64(sampleRate)))
}

func clampInt16(value float64) int16 {
    if value > math.MaxInt16 {
        return math.MaxInt16
    }
    if value < math.MinInt16 {
        return math.MinInt16
    }
    return int16(math.Round(value))
}

type PlanEntry struct {
    File            string  `json:"file"`
    Alias           string  `json:"alias"`
    OffsetMs        float64 `json:"offset_ms"`
    FixedMs         float64 `json:"fixed_ms"`
    BlankMs         float64 `json:"blank_ms"`
    PreutteranceMs  float64 `json:"preutterance_ms"`
    OverlapMs       float64 `json:"overlap_ms"`
    TargetDurMs     float64 `json:"target_dur_ms"`
    PitchFactor     float64 `json:"pitch_factor"`
}

func synthesizeFromPlan(cfg Config) (*audio.PCM, error) {
    plan, err := loadPlan(cfg.PlanPath)
    if err != nil {
        return nil, err
    }
    if len(plan) == 0 {
        return nil, errors.New("empty plan")
    }

    sampleRate := 0
    channels := 0
    var samples []Sample

    for _, pe := range plan {
        pcm, err := audio.ReadWav(pe.File)
        if err != nil {
            return nil, fmt.Errorf("read wav %s: %w", pe.File, err)
        }
        if sampleRate == 0 {
            sampleRate = pcm.SampleRate
            channels = pcm.Channels
        }
        if pcm.SampleRate != sampleRate || pcm.Channels != channels {
            return nil, fmt.Errorf("sample rate/channel mismatch in %s", pe.File)
        }

        entry := oto.Entry{
            Filename:     pe.File,
            Alias:        pe.Alias,
            Offset:       pe.OffsetMs,
            Fixed:        pe.FixedMs,
            Blank:        pe.BlankMs,
            Preutterance: pe.PreutteranceMs,
            Overlap:      pe.OverlapMs,
        }

        trimmed, err := audio.TrimPCM(pcm, entry.Offset, entry.Fixed, entry.Blank)
        if err != nil {
            return nil, fmt.Errorf("trim %s: %w", pe.File, err)
        }

        if pe.PitchFactor != 0 && pe.PitchFactor != 1.0 {
            trimmed = pitchShift(trimmed, pe.PitchFactor)
        }

        if cfg.Pitch != 1.0 {
            trimmed = pitchShift(trimmed, cfg.Pitch)
        }

        if pe.TargetDurMs > 0 {
            entry.Fixed = pe.TargetDurMs
            entry.Blank = 0
        }

        samples = append(samples, Sample{
            PCM:   trimmed,
            Name:  pe.Alias,
            Entry: entry,
        })
    }

    return concatSamples(samples, cfg.GapMs, cfg.CrossMs)
}

func loadPlan(path string) ([]PlanEntry, error) {
    file, err := os.Open(path)
    if err != nil {
        return nil, err
    }
    defer file.Close()

    var plan []PlanEntry
    scanner := bufio.NewScanner(file)
    for scanner.Scan() {
        line := strings.TrimSpace(scanner.Text())
        if line == "" {
            continue
        }
        var pe PlanEntry
        if err := json.Unmarshal([]byte(line), &pe); err != nil {
            return nil, err
        }
        plan = append(plan, pe)
    }
    if err := scanner.Err(); err != nil {
        return nil, err
    }
    return plan, nil
}
