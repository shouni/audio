package phonetic

import (
	"strings"

	"github.com/ikawaha/kagome-dict/ipa"
	"github.com/ikawaha/kagome/v2/tokenizer"
)

// Converter は日本語を音声合成に適した読み（カタカナ）に変換します。
type Converter struct {
	t *tokenizer.Tokenizer
}

// NewConverter は新しい Converter を生成します。
func NewConverter() (*Converter, error) {
	t, err := tokenizer.New(ipa.Dict(), tokenizer.OmitBosEos())
	if err != nil {
		return nil, err
	}
	return &Converter{t: t}, nil
}

// ConvertToReading は助詞補正を行い、読みを返します。
func (c *Converter) ConvertToReading(input string) string {
	tokens := c.t.Tokenize(input)
	var sb strings.Builder
	sb.Grow(len(input) * 2)

	for _, token := range tokens {
		sb.WriteString(tokenReading(token))
	}

	return correctPronunciation(sb.String())
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

// correctPronunciation はトークン結合後の読みを発音向けに補正します。
func correctPronunciation(reading string) string {
	reading = strings.ReplaceAll(reading, "コンニチハ", "コンニチワ")
	reading = strings.ReplaceAll(reading, "コンバンハ", "コンバンワ")
	return reading
}
