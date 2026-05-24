package main

import (
    "errors"
    "flag"
    "fmt"
    "log"
    "math"
    "os"
    "path/filepath"
    "strings"

    "utautts/internal/audio"
    "utautts/internal/oto"
)

func main() {
    var (
        otoPath   string
        aliasText string
        outPath   string
        gapMs     float64
        crossMs   float64
    )

    flag.StringVar(&otoPath, "oto", "", "path to oto.ini")
    flag.StringVar(&aliasText, "text", "", "aliases separated by spaces or plain kana")
    flag.StringVar(&outPath, "out", "", "output wav path")
    flag.Float64Var(&gapMs, "gap", 40, "silence gap between samples (ms)")
    flag.Float64Var(&crossMs, "cross", 10, "crossfade length (ms)")
    flag.Parse()

    if otoPath == "" || aliasText == "" || outPath == "" {
        log.Fatal("-oto, -text, -out are required")
    }

    otoIni, err := oto.ReadIni(otoPath)
    if err != nil {
        log.Fatal(err)
    }

    aliases := splitAliases(aliasText)
    if len(aliases) == 0 {
        log.Fatal("no aliases provided")
    }

    samples, err := loadSamples(otoIni, aliases)
    if err != nil {
        log.Fatal(err)
    }

    pcm, err := concatSamples(samples, gapMs, crossMs)
    if err != nil {
        log.Fatal(err)
    }

    if err := ensureParentDir(outPath); err != nil {
        log.Fatal(err)
    }
    if err := audio.WriteWav(outPath, pcm); err != nil {
        log.Fatal(err)
    }
    fmt.Printf("wrote %s\n", outPath)
}

type sample struct {
    pcm   *audio.PCM
    name  string
    entry oto.Entry
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

    runes := []rune(text)
    out := make([]string, 0, len(runes))
    for _, r := range runes {
        if !isIgnorableRune(r) {
            out = append(out, string(r))
        }
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

func loadSamples(otoIni *oto.Ini, aliases []string) ([]sample, error) {
    samples := make([]sample, 0, len(aliases))
    for i, alias := range aliases {
        prev := ""
        if i > 0 {
            prev = aliases[i-1]
        }
        entry, err := selectEntry(otoIni, alias, prev, i == 0)
        if err != nil {
            return nil, err
        }
        pcm, err := audio.ReadWav(entry.Filename)
        if err != nil {
            return nil, fmt.Errorf("read wav for %s: %w", alias, err)
        }
        trimmed, err := trimPCM(pcm, entry.Offset, entry.Fixed, entry.Blank)
        if err != nil {
            return nil, fmt.Errorf("trim wav for %s: %w", alias, err)
        }
        samples = append(samples, sample{pcm: trimmed, name: entry.Alias, entry: entry})
    }
    return samples, nil
}

func concatSamples(samples []sample, gapMs float64, crossMs float64) (*audio.PCM, error) {
    if len(samples) == 0 {
        return nil, errors.New("no samples")
    }
    sampleRate := samples[0].pcm.SampleRate
    channels := samples[0].pcm.Channels
    for _, item := range samples[1:] {
        if item.pcm.SampleRate != sampleRate || item.pcm.Channels != channels {
            return nil, errors.New("sample rate or channel mismatch")
        }
    }

    gapFrames := msToFrames(gapMs, sampleRate)
    fallbackOverlapFrames := msToFrames(crossMs, sampleRate)

    var output []int16
    prevEnd := 0
    for _, item := range samples {
        data := applyEnvelope(item.pcm.Data, sampleRate, channels, 5, 8)

        preutterFrames := msToFrames(item.entry.Preutterance, sampleRate)
        overlapFrames := msToFrames(item.entry.Overlap, sampleRate)
        if overlapFrames <= 0 {
            overlapFrames = fallbackOverlapFrames
        }

        noteStart := prevEnd + gapFrames
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

        sampleFrames := len(data) / channels
        if sampleFrames == 0 {
            continue
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
    }

    return &audio.PCM{
        SampleRate: sampleRate,
        Channels:   channels,
        Data:       output,
    }, nil
}

func msToSamples(ms float64, sampleRate int, channels int) int {
    if ms <= 0 {
        return 0
    }
    return int(math.Round((ms / 1000.0) * float64(sampleRate) * float64(channels)))
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

func ensureLength(data []int16, length int) []int16 {
    if len(data) >= length {
        return data
    }
    extended := make([]int16, length)
    copy(extended, data)
    return extended
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

func trimPCM(pcm *audio.PCM, offsetMs float64, fixedMs float64, blankMs float64) (*audio.PCM, error) {
    if pcm.Channels <= 0 {
        return nil, errors.New("invalid channel count")
    }
    frames := len(pcm.Data) / pcm.Channels
    if frames == 0 {
        return nil, errors.New("empty pcm data")
    }

    start := msToFrames(offsetMs, pcm.SampleRate)
    if start < 0 {
        start = 0
    }
    blankFrames := msToFramesAllowNegative(blankMs, pcm.SampleRate)
    end := frames - blankFrames
    if end > frames {
        end = frames
    }
    if end < 0 {
        end = 0
    }
    if fixedMs > 0 {
        fixedFrames := msToFrames(fixedMs, pcm.SampleRate)
        if fixedFrames > 0 && end-start < fixedFrames {
            end = start + fixedFrames
            if end > frames {
                end = frames
            }
        }
    }
    if start >= end {
        return nil, errors.New("invalid trim range")
    }

    startIndex := start * pcm.Channels
    endIndex := end * pcm.Channels
    trimmed := make([]int16, endIndex-startIndex)
    copy(trimmed, pcm.Data[startIndex:endIndex])

    return &audio.PCM{
        SampleRate: pcm.SampleRate,
        Channels:   pcm.Channels,
        Data:       trimmed,
    }, nil
}

func ensureParentDir(path string) error {
    dir := filepath.Dir(path)
    if dir == "." || dir == "" {
        return nil
    }
    if _, err := os.Stat(dir); err == nil {
        return nil
    } else if os.IsNotExist(err) {
        return os.MkdirAll(dir, 0o755)
    } else {
        return err
    }
}
