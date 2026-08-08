package frontend

import (
	"reflect"
	"testing"
)

func TestParseKana(t *testing.T) {
	got, err := ParseKana("コンニチハ、きょう。")
	if err != nil {
		t.Fatal(err)
	}
	want := []Mora{
		{Text: "こ", Vowel: "o"},
		{Text: "ん", Vowel: "n"},
		{Text: "に", Vowel: "i"},
		{Text: "ち", Vowel: "i"},
		{Text: "は", Vowel: "a"},
		{Pause: true},
		{Text: "きょ", Vowel: "o"},
		{Text: "う", Vowel: "u"},
		{Pause: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("morae = %#v, want %#v", got, want)
	}
}

func TestParseKanaLongVowel(t *testing.T) {
	got, err := ParseKana("スーパー")
	if err != nil {
		t.Fatal(err)
	}
	if got[1] != (Mora{Text: "ー", Vowel: "u"}) || got[3] != (Mora{Text: "ー", Vowel: "a"}) {
		t.Fatalf("morae = %#v", got)
	}
}

func TestParseKanaRejectsUnknownCharacter(t *testing.T) {
	if _, err := ParseKana("今日は"); err == nil {
		t.Fatal("expected an error")
	}
}
