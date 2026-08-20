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
	Position                  int
	Mora                      frontend.Mora
	Alias                     string
	Kind                      AliasKind
	Composite                 bool
	Transition                *Selection
	FallbackTier              int
	Entry                     oto.Entry
	Candidates                []string
	CandidateCount            int
	TargetScore               float64
	PreferenceScore           float64
	TransitionScore           float64
	JoinScore                 float64
	JoinProbability           float64
	TransitionJoinScore       float64
	TransitionJoinProbability float64
	PathScore                 float64
}

const maxCandidatesPerPosition = 32

type SelectionMode string

const (
	SelectionViterbi    SelectionMode = "viterbi"
	SelectionGreedy     SelectionMode = "greedy"
	SelectionTargetOnly SelectionMode = "target-only"
)

type ResolveConfig struct {
	Tone        string
	Mode        SelectionMode
	AliasPolicy AliasPolicy
	JoinModel   *connection.LearnedModel
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
	policy := cfg.AliasPolicy
	if policy == "" {
		policy = AliasPolicyAuto
	}
	if !policy.valid() {
		return nil, fmt.Errorf("unknown alias policy %q", policy)
	}
	layers, err := b.candidateLayersWithPolicy(morae, cfg.Tone, policy)
	if err != nil {
		return nil, err
	}
	if b.extractor == nil {
		b.extractor = connection.NewExtractor()
	}
	return selectBestPaths(layers, mode, cfg.JoinModel, b.extractor), nil
}

func (b *Bank) candidateLayers(morae []frontend.Mora, tone string) ([][]Selection, error) {
	return b.candidateLayersWithPolicy(morae, tone, AliasPolicyAuto)
}

