package voicebank

import (
	"strings"
	"unicode"
)

type AliasKind string

const (
	AliasCV    AliasKind = "CV"
	AliasVCV   AliasKind = "VCV"
	AliasVC    AliasKind = "VC"
	AliasOther AliasKind = "other"
)

type AliasPolicy string

const (
	AliasPolicyAuto       AliasPolicy = "auto"
	AliasPolicyLegacy     AliasPolicy = "legacy"
	AliasPolicyEnhanced   AliasPolicy = "cvvc-enhanced"
	AliasPolicyVCVPrefer  AliasPolicy = "vcv-prefer"
	AliasPolicyCVVCPrefer AliasPolicy = "cvvc-prefer"
	AliasPolicyCVOnly     AliasPolicy = "cv-only"
)

func (p AliasPolicy) valid() bool {
	return p == "" || p == AliasPolicyAuto || p == AliasPolicyLegacy || p == AliasPolicyEnhanced || p == AliasPolicyVCVPrefer || p == AliasPolicyCVVCPrefer || p == AliasPolicyCVOnly
}

type AliasCapabilities struct {
	Counts         map[AliasKind]int
	VCVContexts    map[string]int
	VCContexts     map[string]int
	HasVCV         bool
	HasVC          bool
	HasInitialVCV  bool
	HasNContextVCV bool
}

func ClassifyAlias(alias string) AliasKind {
	alias = strings.TrimSpace(alias)
	if alias == "" {
		return AliasOther
	}
	parts := strings.Fields(alias)
	if len(parts) == 1 {
		if containsKana(parts[0]) {
			return AliasCV
		}
		return AliasOther
	}
	if len(parts) != 2 {
		return AliasOther
	}
	if parts[0] == "*" && containsKana(parts[1]) {
		return AliasCV
	}
	if (parts[0] == "-" || isVowelContext(parts[0])) && containsKana(parts[1]) {
		return AliasVCV
	}
	if (containsKana(parts[0]) || isVowelContext(parts[0])) && isConsonantContext(parts[1]) {
		return AliasVC
	}
	return AliasOther
}

func containsKana(value string) bool {
	for _, r := range value {
		if unicode.In(r, unicode.Hiragana, unicode.Katakana) {
			return true
		}
	}
	return false
}

func isVowelContext(value string) bool {
	value = strings.ToLower(value)
	return value == "a" || value == "i" || value == "u" || value == "e" || value == "o" || value == "n"
}

func isConsonantContext(value string) bool {
	if len(value) == 0 || len(value) > 4 {
		return false
	}
	for _, r := range value {
		isLetter := (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z')
		if !isLetter || strings.ContainsRune("aeiouAEIOU", r) {
			return false
		}
	}
	return true
}

func (b *Bank) AliasCounts() map[AliasKind]int {
	return b.AliasCapabilities().Counts
}

// RecommendCVVCEnhancedは、少数の補助VCを持つVCV音源を除外してCVVC向け音源を判定する。
func (b *Bank) RecommendCVVCEnhanced() bool {
	counts := b.AliasCounts()
	vc, vcv := counts[AliasVC], counts[AliasVCV]
	return vc >= 24 && (vcv == 0 || vc*2 >= vcv)
}

func (b *Bank) AliasCapabilities() AliasCapabilities {
	capabilities := AliasCapabilities{
		Counts:      map[AliasKind]int{},
		VCVContexts: map[string]int{},
		VCContexts:  map[string]int{},
	}
	for alias := range b.Entries {
		kind := ClassifyAlias(alias)
		capabilities.Counts[kind]++
		parts := strings.Fields(alias)
		if len(parts) != 2 {
			continue
		}
		switch kind {
		case AliasVCV:
			capabilities.HasVCV = true
			capabilities.VCVContexts[parts[0]]++
			if parts[0] == "-" {
				capabilities.HasInitialVCV = true
			}
			if parts[0] == "n" {
				capabilities.HasNContextVCV = true
			}
		case AliasVC:
			capabilities.HasVC = true
			capabilities.VCContexts[parts[0]]++
		}
	}
	return capabilities
}
