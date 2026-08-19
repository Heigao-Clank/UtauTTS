package voicebank

import "sort"

// DefaultSortedKeyはmの辞書順で最初のキーを返す。空の場合は空文字を返す。
// リクエストで明示的なidが省略されたとき、サーバとネイティブエンジンは
// どちらも決定的な「最初のvoicebank」へフォールバックする。このヘルパーを共有し、
// その規約を一箇所に保つ。
func DefaultSortedKey[V any](m map[string]V) string {
	if len(m) == 0 {
		return ""
	}
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys[0]
}