func (b *Bank) candidateLayersWithPolicy(morae []frontend.Mora, tone string, policy AliasPolicy) ([][]Selection, error) {
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

		candidateSpecs := aliasCandidatesWithPolicy(mora.Text, previousVowel, phraseStart, policy)
		consonant := mora.Consonant
		if consonant == "" {
			consonant = frontend.ConsonantOf(mora.Text)
		}
		transitionSpecs := vcAliasCandidates(previousVowel, consonant, policy)
		if hasAffix {
			candidateSpecs = affixCandidates(candidateSpecs, affix)
			transitionSpecs = affixCandidates(transitionSpecs, affix)
		}
		allSpecs := append(append([]aliasCandidate{}, candidateSpecs...), transitionSpecs...)
		candidates := candidateNames(allSpecs)
		var candidatesAtPosition []Selection
		for _, candidate := range candidateSpecs {
			entries := b.Entries[candidate.name]
			for _, entry := range entries {
				main := Selection{
					Position: position, Mora: mora, Alias: candidate.name, Kind: candidate.kind,
					FallbackTier: candidate.tier, Entry: entry, Candidates: candidates,
					TargetScore: candidateScore(candidate.tier, entry),
				}
				candidatesAtPosition = append(candidatesAtPosition, main)
				if candidate.kind != AliasCV || isWildcardAlias(candidate.name) || len(transitionSpecs) == 0 {
					continue
				}
				for _, transitionSpec := range transitionSpecs {
					for _, transitionEntry := range b.Entries[transitionSpec.name] {
						transition := Selection{
							Position: position, Mora: mora, Alias: transitionSpec.name, Kind: AliasVC,
							FallbackTier: transitionSpec.tier, Entry: transitionEntry, Candidates: candidates,
							TargetScore: candidateScore(transitionSpec.tier, transitionEntry),
						}
						composite := main
						composite.Composite = true
						composite.Transition = &transition
						composite.TransitionScore = transition.TargetScore
						candidatesAtPosition = append(candidatesAtPosition, composite)
					}
				}
			}
		}
		if len(candidatesAtPosition) == 0 {
			if mora.Vowel == "cl" {
				candidatesAtPosition = []Selection{{
					Position: position, Mora: mora, Alias: "<closure>",
					Kind: AliasOther, FallbackTier: 0,
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
		applyCompositePreferences(candidatesAtPosition, policy)
		if len(candidatesAtPosition) > maxCandidatesPerPosition {
			sort.SliceStable(candidatesAtPosition, func(i, j int) bool {
				left := candidatesAtPosition[i].TargetScore + candidatesAtPosition[i].PreferenceScore
				right := candidatesAtPosition[j].TargetScore + candidatesAtPosition[j].PreferenceScore
				return left > right
			})
			candidatesAtPosition = candidatesAtPosition[:maxCandidatesPerPosition]
		}
		for index := range candidatesAtPosition {
			candidatesAtPosition[index].CandidateCount = len(candidatesAtPosition)
			if candidatesAtPosition[index].Transition != nil {
				candidatesAtPosition[index].Transition.CandidateCount = len(candidatesAtPosition)
			}
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
	kind AliasKind
}

func affixCandidates(base []aliasCandidate, affix Affix) []aliasCandidate {
	result := make([]aliasCandidate, 0, len(base)*2)
	for _, candidate := range base {
		result = append(result,
			aliasCandidate{name: affix.Prefix + candidate.name + affix.Suffix, tier: candidate.tier, kind: candidate.kind},
			aliasCandidate{name: candidate.name, tier: candidate.tier + 1, kind: candidate.kind},
		)
	}
	return uniqueCandidates(result)
}

func aliasCandidates(mora, previousVowel string, phraseStart bool) []aliasCandidate {
	return aliasCandidatesWithPolicy(mora, previousVowel, phraseStart, AliasPolicyAuto)
}

func aliasCandidatesWithPolicy(mora, previousVowel string, phraseStart bool, policy AliasPolicy) []aliasCandidate {
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
	allowVCVTarget := mora != "っ"
	if policy != AliasPolicyCVOnly && allowVCVTarget && phraseStart {
		for _, form := range forms {
			candidates = append(candidates, aliasCandidate{name: "- " + form, tier: 0, kind: AliasVCV})
		}
	} else if policy != AliasPolicyCVOnly && allowVCVTarget && previousVowel != "" && previousVowel != "cl" {
		for _, form := range forms {
			candidates = append(candidates, aliasCandidate{name: previousVowel + " " + form, tier: 0, kind: AliasVCV})
		}
	}
	for _, form := range forms {
		candidates = append(candidates, aliasCandidate{name: form, tier: policyTier(policy, 1, AliasCV), kind: AliasCV})
	}
	if policy != AliasPolicyCVOnly && !phraseStart {
		for _, form := range forms {
			candidates = append(candidates, aliasCandidate{name: "* " + form, tier: policyTier(policy, 2, AliasCV), kind: AliasCV})
		}
	}
	return uniqueCandidates(candidates)
}

func vcAliasCandidates(previousVowel, consonant string, policy AliasPolicy) []aliasCandidate {
	if policy == AliasPolicyCVOnly || previousVowel == "" || previousVowel == "cl" || consonant == "" || consonant == "cl" {
		return nil
	}
	contexts := vowelContextForms(previousVowel)
	result := make([]aliasCandidate, 0, len(contexts))
	for _, context := range contexts {
		result = append(result, aliasCandidate{name: context + " " + consonant, tier: vcPolicyTier(policy), kind: AliasVC})
	}
	return uniqueCandidates(result)
}

func vowelContextForms(vowel string) []string {
	forms := []string{vowel}
	if kana := map[string]string{"a": "あ", "i": "い", "u": "う", "e": "え", "o": "お", "n": "ん"}[vowel]; kana != "" {
		forms = append(forms, kana)
	}
	return forms
}

func vcPolicyTier(policy AliasPolicy) int {
	if policy == AliasPolicyVCVPrefer {
		return 2
	}
	return 0
}

func applyCompositePreferences(candidates []Selection, policy AliasPolicy) {
	hasComposite := false
	for _, candidate := range candidates {
		if candidate.Composite {
			hasComposite = true
			break
		}
	}
	if !hasComposite {
		return
	}
	for index := range candidates {
		candidate := &candidates[index]
		if candidate.Composite {
			candidate.PreferenceScore = compositePreferenceScore(policy)
			continue
		}
		if candidate.Kind == AliasVCV && policy != AliasPolicyCVVCPrefer {
			candidate.PreferenceScore = 10
		}
	}
}

func compositePreferenceScore(policy AliasPolicy) float64 {
	switch policy {
	case AliasPolicyVCVPrefer:
		return 22
	case AliasPolicyCVVCPrefer:
		return 12
	default:
		return 12
	}
}

func policyTier(policy AliasPolicy, tier int, kind AliasKind) int {
	if policy == AliasPolicyVCVPrefer && kind != AliasVCV {
		return tier + 2
	}
	if policy == AliasPolicyCVVCPrefer && kind == AliasVCV {
		return tier + 2
	}
	return tier
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
			if result[index].kind == AliasOther {
				result[index].kind = value.kind
			}
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

func isWildcardAlias(alias string) bool {
	parts := strings.Fields(alias)
	return len(parts) >= 2 && strings.Contains(parts[0], "*")
}
