package voicebank

import (
	"fmt"
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
}

type MissingAliasError struct {
	Position   int
	Mora       string
	Candidates []string
}

func (e *MissingAliasError) Error() string {
	return fmt.Sprintf("no voicebank entry for mora %q at position %d (tried: %s)", e.Mora, e.Position, strings.Join(e.Candidates, ", "))
}

// Resolve chooses exactly one recording for each non-pause mora. Candidate
// order is stable and favors context-bearing VCV aliases over CV fallbacks.
func (b *Bank) Resolve(morae []frontend.Mora) ([]Selection, error) {
	return b.ResolveAtTone(morae, "")
}

func (b *Bank) ResolveAtTone(morae []frontend.Mora, tone string) ([]Selection, error) {
	selections := make([]Selection, 0, len(morae))
	affix, hasAffix := b.AffixForTone(tone)
	previousVowel := ""
	phraseStart := true
	for position, mora := range morae {
		if mora.Pause {
			previousVowel = ""
			phraseStart = true
			continue
		}

		candidates := aliasCandidates(mora.Text, previousVowel, phraseStart)
		if hasAffix {
			candidates = affixCandidates(candidates, affix)
		}
		var selected *Selection
		for _, candidate := range candidates {
			entries := b.Entries[candidate]
			if len(entries) == 0 {
				continue
			}
			selected = &Selection{
				Position:   position,
				Mora:       mora,
				Alias:      candidate,
				Entry:      entries[0],
				Candidates: candidates,
			}
			break
		}
		if selected == nil {
			return nil, &MissingAliasError{Position: position, Mora: mora.Text, Candidates: candidates}
		}
		selections = append(selections, *selected)
		previousVowel = mora.Vowel
		phraseStart = false
	}
	return selections, nil
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
