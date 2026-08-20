package voicebank

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"utautts/internal/oto"
)

type Affix struct {
	Prefix string
	Suffix string
}

func (b *Bank) loadMetadata() {
	if path := findRootFile(b.Root, "character.txt"); path != "" {
		if text, err := readMetadata(path); err == nil {
			for _, line := range strings.Split(text, "\n") {
				parts := strings.SplitN(strings.TrimSpace(line), "=", 2)
				if len(parts) == 2 && strings.EqualFold(strings.TrimSpace(parts[0]), "name") {
					if name := strings.TrimSpace(parts[1]); name != "" {
						b.Name = name
					}
				}
			}
		}
	}
	path := findRootFile(b.Root, "prefix.map")
	if path == "" {
		return
	}
	text, err := readMetadata(path)
	if err != nil {
		b.Diagnostics = append(b.Diagnostics, Diagnostic{Path: path, Message: err.Error()})
		return
	}
	for index, line := range strings.Split(text, "\n") {
		line = strings.TrimSuffix(line, "\r")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, ";") {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 2 {
			b.Diagnostics = append(b.Diagnostics, Diagnostic{Path: path, Line: index + 1, Message: "invalid prefix.map line"})
			continue
		}
		tone := strings.ToUpper(strings.TrimSpace(fields[0]))
		affix := Affix{Prefix: fields[1]}
		if len(fields) >= 3 {
			affix.Suffix = fields[2]
		}
		b.PrefixMap[tone] = affix
	}
}

func readMetadata(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	text, _, err := oto.Decode(data)
	return text, err
}

func findRootFile(root, name string) string {
	entries, err := os.ReadDir(root)
	if err != nil {
		return ""
	}
	for _, entry := range entries {
		if !entry.IsDir() && strings.EqualFold(entry.Name(), name) {
			return filepath.Join(root, entry.Name())
		}
	}
	return ""
}

func (b *Bank) AffixForTone(tone string) (Affix, bool) {
	if len(b.PrefixMap) == 0 {
		return Affix{}, false
	}
	tone = strings.ToUpper(strings.TrimSpace(tone))
	if tone == "" {
		tone = "C4"
	}
	if affix, ok := b.PrefixMap[tone]; ok {
		return affix, true
	}
	target, ok := toneNumber(tone)
	if !ok {
		return Affix{}, false
	}
	bestDistance := int(^uint(0) >> 1)
	bestTone := ""
	var best Affix
	found := false
	for candidate, affix := range b.PrefixMap {
		number, valid := toneNumber(candidate)
		if !valid {
			continue
		}
		distance := number - target
		if distance < 0 {
			distance = -distance
		}
		if distance < bestDistance || (distance == bestDistance && (bestTone == "" || candidate < bestTone)) {
			bestDistance = distance
			bestTone = candidate
			best = affix
			found = true
		}
	}
	return best, found
}

func toneNumber(tone string) (int, bool) {
	tone = strings.ToUpper(strings.TrimSpace(tone))
	if len(tone) < 2 {
		return 0, false
	}
	semitones := map[byte]int{'C': 0, 'D': 2, 'E': 4, 'F': 5, 'G': 7, 'A': 9, 'B': 11}
	semitone, ok := semitones[tone[0]]
	if !ok {
		return 0, false
	}
	index := 1
	if index < len(tone) && (tone[index] == '#' || tone[index] == 'B') {
		if tone[index] == '#' {
			semitone++
		} else {
			semitone--
		}
		index++
	}
	octave, err := strconv.Atoi(tone[index:])
	if err != nil {
		return 0, false
	}
	return (octave+1)*12 + semitone, true
}
