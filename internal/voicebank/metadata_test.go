package voicebank

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/text/encoding/japanese"

	"utautts/internal/frontend"
	"utautts/internal/oto"
)

func TestLoadCharacterNameAndPrefixMap(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "oto.ini"), "a.wav=あC4,0,0,0,0,0\n")
	name, err := japanese.ShiftJIS.NewEncoder().Bytes([]byte("name=テスト音源\n"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "character.txt"), name, 0o644); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(root, "prefix.map"), "C4\t\tC4\nG4\t強\t\n")

	bank, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if bank.Name != "テスト音源" {
		t.Fatalf("name = %q", bank.Name)
	}
	if got := bank.PrefixMap["C4"]; got != (Affix{Suffix: "C4"}) {
		t.Fatalf("C4 affix = %+v", got)
	}
	if got, ok := bank.AffixForTone("F#4"); !ok || got != (Affix{Prefix: "強"}) {
		t.Fatalf("nearest affix = %+v, %v", got, ok)
	}
}

func TestResolveAtToneUsesAffixedAlias(t *testing.T) {
	bank := &Bank{
		Entries:   map[string][]oto.Entry{"あ_C4": {{Alias: "あ_C4"}}},
		PrefixMap: map[string]Affix{"C4": {Suffix: "_C4"}},
	}
	morae, _ := frontend.ParseKana("あ")
	selections, err := bank.ResolveAtTone(morae, "C4")
	if err != nil {
		t.Fatal(err)
	}
	if selections[0].Alias != "あ_C4" {
		t.Fatalf("alias = %q", selections[0].Alias)
	}
}

func TestResolveAtToneUsesAffixedCVVCTransition(t *testing.T) {
	bank := &Bank{
		Entries: map[string][]oto.Entry{
			"あ_C4":   {{Alias: "あ_C4"}},
			"か_C4":   {{Alias: "か_C4"}},
			"a k_C4": {{Alias: "a k_C4"}},
		},
		PrefixMap: map[string]Affix{"C4": {Suffix: "_C4"}},
	}
	morae, _ := frontend.ParseKana("あか")
	selections, err := bank.ResolveAtTone(morae, "C4")
	if err != nil {
		t.Fatal(err)
	}
	if len(selections) != 2 || !selections[1].Composite || selections[1].Transition == nil || selections[1].Transition.Alias != "a k_C4" {
		t.Fatalf("selections = %#v", selections)
	}
}

func TestLoadPrefixMapPreservesEmptyAffixes(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "oto.ini"), "a.wav=a,0,0,0,0,0\n")
	write(t, filepath.Join(root, "prefix.map"), "B3\t\t_B3\r\nC4\t\t\r\nF4\t\t F4\r\n")

	bank, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if got := bank.PrefixMap["B3"]; got != (Affix{Suffix: "_B3"}) {
		t.Fatalf("B3 affix = %+v", got)
	}
	if got, ok := bank.PrefixMap["C4"]; !ok || got != (Affix{}) {
		t.Fatalf("C4 affix = %+v, present = %t", got, ok)
	}
	if got := bank.PrefixMap["F4"]; got != (Affix{Suffix: " F4"}) {
		t.Fatalf("F4 affix = %+v", got)
	}
	if len(bank.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v", bank.Diagnostics)
	}
	if got, ok := bank.AffixForTone("C4"); !ok || got != (Affix{}) {
		t.Fatalf("C4 lookup = %+v, %t", got, ok)
	}
}
