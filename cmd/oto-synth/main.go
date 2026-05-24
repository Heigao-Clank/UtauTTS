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
        pitch     float64
    )

    flag.StringVar(&otoPath, "oto", "", "path to oto.ini")
    flag.StringVar(&aliasText, "text", "", "aliases separated by spaces or plain kana")
    flag.StringVar(&outPath, "out", "", "output wav path")
    flag.Float64Var(&gapMs, "gap", 40, "silence gap between samples (ms)")
    flag.Float64Var(&crossMs, "cross", 10, "crossfade length (ms)")
    flag.Float64Var(&pitch, "pitch", 1.0, "pitch scale (1.0 = no change)")
    flag.Parse()

    if otoPath == "" || aliasText == "" || outPath == "" {
        log.Fatal("-oto, -text, -out are required")
    }
    if pitch <= 0 {
        log.Fatal("-pitch must be > 0")
    }

    otoIni, err := oto.ReadIni(otoPath)
    if err != nil {
        log.Fatal(err)
    }

    aliases := splitAliases(aliasText)
    if len(aliases) == 0 {
        log.Fatal("no aliases provided")
    }

    samples, err := loadSamples(otoIni, aliases, pitch)
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

    return splitKanaAliases(text)
}

func isIgnorableRune(r rune) bool {
    switch r {
    case ' ', '\t', '\n', '\r':
        return true
    default:
        return false
    }
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

func isSmallKana(r rune) bool {
    switch r {
    case 'ゃ', 'ゅ', 'ょ', 'ぁ', 'ぃ', 'ぅ', 'ぇ', 'ぉ', 'ゎ',
        'ャ', 'ュ', 'ョ', 'ァ', 'ィ', 'ゥ', 'ェ', 'ォ', 'ヮ':
        return true
    default:
        return false
    }
}

func loadSamples(otoIni *oto.Ini, aliases []string, pitch float64) ([]sample, error) {
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
        trimmed, err := audio.TrimPCM(pcm, entry.Offset, entry.Fixed, entry.Blank)
        if err != nil {
            return nil, fmt.Errorf("trim wav for %s: %w", alias, err)
        }
        if pitch != 1.0 {
            trimmed = pitchShift(trimmed, pitch)
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
    noteStart := 0
    for _, item := range samples {
        data := applyEnvelope(item.pcm.Data, sampleRate, channels, 5, 8)

        preutterFrames := msToFrames(item.entry.Preutterance, sampleRate)
        overlapFrames := msToFrames(item.entry.Overlap, sampleRate)
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

        noteLength := noteLengthFrames(item.entry, sampleFrames+sampleOffset, sampleRate)
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

    return &audio.PCM{
        SampleRate: pcm.SampleRate,
        Channels:   pcm.Channels,
        Data:       out,
    }
}

func ensureLength(data []int16, length int) []int16 {
    if len(data) >= length {
        return data
    }
    extended := make([]int16, length)
    copy(extended, data)
    return extended
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
