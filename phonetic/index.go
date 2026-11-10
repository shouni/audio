package phonetic

import (
	"slices"
	"strings"
	"unicode/utf8"
)

// prefixIndex is an index that looks up the longest match starting at a given position in
// the input and ending on a morpheme boundary.
//
// Keys are grouped by their first rune, each group kept longest first, so that matching a
// token checks only the keys sharing its first character instead of every key. Both the
// reading overrides and the counters are looked up through this index.
type prefixIndex map[rune][]string

// newPrefixIndex builds the index from keys. The order of keys does not affect the result.
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

// match returns the longest key that starts at start in input and ends on one of
// boundaries.
//
// Only matches ending on a boundary are taken, because a match that crosses a boundary
// drops whatever characters the key failed to cover (for example 「世界」 against
// 「世界観」).
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
