package frontend

import (
	"strings"
	"testing"
)

func TestToKanaUsesDictionaryPronunciation(t *testing.T) {
	got, err := ToKana("今日はいい天気です。")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, "キョーワ") || !strings.HasSuffix(got, "デス。") {
		t.Fatalf("reading = %q", got)
	}
}

func TestToKanaPreservesKanaAndPunctuation(t *testing.T) {
	got, err := ToKana("こんにちは、テストです。")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "、") || !strings.HasSuffix(got, "。") {
		t.Fatalf("reading = %q", got)
	}
}

func TestToKanaRejectsUnknownLatinToken(t *testing.T) {
	if _, err := ToKana("UtauTTS"); err == nil {
		t.Fatal("expected an unknown-token error")
	}
}
