// Package phonetic は、日本語テキストを形態素解析し、音声合成に適した読み（カタカナ）へ変換します。
package phonetic

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
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
//
// 生成後に内部状態を変更しないため、1つの Converter を複数のゴルーチンから
// 同時に利用できます（Option による設定は生成時のみ有効です）。
type Converter struct {
	t                *tokenizer.Tokenizer
	readingOverrides map[string]string
	// overrideKeysByFirstRune は、読み上書きのキーを先頭ルーンごとにまとめ、
	// 各グループ内を最長一致用に長い順で保持します。トークンごとの照合を
	// 全キーの走査ではなく先頭文字が一致するキーだけに絞るための索引です。
	overrideKeysByFirstRune map[rune][]string
	phraseSpacing           bool
}

// Option は Converter の生成時に変換動作を調整する関数です。
type Option func(*Converter)

// defaultReadingOverrides は標準で適用する表層形ごとの読み上書きです。
var defaultReadingOverrides = mustLoadReadingOverridesJSON(defaultReadingOverridesJSON)

var particleReadings = map[string]string{
	"は": "ワ",
	"へ": "エ",
	"を": "オ",
}

// WithPhraseSpacing は文節境界（助詞・助動詞の直後）にスペースを挿入する Option を返します。
func WithPhraseSpacing() Option {
	return func(c *Converter) {
		c.phraseSpacing = true
	}
}

// WithReadingOverrides は表層形に対する読みの上書きを追加する Option を返します。
//
// overrides のキーは入力文字列中の表層形、値はその表層形に対応する読みです。
// 空文字のキーまたは値は無視されます。既存の上書きと同じ表層形を指定した場合は、
// 指定した読みで置き換えます。
func WithReadingOverrides(overrides map[string]string) Option {
	return func(c *Converter) {
		for surface, reading := range overrides {
			if !validReadingOverride(surface, reading) {
				continue
			}
			c.readingOverrides[surface] = reading
		}
		c.rebuildOverrideKeys()
	}
}

