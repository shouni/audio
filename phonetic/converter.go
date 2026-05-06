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

// ConvertToReading は「は」→「ワ」などの助詞補正を行い、読みを返します。
func (c *Converter) ConvertToReading(input string) string {
	const (
		posIndex     = 0
		readingIndex = 7
	)
	tokens := c.t.Tokenize(input)
	var sb strings.Builder
	sb.Grow(len(input) * 2)

	for _, token := range tokens {
		features := token.Features()
		if len(features) > readingIndex && features[readingIndex] != "*" {
			reading := features[readingIndex]
			// 助詞の歌唱用補正
			if len(features) > posIndex && features[posIndex] == "助詞" {
				if token.Surface == "は" {
					reading = "ワ"
				} else if token.Surface == "へ" {
					reading = "エ"
				}
			}
			sb.WriteString(reading)
		} else {
			// 未知語、英数字、記号などはそのまま保持
			sb.WriteString(token.Surface)
		}
	}
	return sb.String()
}
