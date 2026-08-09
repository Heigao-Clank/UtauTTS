package voicebank

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"utautts/internal/frontend"
	"utautts/internal/oto"
)

type Selection struct {
	Position   int
	Mora       frontend.Mora
	Alias      string
	Entry      oto.Entry
	Candidates []string
	Score      float64
}

const maxCandidatesPerPosition = 32

type MissingAliasError struct {
	Position   int
	Mora       string
	Candidates []string
}

func (e *MissingAliasError) Error() string {
	return fmt.Sprintf("no voicebank entry for mora %q at position %d (tried: %s)", e.Mora, e.Position, strings.Join(e.Candidates, ", "))
}

func (b *Bank) Resolve(morae []frontend.Mora) ([]Selection, error) {
	return b.ResolveAtTone(morae, "")
}

func (b *Bank) ResolveAtTone(morae []frontend.Mora, tone string) ([]Selection, error) {
	layers := make([][]Selection, 0, len(morae))
	affix, hasAffix := b.AffixForTone(tone)
	previousVowel := ""
	phraseStart := true
	for position, mora := range morae {
		if mora.Pause {
			layers = append(layers, nil)
			previousVowel = ""
			phraseStart = true
			continue
		}

		candidates := aliasCandidates(mora.Text, previousVowel, phraseStart)
		if hasAffix {
			candidates = affixCandidates(candidates, affix)
		}
		var candidatesAtPosition []Selection
		for candidateIndex, candidate := range candidates {
			entries := b.Entries[candidate]
			for _, entry := range entries {
				candidatesAtPosition = append(candidatesAtPosition, Selection{
					Position: position, Mora: mora, Alias: candidate, Entry: entry,
					Candidates: candidates, Score: candidateScore(candidateIndex, entry),
				})
			}
		}
		if len(candidatesAtPosition) == 0 {
			return nil, &MissingAliasError{Position: position, Mora: mora.Text, Candidates: candidates}
		}
		if len(candidatesAtPosition) > maxCandidatesPerPosition {
			sort.SliceStable(candidatesAtPosition, func(i, j int) bool {
				return candidatesAtPosition[i].Score > candidatesAtPosition[j].Score
			})
			candidatesAtPosition = candidatesAtPosition[:maxCandidatesPerPosition]
		}
		layers = append(layers, candidatesAtPosition)
		previousVowel = mora.Vowel
		phraseStart = false
	}
	return selectBestPaths(layers), nil
}

// candidateScore combines linguistic candidate priority with basic oto.ini
// consistency. It mainly chooses between duplicate recordings, while allowing
// a badly configured VCV entry to lose to a usable fallback.
func candidateScore(candidateIndex int, entry oto.Entry) float64 {
	score := 100 - float64(candidateIndex)*10
	if entry.Preutterance >= 0 {
		score += 4
	} else {
		score -= 30 + math.Abs(entry.Preutterance)
	}
	if entry.Fixed >= entry.Preutterance && entry.Fixed >= 0 {
		score += 4
	} else {
		score -= 20 + math.Abs(entry.Preutterance-entry.Fixed)
	}
	if entry.Overlap <= entry.Preutterance {
		score += 4
	} else {
		score -= 20 + math.Abs(entry.Overlap-entry.Preutterance)
	}
	if entry.Offset >= 0 {
		score += 2
	} else {
		score -= 20
	}
	return score
}

func affixCandidates(base []string, affix Affix) []string {
	result := make([]string, 0, len(base)*2)
	for _, candidate := range base {
		result = append(result, affix.Prefix+candidate+affix.Suffix, candidate)
	}
	return unique(result)
}

func aliasCandidates(mora, previousVowel string, phraseStart bool) []string {
	forms := make([]string, 0, 4)
	if mora == "ー" {
		if vowelKana := map[string]string{"a": "あ", "i": "い", "u": "う", "e": "え", "o": "お"}[previousVowel]; vowelKana != "" {
			forms = append(forms, vowelKana, toKatakana(vowelKana))
		}
	}
	forms = append(forms, mora)
	katakana := toKatakana(mora)
	if katakana != mora {
		forms = append(forms, katakana)
	}

	var candidates []string
	if phraseStart {
		for _, form := range forms {
			candidates = append(candidates, "- "+form)
		}
	} else if previousVowel != "" && previousVowel != "cl" {
		for _, form := range forms {
			candidates = append(candidates, previousVowel+" "+form)
		}
	}
	for _, form := range forms {
		candidates = append(candidates, form)
	}
	if !phraseStart {
		for _, form := range forms {
			candidates = append(candidates, "* "+form)
		}
	}
	return unique(candidates)
}

func toKatakana(value string) string {
	var result strings.Builder
	for _, r := range value {
		if r >= 'ぁ' && r <= 'ゖ' {
			r += 0x60
		}
		result.WriteRune(r)
	}
	return result.String()
}

func unique(values []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}