// WithoutDefaultReadingOverrides は同梱の標準読み上書きを外し、辞書読みだけを
// 使う Option を返します。WithReadingOverrides と併用する場合は、この Option を
// 先に指定してください（Option は指定順に適用されます）。
func WithoutDefaultReadingOverrides() Option {
	return func(c *Converter) {
		c.readingOverrides = map[string]string{}
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
// 変換は input 全体を一度だけ形態素解析し、その結果に読み上書きを重ねる順序で行います。
// 上書きを先に適用して入力を分断すると、残った断片が文脈を失った状態で解析され、
// 品詞が誤判定されます（例: 「運命の閃光が」の「が」は、単独で解析すると助詞ではなく
// 接続詞になり、助詞前提の読み補正と文節スペースが両方とも外れる）。
//
// 読み上書きは形態素の境界で始まり境界で終わる一致だけを採用します。境界をまたぐ一致まで
// 拾うと、上書きが覆い切れなかった残りの文字が欠落します（例: 「世界観」に対する「世界」）。
func (c *Converter) ConvertToReading(input string) string {
	tokens := c.t.Tokenize(input)
	boundaries := tokenBoundaries(input, tokens)

	var sb strings.Builder
	sb.Grow(len(input) * 2)

	for i := 0; i < len(tokens); {
		token := tokens[i]

		surface, reading, ok := c.matchOverrideAt(input, token.Position, boundaries)
		if !ok {
			features := token.Features()
			sb.WriteString(tokenReading(token, features))
			c.writePhraseBreak(&sb, features)
			i++
			continue
		}

		sb.WriteString(reading)
		// 上書きが覆ったトークンをまとめて読み飛ばし、文節スペースは末尾のトークンで判定する。
		end := token.Position + len(surface)
		last := i
		for i < len(tokens) && tokens[i].Position < end {
			last = i
			i++
		}
		c.writePhraseBreak(&sb, tokens[last].Features())
	}

	result := sb.String()
	if c.phraseSpacing {
		result = strings.TrimRight(result, " ")
	}
	return result
}

// writePhraseBreak は、文節境界のトークン直後にスペースを書き込みます。
func (c *Converter) writePhraseBreak(sb *strings.Builder, features []string) {
	if c.phraseSpacing && isPhraseBreak(features) {
		sb.WriteByte(' ')
	}
}

// tokenBoundaries は、各トークンの開始バイト位置と入力末尾から成る境界集合を返します。
func tokenBoundaries(input string, tokens []tokenizer.Token) map[int]struct{} {
	boundaries := make(map[int]struct{}, len(tokens)+1)
	for _, token := range tokens {
		boundaries[token.Position] = struct{}{}
	}
	boundaries[len(input)] = struct{}{}
	return boundaries
}

// matchOverrideAt は、start から始まり形態素境界で終わる最長の読み上書きを返します。
func (c *Converter) matchOverrideAt(input string, start int, boundaries map[int]struct{}) (string, string, bool) {
	rest := input[start:]
	first, _ := utf8.DecodeRuneInString(rest)
	for _, surface := range c.overrideKeysByFirstRune[first] {
		if !strings.HasPrefix(rest, surface) {
			continue
		}
		if _, ok := boundaries[start+len(surface)]; !ok {
			continue
		}
		return surface, c.readingOverrides[surface], true
	}
	return "", "", false
}

// rebuildOverrideKeys は読み上書きのキーを先頭ルーンごとの索引に組み直します。
// 各グループは最長一致用に長い順で保持します。
func (c *Converter) rebuildOverrideKeys() {
	keys := slices.Collect(maps.Keys(c.readingOverrides))
	slices.SortFunc(keys, func(a, b string) int {
		return len(b) - len(a)
	})

	buckets := make(map[rune][]string, len(keys))
	for _, key := range keys {
		first, _ := utf8.DecodeRuneInString(key)
		buckets[first] = append(buckets[first], key)
	}
	c.overrideKeysByFirstRune = buckets
}

// cloneReadingOverrides は読み上書きのマップを複製します。
func cloneReadingOverrides(overrides map[string]string) map[string]string {
	cloned := make(map[string]string, len(overrides))
	maps.Copy(cloned, overrides)
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
		if !validReadingOverride(surface, reading) {
			return nil, fmt.Errorf("invalid reading override: surface and reading must not be empty (surface: %q, reading: %q)", surface, reading)
		}
	}

	return overrides, nil
}

func validReadingOverride(surface, reading string) bool {
	return surface != "" && reading != ""
}

// tokenReading は1トークンの辞書読みを返し、助詞の発音を補正します。
func tokenReading(token tokenizer.Token, features []string) string {
	if corrected, ok := particleReading(token, features); ok {
		return corrected
	}
	return dictionaryReading(token, features)
}

// dictionaryReading は辞書の読みを返します。辞書に読みがない語（未知語や記号）は
// 表層形で代用しますが、出力の契約はカタカナなので、ひらがなだけはカタカナへ正規化します。
func dictionaryReading(token tokenizer.Token, features []string) string {
	const (
		readingIndex = 7
	)

	if len(features) <= readingIndex || features[readingIndex] == "*" {
		return hiraganaToKatakana(token.Surface)
	}
	return features[readingIndex]
}

// hiraganaToKatakana は文字列中のひらがなを対応するカタカナに写します。
// それ以外の文字（カタカナ・漢字・英数字・記号）は変更しません。
func hiraganaToKatakana(s string) string {
	if !strings.ContainsFunc(s, isConvertibleHiragana) {
		return s
	}
	return strings.Map(func(r rune) rune {
		if isConvertibleHiragana(r) {
			return r + hiraganaToKatakanaOffset
		}
		return r
	}, s)
}

// hiraganaToKatakanaOffset は Unicode 上のひらがな・カタカナブロック間の距離です。
const hiraganaToKatakanaOffset = 'ア' - 'あ'

// isConvertibleHiragana は、対応するカタカナが存在するひらがなかを判定します。
// ぁ (U+3041) 〜 ゖ (U+3096) と、繰り返し記号 ゝ ゞ が対象です。
func isConvertibleHiragana(r rune) bool {
	return (r >= 'ぁ' && r <= 'ゖ') || r == 'ゝ' || r == 'ゞ'
}

func isPhraseBreak(features []string) bool {
	if len(features) == 0 {
		return false
	}
	pos := features[0]
	return pos == "助詞" || pos == "助動詞"
}

func particleReading(token tokenizer.Token, features []string) (string, bool) {
	const posIndex = 0

	if len(features) <= posIndex || features[posIndex] != "助詞" {
		return "", false
	}

	reading, ok := particleReadings[token.Surface]
	return reading, ok
}
