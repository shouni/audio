// Package phonetic analyses Japanese text morphologically and converts it into a reading
// (katakana) suited to speech synthesis.
package phonetic

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/ikawaha/kagome-dict/ipa"
	"github.com/ikawaha/kagome/v2/tokenizer"
)

//go:embed reading_overrides.json
var defaultReadingOverridesJSON []byte

// Converter converts Japanese text into a reading (katakana) suited to speech synthesis.
//
// Converter starts from the morphological analyser's dictionary readings, then applies the
// pronunciation correction for the particles 「は」「へ」「を」 and the reading overrides
// keyed by surface form.
//
// It never changes its internal state after construction, so one Converter can be used
// from several goroutines at once (options take effect only at construction).
type Converter struct {
	t                *tokenizer.Tokenizer
	readingOverrides map[string]string
	// overrideIndex looks up the keys of readingOverrides by longest match.
	overrideIndex prefixIndex
	phraseSpacing bool
	numberReading bool
	// optionErr is an error raised while applying an Option. Options return nothing, so
	// the error is parked here and NewConverter returns it.
	optionErr error
}

// Option is a function that adjusts conversion when a Converter is constructed.
type Option func(*Converter)

// defaultReadingOverrides are the per-surface-form reading overrides applied by default.
var defaultReadingOverrides = mustLoadReadingOverridesJSON(defaultReadingOverridesJSON)

// particleReadings are the readings of surface forms that are pronounced differently from
// how they are written when used as particles.
var particleReadings = map[string]string{
	"は": "ワ",
	"へ": "エ",
	"を": "オ",
}

// WithPhraseSpacing returns an Option that inserts a space at phrase boundaries (right
// after a particle or auxiliary verb).
func WithPhraseSpacing() Option {
	return func(c *Converter) {
		c.phraseSpacing = true
	}
}

// WithNumberReading returns an Option that converts Arabic numerals into Japanese readings.
//
// The morphological analyser's dictionary has no readings for Arabic numerals, so by
// default "2026年8月" becomes "2026ネン8ツキ". With this Option it becomes
// "ニセンニジュウロクネンハチガツ": the number is read, and so is the sound change at the
// counter (一回→イッカイ, 三本→サンボン).
//
// The dictionary can read kanji numerals on their own, so they are read here only when a
// counter follows and a sound change is needed ("三本" → サンボン, "二人" → フタリ).
// Counters not directly after a number are unaffected. Runs with a leading 0 ("007") and
// digits after a decimal point are read one character at a time rather than as place
// values.
func WithNumberReading() Option {
	return func(c *Converter) {
		c.numberReading = true
	}
}

// WithReadingOverridesJSON returns an Option that loads reading overrides from JSON and
// adds them.
//
// data is an object with surface forms as keys and readings as values (the same format as
// the bundled reading_overrides.json). Use it to load a project-specific dictionary
// without copying it into Go code.
//
// When the JSON is malformed or contains an empty key or value, NewConverter returns an
// error.
func WithReadingOverridesJSON(data []byte) Option {
	return func(c *Converter) {
		overrides, err := loadReadingOverridesJSON(data)
		if err != nil {
			c.optionErr = err
			return
		}
		WithReadingOverrides(overrides)(c)
	}
}

// WithReadingOverrides returns an Option that adds reading overrides keyed by surface form.
//
// The keys of overrides are surface forms in the input, the values their readings.
// Entries with an empty key or value are ignored. A surface form that already has an
// override is replaced with the given reading.
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

// WithoutDefaultReadingOverrides returns an Option that drops the bundled default reading
// overrides and uses dictionary readings only. When combining it with WithReadingOverrides,
// pass this Option first (options are applied in the order given).
func WithoutDefaultReadingOverrides() Option {
	return func(c *Converter) {
		c.readingOverrides = map[string]string{}
		c.rebuildOverrideKeys()
	}
}

// NewConverter creates a new Converter.
//
// options are applied in order after the default reading overrides are set.
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
		if c.optionErr != nil {
			return nil, c.optionErr
		}
	}
	return c, nil
}

// ConvertToReading converts input into a katakana reading.
//
// The input is split into lines and each line is analysed separately. Analysing across
// newlines lets a line-initial word be dragged by the context of the preceding symbols or
// unknown words (English tags and the like) and split differently (for example, analysing
// "[Chorus]\n重なる" as one piece splits 重なる into 重/なる, read オモナル). In lyrics a
// line is the unit of a sentence, so analysing by line is also what gets the parts of
// speech right.
//
// Line terminators are carried into the output as they were in the input (CRLF stays
// CRLF).
//
// Pass words whose reading the caller wants to decide in katakana. A katakana word is
// analysed as one noun and comes back as its own reading. A word written in hiragana only
// that starts with 「へ」 or 「を」 has its first sound split off as a particle and gets the
// particle correction ("へいそうする" → エ イソウスル; in "へいわな街" the following 街 is
// read ガイ as well). Whether a hiragana 「へ」 is a particle or the start of a word cannot
// be told from the characters alone, so this conversion cannot fix it.
func (c *Converter) ConvertToReading(input string) string {
	var sb strings.Builder
	sb.Grow(len(input) * 2)
	for line := range strings.Lines(input) {
		body, terminator := splitLineTerminator(line)
		sb.WriteString(c.convertLine(body))
		sb.WriteString(terminator)
	}
	return sb.String()
}

// splitLineTerminator splits a line into its body and its line terminator.
//
// Analysing with the \r of a CRLF left in the body makes the tokenizer pick it up as an
// unknown word that leaks into the reading, and the \r also blocks the trailing trim of
// phrase spacing.
func splitLineTerminator(line string) (body, terminator string) {
	body, ok := strings.CutSuffix(line, "\n")
	if !ok {
		return line, ""
	}
	if withoutCR, ok := strings.CutSuffix(body, "\r"); ok {
		return withoutCR, "\r\n"
	}
	return body, "\n"
}

