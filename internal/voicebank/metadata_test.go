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
