package voicebank

import (
	"errors"
	"testing"

	"utautts/internal/frontend"
	"utautts/internal/oto"
)

func TestResolvePrefersVCVAndFallsBackToCV(t *testing.T) {
	bank := &Bank{Entries: map[string][]oto.Entry{
		"- こ": {{Alias: "- こ", Filename: "start.wav"}},
		"o ん": {{Alias: "o ん", Filename: "vcv.wav"}},
		"に":   {{Alias: "に", Filename: "cv.wav"}},
	}}
	morae, err := frontend.ParseKana("こんに")
	if err != nil {
		t.Fatal(err)
	}
	got, err := bank.Resolve(morae)
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Alias != "- こ" || got[1].Alias != "o ん" || got[2].Alias != "に" {
		t.Fatalf("selections = %#v", got)
	}
}

func TestResolveResetsContextAtPause(t *testing.T) {
	bank := &Bank{Entries: map[string][]oto.Entry{
		"- あ": {{Alias: "- あ"}},
		"- い": {{Alias: "- い"}},
	}}
	morae, err := frontend.ParseKana("あ、い")
	if err != nil {
		t.Fatal(err)
	}
	got, err := bank.Resolve(morae)
	if err != nil {
		t.Fatal(err)
	}
	if got[1].Alias != "- い" {
		t.Fatalf("second alias = %q", got[1].Alias)
	}
}

func TestResolveReturnsMissingAlias(t *testing.T) {
	bank := &Bank{Entries: map[string][]oto.Entry{}}
	morae, _ := frontend.ParseKana("あ")
	_, err := bank.Resolve(morae)
	var missing *MissingAliasError
	if !errors.As(err, &missing) || missing.Position != 0 {
		t.Fatalf("error = %v", err)
	}
}

func TestResolveLongMarkUsesPreviousVowel(t *testing.T) {
	bank := &Bank{Entries: map[string][]oto.Entry{
		"す":   {{Alias: "す"}},
		"u う": {{Alias: "u う"}},
	}}
	morae, _ := frontend.ParseKana("スー")
	got, err := bank.Resolve(morae)
	if err != nil {
		t.Fatal(err)
	}
	if got[1].Mora.Text != "ー" || got[1].Alias != "u う" {
		t.Fatalf("long vowel selection = %+v", got[1])
	}
}