// convertLine converts one line of text into a katakana reading.
//
// It tokenizes the whole line once and then lays the reading overrides on top of the
// result, in that order. Applying the overrides first would cut the input into fragments
// that are then analysed without context, and the parts of speech come out wrong (for
// example, the 「が」 in 「運命の閃光が」 is taken as a conjunction rather than a particle
// once isolated, which drops both the particle correction and the phrase spacing).
//
// Only override matches that start and end on a morpheme boundary are taken. A match that
// crosses a boundary drops whatever characters the override failed to cover (for example
// 「世界」 against 「世界観」).
func (c *Converter) convertLine(input string) string {
	if input == "" {
		return ""
	}

	tokens := c.t.Tokenize(input)
	boundaries := tokenBoundaries(input, tokens)

	var sb strings.Builder
	sb.Grow(len(input) * 2)

	for i := 0; i < len(tokens); {
		token := tokens[i]

		surface, ok := c.overrideIndex.match(input, token.Position, boundaries)
		if !ok {
			// Numbers come after the reading overrides. Overrides go first because a reading
			// the user stated must always win; a number's reading is, like the dictionary's,
			// only the default interpretation.
			if numberReading, next, matched := c.matchNumberAt(input, tokens, i, boundaries); matched {
				sb.WriteString(numberReading)
				c.writePhraseBreak(&sb, tokens[next-1].POS())
				i = next
				continue
			}
			pos := token.POS()
			sb.WriteString(tokenReading(token, pos))
			c.writePhraseBreak(&sb, pos)
			i++
			continue
		}

		sb.WriteString(c.readingOverrides[surface])
		// Skip every token the override covered, and decide phrase spacing by the last of them.
		end := token.Position + len(surface)
		last := i
		for i < len(tokens) && tokens[i].Position < end {
			last = i
			i++
		}
		c.writePhraseBreak(&sb, tokens[last].POS())
	}

	result := sb.String()
	if c.phraseSpacing {
		result = strings.TrimRight(result, " ")
	}
	return result
}

// writePhraseBreak writes a space right after a token at a phrase boundary.
func (c *Converter) writePhraseBreak(sb *strings.Builder, pos []string) {
	if c.phraseSpacing && isPhraseBreak(pos) {
		sb.WriteByte(' ')
	}
}

// tokenBoundaries returns the set of boundaries: the start byte offset of every token, plus
// the end of the input.
func tokenBoundaries(input string, tokens []tokenizer.Token) map[int]struct{} {
	boundaries := make(map[int]struct{}, len(tokens)+1)
	for _, token := range tokens {
		boundaries[token.Position] = struct{}{}
	}
	boundaries[len(input)] = struct{}{}
	return boundaries
}

// rebuildOverrideKeys rebuilds the index from the keys of the reading overrides.
func (c *Converter) rebuildOverrideKeys() {
	c.overrideIndex = newPrefixIndex(slices.Collect(maps.Keys(c.readingOverrides)))
}

// cloneReadingOverrides copies a map of reading overrides.
func cloneReadingOverrides(overrides map[string]string) map[string]string {
	cloned := make(map[string]string, len(overrides))
	maps.Copy(cloned, overrides)
	return cloned
}

// mustLoadReadingOverridesJSON loads the reading overrides from the bundled JSON.
func mustLoadReadingOverridesJSON(data []byte) map[string]string {
	overrides, err := loadReadingOverridesJSON(data)
	if err != nil {
		panic(err)
	}
	return overrides
}

// loadReadingOverridesJSON loads JSON with surface forms as keys and readings as values.
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

// tokenReading returns the dictionary reading of one token, with the particle
// pronunciation corrected. pos is the token's part-of-speech hierarchy (token.POS()).
func tokenReading(token tokenizer.Token, pos []string) string {
	if corrected, ok := particleReading(token, pos); ok {
		return corrected
	}
	return dictionaryReading(token)
}

// dictionaryReading returns the dictionary reading. A word the dictionary has no reading
// for (an unknown word or a symbol) falls back to its surface form, but since the output
// contract is katakana, hiragana is normalized to katakana.
func dictionaryReading(token tokenizer.Token) string {
	reading, ok := token.Reading()
	if !ok || reading == "*" {
		return hiraganaToKatakana(token.Surface)
	}
	return reading
}

// hiraganaToKatakana maps the hiragana in a string to the corresponding katakana.
// Every other character (katakana, kanji, alphanumerics, symbols) is left unchanged.
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

// hiraganaToKatakanaOffset is the distance between the hiragana and katakana blocks in
// Unicode.
const hiraganaToKatakanaOffset = 'ア' - 'あ'

// isConvertibleHiragana reports whether r is a hiragana that has a katakana counterpart:
// ぁ (U+3041) through ゖ (U+3096), plus the iteration marks ゝ and ゞ.
func isConvertibleHiragana(r rune) bool {
	return (r >= 'ぁ' && r <= 'ゖ') || r == 'ゝ' || r == 'ゞ'
}

// isPhraseBreak reports whether the top level of the part-of-speech hierarchy is one that
// ends a phrase.
func isPhraseBreak(pos []string) bool {
	return len(pos) > 0 && (pos[0] == "助詞" || pos[0] == "助動詞")
}

// particleReading returns the pronunciation of 「は」「へ」「を」 used as a particle.
func particleReading(token tokenizer.Token, pos []string) (string, bool) {
	if len(pos) == 0 || pos[0] != "助詞" {
		return "", false
	}

	reading, ok := particleReadings[token.Surface]
	return reading, ok
}
