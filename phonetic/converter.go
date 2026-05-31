package phonetic

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/ikawaha/kagome-dict/ipa"
	"github.com/ikawaha/kagome/v2/tokenizer"
)

//go:embed reading_overrides.json
var defaultReadingOverridesJSON []byte

// Converter は日本語テキストを音声合成に適した読み（カタカナ）へ変換します。
//
// Converter は形態素解析器の辞書読みを基にしつつ、助詞「は」「へ」「を」の
// 発音補正と、表層形に対する読みの上書きを適用します。
type Converter struct {
	t                *tokenizer.Tokenizer
	readingOverrides map[string]string
	overrideKeys     []string
}

// Option は Converter の生成時に変換動作を調整する関数です。
type Option func(*Converter)

// defaultReadingOverrides は標準で適用する表層形ごとの読み上書きです。
var defaultReadingOverrides = mustLoadReadingOverridesJSON(defaultReadingOverridesJSON)

// WithReadingOverrides は表層形に対する読みの上書きを追加する Option を返します。
//
// overrides のキーは入力文字列中の表層形、値はその表層形に対応する読みです。
// 空文字のキーまたは値は無視されます。既存の上書きと同じ表層形を指定した場合は、
// 指定した読みで置き換えます。
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
//
// options は標準の読み上書きを設定した後に順番に適用されます。
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

// ConvertToReading は input をカタカナの読みに変換します。
//
// 変換時は、まず登録済みの読み上書きを最長一致で適用し、残った部分を形態素解析して
// 辞書読みと助詞補正を適用します。上書き読みが登録されている表層形は、形態素解析の
// 分割結果にかかわらず指定された読みになります。
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

// convertTokenized は input を形態素解析し、各トークンの読みを連結して返します。
func (c *Converter) convertTokenized(input string) string {
	tokens := c.t.Tokenize(input)
	var sb strings.Builder
	sb.Grow(len(input) * 2)

	for _, token := range tokens {
		sb.WriteString(tokenReading(token))
	}

	return sb.String()
}

// matchOverride は input の先頭に一致する読み上書きを最長一致で返します。
func (c *Converter) matchOverride(input string) (string, string, bool) {
	for _, surface := range c.overrideKeys {
		if strings.HasPrefix(input, surface) {
			return surface, c.readingOverrides[surface], true
		}
	}
	return "", "", false
}

// rebuildOverrideKeys は読み上書きのキーを最長一致用の順序に並べ直します。
func (c *Converter) rebuildOverrideKeys() {
	c.overrideKeys = make([]string, 0, len(c.readingOverrides))
	for surface := range c.readingOverrides {
		c.overrideKeys = append(c.overrideKeys, surface)
	}
	sort.Slice(c.overrideKeys, func(i, j int) bool {
		return len(c.overrideKeys[i]) > len(c.overrideKeys[j])
	})
}

// cloneReadingOverrides は読み上書きのマップを複製します。
func cloneReadingOverrides(overrides map[string]string) map[string]string {
	cloned := make(map[string]string, len(overrides))
	for surface, reading := range overrides {
		cloned[surface] = reading
	}
	return cloned
}

// mustLoadReadingOverridesJSON は同梱JSONから読み上書きを読み込みます。
func mustLoadReadingOverridesJSON(data []byte) map[string]string {
	overrides, err := loadReadingOverridesJSON(data)
	if err != nil {
		panic(err)
	}
	return overrides
}

// loadReadingOverridesJSON は表層形をキー、読みを値にしたJSONを読み込みます。
func loadReadingOverridesJSON(data []byte) (map[string]string, error) {
	var overrides map[string]string
	if err := json.Unmarshal(data, &overrides); err != nil {
		return nil, fmt.Errorf("load reading overrides: %w", err)
	}

	for surface, reading := range overrides {
		if surface == "" || reading == "" {
			return nil, fmt.Errorf("invalid reading override: surface and reading must not be empty (surface: %q, reading: %q)", surface, reading)
		}
	}

	return overrides, nil
}

// tokenReading は1トークンの辞書読みを返し、助詞の発音を補正します。
func tokenReading(token tokenizer.Token) string {
	const (
		posIndex     = 0
		readingIndex = 7
	)

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
