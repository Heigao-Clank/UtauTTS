package frontend

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"github.com/ikawaha/kagome-dict/ipa"
	"github.com/ikawaha/kagome/v2/tokenizer"
)

var (
	japaneseOnce      sync.Once
	japaneseTokenizer *tokenizer.Tokenizer
	japaneseError     error
)

func ToKana(text string) (string, error) {
	return ToKanaWithDictionary(text, nil)
}

func ToKanaWithDictionary(text string, dictionary map[string]string) (string, error) {
	return toKana(ApplyDictionary(text, dictionary))
}

func ApplyDictionary(text string, dictionary map[string]string) string {
	if text == "" || len(dictionary) == 0 {
		return text
	}
	type replacement struct {
		surface string
		reading string
	}
	replacements := make([]replacement, 0, len(dictionary))
	for surface, reading := range dictionary {
		if strings.TrimSpace(surface) == "" || strings.TrimSpace(reading) == "" {
			continue
		}
		replacements = append(replacements, replacement{surface: surface, reading: reading})
	}
	if len(replacements) == 0 {
		return text
	}
	sort.Slice(replacements, func(i, j int) bool {
		if len(replacements[i].surface) != len(replacements[j].surface) {
			return len(replacements[i].surface) > len(replacements[j].surface)
		}
		return replacements[i].surface < replacements[j].surface
	})

	var result strings.Builder
	result.Grow(len(text))
	for index := 0; index < len(text); {
		matched := false
		for _, item := range replacements {
			if !strings.HasPrefix(text[index:], item.surface) {
				continue
			}
			result.WriteString(item.reading)
			index += len(item.surface)
			matched = true
			break
		}
		if matched {
			continue
		}
		_, size := utf8.DecodeRuneInString(text[index:])
		if size == 0 {
			size = 1
		}
		result.WriteString(text[index : index+size])
		index += size
	}
	return result.String()
}

func toKana(text string) (string, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", fmt.Errorf("empty text")
	}
	japaneseOnce.Do(func() {
		japaneseTokenizer, japaneseError = tokenizer.New(ipa.Dict(), tokenizer.OmitBosEos())
	})
	if japaneseError != nil {
		return "", japaneseError
	}

	var reading strings.Builder
	for _, token := range japaneseTokenizer.Tokenize(text) {
		if pronunciation, ok := token.Pronunciation(); ok && pronunciation != "" && pronunciation != "*" {
			reading.WriteString(pronunciation)
			continue
		}
		if safeSurface(token.Surface) {
			reading.WriteString(token.Surface)
			continue
		}
		return "", fmt.Errorf("no pronunciation for token %q", token.Surface)
	}
	if reading.Len() == 0 {
		return "", fmt.Errorf("text produced an empty reading")
	}
	return reading.String(), nil
}

func safeSurface(surface string) bool {
	for _, r := range surface {
		if unicode.IsSpace(r) || isKana(r) || strings.ContainsRune("、。，．,.!?！？・", r) {
			continue
		}
		return false
	}
	return true
}
