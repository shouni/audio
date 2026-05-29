package phonetic

import (
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/ikawaha/kagome-dict/ipa"
	"github.com/ikawaha/kagome/v2/tokenizer"
)

// Converter は日本語を音声合成に適した読み（カタカナ）に変換します。
type Converter struct {
	t                *tokenizer.Tokenizer
	readingOverrides map[string]string
	overrideKeys     []string
}

// Option は Converter の動作を調整します。
type Option func(*Converter)

var defaultReadingOverrides = map[string]string{
	"こんにちは": "コンニチワ",
	"こんばんは": "コンバンワ",
	"夜露死苦":  "ヨロシク",
}

// WithReadingOverrides は、表層形に対する読みの上書きを追加します。
func WithReadingOverrides(overrides map[string]string) Option {
	return func(c *Converter) {
		for surface, reading := range overrides {
			if surface == "" || reading == "" {
				continue
			}
			c.readingOverrides[surface] = reading
		}
		c.rebuildOverrideKeys()
	}
}

// NewConverter は新しい Converter を生成します。
func NewConverter(options ...Option) (*Converter, error) {
	t, err := tokenizer.New(ipa.Dict(), tokenizer.OmitBosEos())
	if err != nil {
		return nil, err
	}
	c := &Converter{
		t:                t,
		readingOverrides: cloneReadingOverrides(defaultReadingOverrides),
	}
	c.rebuildOverrideKeys()
	for _, option := range options {
		option(c)
	}
	return c, nil
}

// ConvertToReading は助詞補正を行い、読みを返します。
func (c *Converter) ConvertToReading(input string) string {
	var sb strings.Builder
	var pending strings.Builder
	sb.Grow(len(input) * 2)
	pending.Grow(len(input))

	for i := 0; i < len(input); {
		if surface, reading, ok := c.matchOverride(input[i:]); ok {
			if pending.Len() > 0 {
				sb.WriteString(c.convertTokenized(pending.String()))
				pending.Reset()
			}
			sb.WriteString(reading)
			i += len(surface)
			continue
		}

		r, size := utf8.DecodeRuneInString(input[i:])
		pending.WriteRune(r)
		i += size
	}

	if pending.Len() > 0 {
		sb.WriteString(c.convertTokenized(pending.String()))
	}

	return sb.String()
}

func (c *Converter) convertTokenized(input string) string {
	tokens := c.t.Tokenize(input)
	var sb strings.Builder
	sb.Grow(len(input) * 2)

	for _, token := range tokens {
		sb.WriteString(tokenReading(token, c.readingOverrides))
	}

	return sb.String()
}

func (c *Converter) matchOverride(input string) (string, string, bool) {
	for _, surface := range c.overrideKeys {
		if strings.HasPrefix(input, surface) {
			return surface, c.readingOverrides[surface], true
		}
	}
	return "", "", false
}

func (c *Converter) rebuildOverrideKeys() {
	c.overrideKeys = c.overrideKeys[:0]
	for surface := range c.readingOverrides {
		c.overrideKeys = append(c.overrideKeys, surface)
	}
	sort.Slice(c.overrideKeys, func(i, j int) bool {
		return len(c.overrideKeys[i]) > len(c.overrideKeys[j])
	})
}

func cloneReadingOverrides(overrides map[string]string) map[string]string {
	cloned := make(map[string]string, len(overrides))
	for surface, reading := range overrides {
		cloned[surface] = reading
	}
	return cloned
}

// tokenReading は1トークンの辞書読みを返し、助詞の発音を補正します。
func tokenReading(token tokenizer.Token, overrides map[string]string) string {
	const (
		posIndex     = 0
		readingIndex = 7
	)

	if reading, ok := overrides[token.Surface]; ok {
		return reading
	}

	features := token.Features()

	reading := token.Surface
	if len(features) > readingIndex && features[readingIndex] != "*" {
		reading = features[readingIndex]
	}

	// 助詞の歌唱用補正
	if len(features) > posIndex && features[posIndex] == "助詞" {
		switch token.Surface {
		case "は":
			reading = "ワ"
		case "へ":
			reading = "エ"
		case "を":
			reading = "オ"
		}
	}

	return reading
}
