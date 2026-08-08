package voicebank

import "testing"

func TestClassifyAlias(t *testing.T) {
	tests := map[string]AliasKind{
		"あ":     AliasCV,
		"きゃ":    AliasCV,
		"- あ":   AliasVCV,
		"a か":   AliasVCV,
		"n だ":   AliasVCV,
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
