package voicebank

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"utautts/internal/connection"
	"utautts/internal/frontend"
	"utautts/internal/oto"
)

type Selection struct {
	Position        int
	Mora            frontend.Mora
	Alias           string
	Entry           oto.Entry
	Candidates      []string
	CandidateCount  int
	TargetScore     float64
	JoinScore       float64
	JoinProbability float64
	PathScore       float64
}

const maxCandidatesPerPosition = 32

type SelectionMode string

const (
	SelectionViterbi    SelectionMode = "viterbi"
	SelectionGreedy     SelectionMode = "greedy"
	SelectionTargetOnly SelectionMode = "target-only"
)

type ResolveConfig struct {
	Tone      string
	Mode      SelectionMode
	JoinModel *connection.LearnedModel
}

type MissingAliasError struct {
	Position   int
	Mora       string
	Candidates []string
}

func (e *MissingAliasError) Error() string {
	return fmt.Sprintf("no voicebank entry for mora %q at position %d (tried: %s)", e.Mora, e.Position, strings.Join(e.Candidates, ", "))
}

func (b *Bank) Resolve(morae []frontend.Mora) ([]Selection, error) {
	return b.ResolveWithConfig(morae, ResolveConfig{})
}

func (b *Bank) ResolveAtTone(morae []frontend.Mora, tone string) ([]Selection, error) {
	return b.ResolveWithConfig(morae, ResolveConfig{Tone: tone})
}

func (b *Bank) ResolveWithConfig(morae []frontend.Mora, cfg ResolveConfig) ([]Selection, error) {
	mode := cfg.Mode
	if mode == "" {
		mode = SelectionViterbi
	}
	if mode != SelectionViterbi && mode != SelectionGreedy && mode != SelectionTargetOnly {
		return nil, fmt.Errorf("unknown selection mode %q", mode)
	}
	layers, err := b.candidateLayers(morae, cfg.Tone)
	if err != nil {
		return nil, err
	}
	if b.extractor == nil {
		b.extractor = connection.NewExtractor()
	}
	return selectBestPaths(layers, mode, cfg.JoinModel, b.extractor), nil
}

func (b *Bank) candidateLayers(morae []frontend.Mora, tone string) ([][]Selection, error) {
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

		candidateSpecs := aliasCandidates(mora.Text, previousVowel, phraseStart)
		if hasAffix {
			candidateSpecs = affixCandidates(candidateSpecs, affix)
		}
		candidates := candidateNames(candidateSpecs)
		var candidatesAtPosition []Selection
		for _, candidate := range candidateSpecs {
			entries := b.Entries[candidate.name]
			for _, entry := range entries {
				targetScore := candidateScore(candidate.tier, entry)
				candidatesAtPosition = append(candidatesAtPosition, Selection{
					Position: position, Mora: mora, Alias: candidate.name, Entry: entry,
					Candidates: candidates, TargetScore: targetScore,
				})
			}
		}
		if len(candidatesAtPosition) == 0 {
			if mora.Vowel == "cl" {
				candidatesAtPosition = []Selection{{
					Position: position, Mora: mora, Alias: "<closure>",
					Candidates: candidates, CandidateCount: 1,
					TargetScore: 100,
				}}
				layers = append(layers, candidatesAtPosition)
				previousVowel = mora.Vowel
				phraseStart = false
				continue
			}
			return nil, &MissingAliasError{Position: position, Mora: mora.Text, Candidates: candidates}
		}
		if len(candidatesAtPosition) > maxCandidatesPerPosition {
			sort.SliceStable(candidatesAtPosition, func(i, j int) bool {
				return candidatesAtPosition[i].TargetScore > candidatesAtPosition[j].TargetScore
			})
			candidatesAtPosition = candidatesAtPosition[:maxCandidatesPerPosition]
		}
		for index := range candidatesAtPosition {
			candidatesAtPosition[index].CandidateCount = len(candidatesAtPosition)
		}
		layers = append(layers, candidatesAtPosition)
		previousVowel = mora.Vowel
		phraseStart = false
	}
	return layers, nil
}

// candidateScoreは言語的な候補優先度とoto.iniの基本的な整合性を組み合わせる。
// 主に重複録音の間の選択に使われ、設定が悪いVCVエントリは
// 使用可能なフォールバックに負けることを許す。
func candidateScore(candidateTier int, entry oto.Entry) float64 {
	score := 100 - float64(candidateTier)*10
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

type aliasCandidate struct {
	name string
	tier int
}

func affixCandidates(base []aliasCandidate, affix Affix) []aliasCandidate {
	result := make([]aliasCandidate, 0, len(base)*2)
	for _, candidate := range base {
		result = append(result,
			aliasCandidate{name: affix.Prefix + candidate.name + affix.Suffix, tier: candidate.tier},
			aliasCandidate{name: candidate.name, tier: candidate.tier + 1},
		)
	}
	return uniqueCandidates(result)
}

func aliasCandidates(mora, previousVowel string, phraseStart bool) []aliasCandidate {
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

	var candidates []aliasCandidate
	if phraseStart {
		for _, form := range forms {
			candidates = append(candidates, aliasCandidate{name: "- " + form, tier: 0})
		}
	} else if previousVowel != "" && previousVowel != "cl" {
		for _, form := range forms {
			candidates = append(candidates, aliasCandidate{name: previousVowel + " " + form, tier: 0})
		}
	}
	for _, form := range forms {
		candidates = append(candidates, aliasCandidate{name: form, tier: 1})
	}
	if !phraseStart {
		for _, form := range forms {
			candidates = append(candidates, aliasCandidate{name: "* " + form, tier: 2})
		}
	}
	return uniqueCandidates(candidates)
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

func uniqueCandidates(values []aliasCandidate) []aliasCandidate {
	indices := map[string]int{}
	result := make([]aliasCandidate, 0, len(values))
	for _, value := range values {
		if index, ok := indices[value.name]; ok {
			result[index].tier = min(result[index].tier, value.tier)
		} else {
			indices[value.name] = len(result)
			result = append(result, value)
		}
	}
	return result
}

func candidateNames(candidates []aliasCandidate) []string {
	result := make([]string, len(candidates))
	for index, candidate := range candidates {
		result[index] = candidate.name
	}
	return result
}
