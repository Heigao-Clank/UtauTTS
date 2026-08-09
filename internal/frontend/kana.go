package frontend

import (
	"fmt"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

type Mora struct {
	Text  string
	Vowel string
	Pause bool
}

func ParseKana(reading string) ([]Mora, error) {
	reading = norm.NFC.String(reading)
	var result []Mora
	for _, r := range reading {
		if unicode.IsSpace(r) || strings.ContainsRune("、。，．,.!?！？・", r) {
			if len(result) > 0 && !result[len(result)-1].Pause {
				result = append(result, Mora{Pause: true})
			}
			continue
		}
		if !isKana(r) {
			return nil, fmt.Errorf("unsupported character %q in kana reading", r)
		}

		hiragana := toHiragana(r)
		if isCombiningSmallKana(hiragana) && len(result) > 0 && !result[len(result)-1].Pause {
			result[len(result)-1].Text += string(hiragana)
			result[len(result)-1].Vowel = vowelOf(hiragana, result[len(result)-1].Vowel)
			continue
		}
		if hiragana == 'ー' {
			if len(result) == 0 || result[len(result)-1].Pause {
				return nil, fmt.Errorf("long vowel mark has no preceding mora")
			}
			result = append(result, Mora{Text: "ー", Vowel: result[len(result)-1].Vowel})
			continue
		}
		result = append(result, Mora{Text: string(hiragana), Vowel: vowelOf(hiragana, "")})
	}
	return result, nil
}

func isKana(r rune) bool {
	return unicode.Is(unicode.Hiragana, r) || unicode.Is(unicode.Katakana, r) || r == 'ー'
}

func toHiragana(r rune) rune {
	if r >= 'ァ' && r <= 'ヶ' {
		return r - 0x60
	}
	return r
}

func isCombiningSmallKana(r rune) bool {
	return strings.ContainsRune("ぁぃぅぇぉゃゅょゎ", r)
}

func vowelOf(r rune, fallback string) string {
	switch {
	case strings.ContainsRune("あかがさざただなはばぱまやらわぁゃゎ", r):
		return "a"
	case strings.ContainsRune("いきぎしじちぢにひびぴみりゐぃ", r):
		return "i"
	case strings.ContainsRune("うくぐすずつづぬふぶぷむゆるゔぅゅ", r):
		return "u"
	case strings.ContainsRune("えけげせぜてでねへべぺめれゑぇ", r):
		return "e"
	case strings.ContainsRune("おこごそぞとどのほぼぽもよろをぉょ", r):
		return "o"
	case r == 'ん':
		return "n"
	case r == 'っ':
		return "cl"
	default:
		return fallback
	}
}
