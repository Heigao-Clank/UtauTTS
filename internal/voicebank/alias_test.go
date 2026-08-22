package voicebank

import (
	"strings"
	"testing"

	"utautts/internal/oto"
)

func TestClassifyAlias(t *testing.T) {
	tests := map[string]AliasKind{
		"あ":     AliasCV,
		"きゃ":    AliasCV,
		"- あ":   AliasVCV,
		"a か":   AliasVCV,
		"n だ":   AliasVCV,
		"* あ":   AliasCV,
		"あ k":   AliasVC,
		"a k":   AliasVC,
		"R":     AliasOther,
		"pau":   AliasOther,
		"a b c": AliasOther,
	}
	for alias, want := range tests {
		if got := ClassifyAlias(alias); got != want {
			t.Errorf("ClassifyAlias(%q) = %q, want %q", alias, got, want)
		}
	}
}

func TestAliasPolicyValues(t *testing.T) {
	for _, policy := range []AliasPolicy{AliasPolicyAuto, AliasPolicyVCVPrefer, AliasPolicyCVVCPrefer, AliasPolicyCVOnly} {
		if !policy.valid() {
			t.Errorf("policy %q was rejected", policy)
		}
	}
	if AliasPolicy("invalid").valid() {
		t.Fatal("invalid alias policy was accepted")
	}
}

func TestAliasCapabilitiesSummarizeVCVContexts(t *testing.T) {
	bank := &Bank{Entries: map[string][]oto.Entry{
		"あ":   {{Alias: "あ"}},
		"- あ": {{Alias: "- あ"}},
		"a か": {{Alias: "a か"}},
		"n だ": {{Alias: "n だ"}},
		"あ k": {{Alias: "あ k"}},
	}}
	capabilities := bank.AliasCapabilities()
	if !capabilities.HasVCV || !capabilities.HasInitialVCV || !capabilities.HasNContextVCV {
		t.Fatalf("capabilities = %+v", capabilities)
	}
	if capabilities.Counts[AliasCV] != 1 || capabilities.Counts[AliasVCV] != 3 || capabilities.Counts[AliasVC] != 1 {
		t.Fatalf("counts = %#v", capabilities.Counts)
	}
	if capabilities.VCVContexts["a"] != 1 || capabilities.VCVContexts["n"] != 1 || capabilities.VCVContexts["-"] != 1 {
		t.Fatalf("contexts = %#v", capabilities.VCVContexts)
	}
	if !capabilities.HasVC || capabilities.VCContexts["あ"] != 1 {
		t.Fatalf("vc capabilities = %+v", capabilities)
	}
}

func TestRecommendCVVCEnhancedUsesInventoryBalance(t *testing.T) {
	makeBank := func(vc, vcv int) *Bank {
		bank := &Bank{Entries: map[string][]oto.Entry{}}
		for index := 0; index < vc; index++ {
			bank.Entries[strings.Repeat("あ", index+1)+" k"] = nil
		}
		for index := 0; index < vcv; index++ {
			bank.Entries["a "+strings.Repeat("あ", index+1)] = nil
		}
		return bank
	}
	if !makeBank(180, 190).RecommendCVVCEnhanced() {
		t.Fatal("balanced CVVC inventory was not recognized")
	}
	if makeBank(126, 4736).RecommendCVVCEnhanced() {
		t.Fatal("VCV-dominant inventory was incorrectly recognized as CVVC")
	}
	if makeBank(1, 0).RecommendCVVCEnhanced() {
		t.Fatal("tiny incidental VC inventory was incorrectly recognized as CVVC")
	}
}
