package phonetic

import (
	"slices"
	"strings"
	"unicode/utf8"
)

// prefixIndex は、入力のある位置から始まり形態素境界で終わる最長一致を引く索引です。
//
// キーを先頭ルーンごとにまとめ、各グループを長い順で保持します。トークンごとの照合を
// 全キーの走査ではなく、先頭文字が一致するキーだけに絞るためです。読み上書きと助数詞は
// どちらもこの索引で引きます。
type prefixIndex map[rune][]string

// newPrefixIndex は keys から索引を組み立てます。keys の順序は結果に影響しません。
func newPrefixIndex(keys []string) prefixIndex {
	sorted := slices.Clone(keys)
	slices.SortFunc(sorted, func(a, b string) int {
		if diff := len(b) - len(a); diff != 0 {
			return diff
		}
		return strings.Compare(a, b)
	})

	index := make(prefixIndex)
	for _, key := range sorted {
		first, _ := utf8.DecodeRuneInString(key)
		index[first] = append(index[first], key)
	}
	return index
}

// match は、input の start から始まり boundaries のいずれかで終わる最長のキーを返します。
//
// 境界で終わる一致だけを採るのは、境界をまたぐ一致まで拾うと、キーが覆い切れなかった
// 残りの文字が欠落するためです（例: 「世界観」に対する「世界」）。
func (idx prefixIndex) match(input string, start int, boundaries map[int]struct{}) (string, bool) {
	if start >= len(input) {
		return "", false
	}
	rest := input[start:]
	first, _ := utf8.DecodeRuneInString(rest)
	for _, key := range idx[first] {
		if !strings.HasPrefix(rest, key) {
			continue
		}
		if _, ok := boundaries[start+len(key)]; !ok {
			continue
		}
		return key, true
	}
	return "", false
}
