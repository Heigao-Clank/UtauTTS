package voicebank

import (
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
