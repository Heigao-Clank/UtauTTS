package frontend

import (
	"fmt"
	"strings"
	"sync"
	"unicode"

	"github.com/ikawaha/kagome-dict/ipa"
	"github.com/ikawaha/kagome/v2/tokenizer"
)

var (
	japaneseOnce      sync.Once
	japaneseTokenizer *tokenizer.Tokenizer
	japaneseError     error
)

// ToKana converts Japanese text to its dictionary pronunciation. Unknown
// non-kana tokens are errors because silently spelling or dropping them would
// make the synthesis plan misleading.
func ToKana(text string) (string, error) {
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
