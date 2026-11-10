package phonetic

import (
	"maps"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/ikawaha/kagome/v2/tokenizer"
)

// Number readings cannot be built from the morphological analyser's dictionary alone. The
// IPA dictionary has no readings for Arabic numerals (the reading of "2026" comes out as
// "*"), and it has only the standalone reading of a counter, detached from any number
// ("ツキ" for "月"). Output as is, "2026年8月" becomes "2026ネン8ツキ", breaking both the
// "output is katakana" contract and the reading of the month (ハチガツ).
//
// This file turns runs of digits into Japanese readings and applies the sound changes at
// the join with a following counter: gemination, rendaku voicing and handakuten
// (一回→イッカイ, 三本→サンボン, 六分→ロップン).
//
// Kanji numerals follow the same rules. The dictionary can read a kanji numeral itself
// (三 → サン) but not the sound change at the counter, so "三本" comes out サンホン and
// "一階" イチカイ. Unlike Arabic numerals, a kanji numeral on its own is fine with the
// dictionary reading, so they are taken over only when a counter follows.

// digitReadings are the readings of a single Arabic digit.
var digitReadings = [10]string{"ゼロ", "イチ", "ニ", "サン", "ヨン", "ゴ", "ロク", "ナナ", "ハチ", "キュウ"}

// numeral is the reading and value read from a run of digits.
type numeral struct {
	// reading is the reading of the number itself.
	reading string
	// value is the numeric value. It is meaningless when hasValue is false.
	value uint64
	// hasValue reports whether value can be used to pick a counter's irregular reading
	// (4日→ヨッカ and so on). It is false for decimals, runs with a leading 0, and digit
	// counts that do not fit in a uint64.
	hasValue bool
}

// readNumeral converts a run of Arabic digits ("1234", "3.5") into a reading.
func readNumeral(digits string) numeral {
	integer, fraction, hasFraction := strings.Cut(digits, ".")

	// A leading 0 usually marks a code or identifier ("007", "0120") rather than a
	// quantity, so read it one character at a time instead of by place value.
	if integer == "" || (len(integer) > 1 && integer[0] == '0') {
		return numeral{reading: readDigitsOneByOne(digits)}
	}
	value, err := strconv.ParseUint(integer, 10, 64)
	if err != nil {
		return numeral{reading: readDigitsOneByOne(digits)}
	}

	reading := readInteger(value)
	if !hasFraction {
		return numeral{reading: reading, value: value, hasValue: true}
	}
	// Digits after the decimal point are read one at a time, not by place value
	// (3.14 → サンテンイチヨン).
	return numeral{reading: reading + "テン" + readDigitsOneByOne(fraction)}
}

// readDigitsOneByOne reads digits one character at a time.
func readDigitsOneByOne(digits string) string {
	var sb strings.Builder
	sb.Grow(len(digits) * 6)
	for _, r := range digits {
		if r >= '0' && r <= '9' {
			sb.WriteString(digitReadings[r-'0'])
			continue
		}
		if r == '.' {
			sb.WriteString("テン")
		}
	}
	return sb.String()
}

// scaleUnits are the place units for each group of 4 digits. The index is the number of
// the digit group counted from the bottom.
var scaleUnits = [...]struct {
	reading string
	class   soundClass
}{
	{"", soundPlain},
	{"マン", soundPlain},
	{"オク", soundPlain},
	{"チョウ", soundT}, // 一兆 → イッチョウ
	{"ケイ", soundK},  // 一京 → イッケイ
}

// readInteger converts an integer into a reading.
func readInteger(value uint64) string {
	if value == 0 {
		return digitReadings[0]
	}

	var groups []uint64
	for value > 0 {
		groups = append(groups, value%10000)
		value /= 10000
	}

	var sb strings.Builder
	for i, g := range slices.Backward(groups) {
		if g == 0 {
			continue
		}
		group := readGroup(int(g), i > 0)
		if i == 0 {
			sb.WriteString(group)
			continue
		}
		sb.WriteString(joinWithSound(group, scaleUnits[i].reading, scaleUnits[i].class))
	}
	return sb.String()
}

// readGroup builds the reading of 1-9999.
// scaled says whether a higher unit such as 万 or 億 follows, which decides how the
// thousands are read (セン or イッセン).
func readGroup(n int, scaled bool) string {
	var sb strings.Builder
	if d := n / 1000; d > 0 {
		sb.WriteString(readThousands(d, scaled))
	}
	if d := n / 100 % 10; d > 0 {
		sb.WriteString(readHundreds(d))
	}
	if d := n / 10 % 10; d > 0 {
		sb.WriteString(readTens(d))
	}
	if d := n % 10; d > 0 {
		sb.WriteString(digitReadings[d])
	}
	return sb.String()
}

// readThousands reads the thousands digit. 1000 is 「セン」, not 「イチセン」, but when
// 万 or 億 follows it is read イッセン, as in 「一千万」 (一千万→イッセンマン).
func readThousands(d int, scaled bool) string {
	switch d {
	case 1:
		if scaled {
			return "イッセン"
		}
		return "セン"
	case 3:
		return "サンゼン"
	case 8:
		return "ハッセン"
	default:
		return digitReadings[d] + "セン"
	}
}

// readHundreds reads the hundreds digit.
func readHundreds(d int) string {
	switch d {
	case 1:
		return "ヒャク"
	case 3:
		return "サンビャク"
	case 6:
		return "ロッピャク"
	case 8:
		return "ハッピャク"
	default:
		return digitReadings[d] + "ヒャク"
	}
}

// readTens reads the tens digit. 10 is 「ジュウ」, not 「イチジュウ」.
func readTens(d int) string {
	if d == 1 {
		return "ジュウ"
	}
	return digitReadings[d] + "ジュウ"
}

// soundClass classifies how the start of a counter changes with the number before it.
type soundClass int

const (
	// soundPlain is a counter whose sound does not change (枚, 年, 円, 台).
	soundPlain soundClass = iota
	// soundK is a ka-row counter (回, 個, 曲). 一回→イッカイ, 六回→ロッカイ.
	soundK
	// soundKVoiced is a ka-row counter voiced after 三 (軒, 階). 三軒→サンゲン, 三階→サンガイ.
	soundKVoiced
	// soundS is a sa-row counter (冊, 歳, 週). 一冊→イッサツ, while 六冊 stays ロクサツ.
	soundS
	// soundT is a ta-row counter (点, 通, 兆). It geminates under the same conditions as the
	// sa row.
	soundT
	// soundH is a ha-row counter voiced after 三 (本, 匹, 杯). 一本→イッポン, 三本→サンボン.
	soundH
	// soundP is a ha-row counter that takes handakuten after 三 (発, 泊, 編). 一発→イッパツ,
	// 三発→サンパツ.
	soundP
	// soundPYon is like soundP but also takes handakuten after 四 (分). 四分→ヨンプン.
	soundPYon
)

// tailKind classifies the ending of the number that comes right before a counter.
type tailKind int

const (
	tailNone tailKind = iota
	tailOne
	tailThree
	tailFour
	tailSix
	tailEight
	tailTen
	tailHundred
	tailThousand
	// tailMan is a number ending in 万. Like 三, it voices the ha row (百万本→ヒャクマンボン).
	tailMan
)

// numberTails are the number endings that trigger gemination or voicing.
// The voiced spellings are listed too, so that the hundreds are still recognized as an
// ending after they have been voiced or taken handakuten themselves (サンビャク,
// ロッピャク).
var numberTails = []struct {
	tail string
	// geminated is the geminated form. An empty string means the ending does not geminate.
	geminated string
	kind      tailKind
}{
	{"ヒャク", "ヒャッ", tailHundred},
	{"ビャク", "ビャッ", tailHundred},
	{"ピャク", "ピャッ", tailHundred},
	{"ジュウ", "ジュッ", tailTen},
	{"イチ", "イッ", tailOne},
	{"ロク", "ロッ", tailSix},
	{"ハチ", "ハッ", tailEight},
	{"サン", "", tailThree},
	{"ヨン", "", tailFour},
	{"セン", "", tailThousand},
	{"ゼン", "", tailThousand},
	{"マン", "", tailMan},
}

// voicing is the voicing applied to the start of a counter.
type voicing int

const (
	voicingNone voicing = iota
	// voicingSemi is handakuten (ホン → ポン).
	voicingSemi
	// voicingFull is rendaku voicing (ホン → ボン).
	voicingFull
)

// joinWithSound joins a number's reading and a counter, applying the sound change at the
// join.
func joinWithSound(number, counter string, class soundClass) string {
	if counter == "" {
		return number
	}

	tail, geminated, kind := findNumberTail(number)
	geminate, voice := soundChange(kind, class)
	if geminate && geminated != "" {
		number = number[:len(number)-len(tail)] + geminated
	}
	return number + applyVoicing(counter, voice)
}

// findNumberTail returns the ending of a number's reading.
func findNumberTail(number string) (tail, geminated string, kind tailKind) {
	for _, candidate := range numberTails {
		if strings.HasSuffix(number, candidate.tail) {
			return candidate.tail, candidate.geminated, candidate.kind
		}
	}
	return "", "", tailNone
}

// soundChange returns the sound change for a combination of ending and counter.
func soundChange(kind tailKind, class soundClass) (geminate bool, voice voicing) {
	switch class {
	case soundK:
		// 一回イッカイ 六回ロッカイ 八回ハッカイ 十回ジュッカイ 百回ヒャッカイ
		return geminatesKRow(kind), voicingNone
	case soundKVoiced:
		if kind == tailThree {
			return false, voicingFull // 三軒サンゲン 三階サンガイ
		}
		return geminatesKRow(kind), voicingNone
	case soundS, soundT:
		// 一冊イッサツ 八冊ハッサツ 十冊ジュッサツ. 六冊 and 百冊 do not change.
		return kind == tailOne || kind == tailEight || kind == tailTen, voicingNone
	case soundH:
		if geminatesKRow(kind) {
			return true, voicingSemi // 一本イッポン 六本ロッポン 百本ヒャッポン
		}
		if kind == tailThree || kind == tailThousand || kind == tailMan {
			return false, voicingFull // 三本サンボン 千本センボン 一万本イチマンボン
		}
		return false, voicingNone
	case soundP, soundPYon:
		if geminatesKRow(kind) {
			return true, voicingSemi // 一発イッパツ 六発ロッパツ 十分ジュップン
		}
		if kind == tailThree || kind == tailThousand || kind == tailMan {
			return false, voicingSemi // 三発サンパツ 千発センパツ 一万発イチマンパツ
		}
		if kind == tailFour && class == soundPYon {
			return false, voicingSemi // 四分ヨンプン. 四発 stays ヨンハツ.
		}
		return false, voicingNone
	}
	return false, voicingNone
}

// geminatesKRow reports whether the ending makes a ka-row or ha-row counter geminate.
// 一, 六, 八, 十 and 百 do, one more than for the sa and ta rows (六回 is ロッカイ, 六冊 is
// ロクサツ).
func geminatesKRow(kind tailKind) bool {
	return kind == tailOne || kind == tailSix || kind == tailEight || kind == tailTen || kind == tailHundred
}

// voicedForms are the katakana after rendaku voicing, semiVoicedForms those after
// handakuten.
var (
	voicedForms = map[rune]rune{
		'カ': 'ガ', 'キ': 'ギ', 'ク': 'グ', 'ケ': 'ゲ', 'コ': 'ゴ',
		'サ': 'ザ', 'シ': 'ジ', 'ス': 'ズ', 'セ': 'ゼ', 'ソ': 'ゾ',
		'タ': 'ダ', 'チ': 'ヂ', 'ツ': 'ヅ', 'テ': 'デ', 'ト': 'ド',
		'ハ': 'バ', 'ヒ': 'ビ', 'フ': 'ブ', 'ヘ': 'ベ', 'ホ': 'ボ',
	}
	semiVoicedForms = map[rune]rune{
		'ハ': 'パ', 'ヒ': 'ピ', 'フ': 'プ', 'ヘ': 'ペ', 'ホ': 'ポ',
	}
)

// applyVoicing applies a voicing to the first character of a counter.
func applyVoicing(counter string, voice voicing) string {
	forms := voicedForms
	switch voice {
	case voicingNone:
		return counter
	case voicingSemi:
		forms = semiVoicedForms
	}

	first, size := utf8.DecodeRuneInString(counter)
	voiced, ok := forms[first]
	if !ok {
		return counter
	}
	return string(voiced) + counter[size:]
}

// counter is how one counter is read.
type counter struct {
	reading string
	class   soundClass
	// irregular are per-number readings that sound changes cannot explain (4日→ヨッカ,
	// 1人→ヒトリ). The key is the number, the value the whole reading of number plus
	// counter.
	irregular map[uint64]string
	// digitOverrides are readings of the ones digit that change only right before this
	// counter (the 「ヨ」 of 4人→ヨニン). The change depends on the ones digit, so 14人 is
	// ジュウヨニン too. Per-value irregular entries would mean listing 4, 14, 24… without
	// end, so these are keyed by the ones digit instead.
	digitOverrides map[uint64]string
	// contextual decides the reading when it depends on the word right after the counter
	// (1日中→イチニチチュウ). following is the token right after the counter, or nil at the
	// end of the sentence. If it returns no reading the normal rules apply. Unused when
	// nil.
	contextual func(n numeral, following *tokenizer.Token) (string, bool)
}

// read returns the reading of the number joined with the counter. following is the token
// right after the counter, or nil at the end of the sentence.
func (c counter) read(n numeral, following *tokenizer.Token) string {
	if c.contextual != nil {
		if reading, ok := c.contextual(n, following); ok {
			return reading
		}
	}
	if n.hasValue {
		if irregular, ok := c.irregular[n.value]; ok {
			return irregular
		}
	}
	return joinWithSound(c.overrideLastDigit(n), c.reading, c.class)
}

// overrideLastDigit swaps in the reading of the ones digit that changes only right before
// the counter.
func (c counter) overrideLastDigit(n numeral) string {
	if !n.hasValue {
		return n.reading
	}
	lastDigit := n.value % 10
	override, ok := c.digitOverrides[lastDigit]
	if !ok {
		return n.reading
	}
	// Leave alone a reading that does not end in the ones digit (digits read one by one, for
	// example).
	standard := digitReadings[lastDigit]
	if !strings.HasSuffix(n.reading, standard) {
		return n.reading
	}
	return n.reading[:len(n.reading)-len(standard)] + override
}

// counters are the counters whose reading changes when placed right after a number.
//
// Only counters whose reading with a number differs from the dictionary's standalone
// reading belong here. A counter with neither a sound change nor an irregular reading is
// read correctly by the dictionary as is.
var counters = map[string]counter{
	// Time and dates. They differ widely from the standalone readings, so they carry
	// irregular readings.
	"年":  {reading: "ネン", digitOverrides: map[uint64]string{4: "ヨ"}},
	"月":  {reading: "ガツ", irregular: monthReadings()},
	"ヶ月": {reading: "カゲツ", class: soundK},
	"ケ月": {reading: "カゲツ", class: soundK},
	"か月": {reading: "カゲツ", class: soundK},
	"カ月": {reading: "カゲツ", class: soundK},
	"箇月": {reading: "カゲツ", class: soundK},
	"日":  {reading: "ニチ", irregular: dayReadings, contextual: readFirstDayAsDuration},
	// The analyser emits 「日間」 and 「分間」 as one token, so the 「日」 and 「分」 entries never
	// see them.
	"日間": {reading: "ニチカン", irregular: dayDurationReadings()},
	"分間": {reading: "フンカン", class: soundPYon},
	// 0時 is レイジ, not ゼロジ (as time signals and timetables read it).
	"時":  {reading: "ジ", digitOverrides: map[uint64]string{4: "ヨ", 7: "シチ", 9: "ク"}, irregular: map[uint64]string{0: "レイジ"}},
	"時間": {reading: "ジカン", digitOverrides: map[uint64]string{4: "ヨ", 9: "ク"}},
	"分":  {reading: "フン", class: soundPYon},
	"円":  {reading: "エン", digitOverrides: map[uint64]string{4: "ヨ"}},
	"週":  {reading: "シュウ", class: soundS},
	"週間": {reading: "シュウカン", class: soundS},

	// People and things.
	"人": {
		reading:        "ニン",
		irregular:      map[uint64]string{1: "ヒトリ", 2: "フタリ"},
		digitOverrides: map[uint64]string{4: "ヨ"},
	},
	"つ": {reading: "ツ", irregular: map[uint64]string{
		1: "ヒトツ", 2: "フタツ", 3: "ミッツ", 4: "ヨッツ", 5: "イツツ",
		6: "ムッツ", 7: "ナナツ", 8: "ヤッツ", 9: "ココノツ", 10: "トオ",
	}},
	"歳": {reading: "サイ", class: soundS, irregular: map[uint64]string{20: "ハタチ"}},
	"才": {reading: "サイ", class: soundS, irregular: map[uint64]string{20: "ハタチ"}},

	// Ka row. Only gemination occurs.
	"回": {reading: "カイ", class: soundK},
	"個": {reading: "コ", class: soundK},
	"件": {reading: "ケン", class: soundK},
	"軒": {reading: "ケン", class: soundKVoiced},
	"曲": {reading: "キョク", class: soundK},
	"巻": {reading: "カン", class: soundK},
	"課": {reading: "カ", class: soundK},
	"校": {reading: "コウ", class: soundK},
	"階": {reading: "カイ", class: soundKVoiced},

	// Sa and ta rows.
	"冊":  {reading: "サツ", class: soundS},
	"隻":  {reading: "セキ", class: soundS},
	"足":  {reading: "ソク", class: soundS},
	"周":  {reading: "シュウ", class: soundS},
	"社":  {reading: "シャ", class: soundS},
	"章":  {reading: "ショウ", class: soundS},
	"種":  {reading: "シュ", class: soundS},
	"種類": {reading: "シュルイ", class: soundS},
	"点":  {reading: "テン", class: soundT},
	"通":  {reading: "ツウ", class: soundT},
	"着":  {reading: "チャク", class: soundT},
	"体":  {reading: "タイ", class: soundT},

	// Ha row. Voicing or handakuten occurs on top of gemination. Whether 三 voices or takes
	// handakuten depends on the counter: 三本サンボン and 三杯サンバイ, but 三発サンパツ and
	// 三泊サンパク.
	"本": {reading: "ホン", class: soundH},
	"歩": {reading: "ホ", class: soundP},
	"匹": {reading: "ヒキ", class: soundH},
	"杯": {reading: "ハイ", class: soundH},
	"泊": {reading: "ハク", class: soundP},
	"発": {reading: "ハツ", class: soundP},
	"拍": {reading: "ハク", class: soundP},
	"編": {reading: "ヘン", class: soundP},
	"篇": {reading: "ヘン", class: soundP},

	// Places and units. Units geminate even as katakana words (一キロ→イッキロ,
	// 一セット→イッセット).
	"ヶ所":      {reading: "カショ", class: soundK},
	"ケ所":      {reading: "カショ", class: soundK},
	"か所":      {reading: "カショ", class: soundK},
	"カ所":      {reading: "カショ", class: soundK},
	"箇所":      {reading: "カショ", class: soundK},
	"丁目":      {reading: "チョウメ", class: soundT},
	"頭":       {reading: "トウ", class: soundT},
	"ページ":     {reading: "ページ", class: soundP},
	"キロ":      {reading: "キロ", class: soundK},
	"キロメートル":  {reading: "キロメートル", class: soundK},
	"キログラム":   {reading: "キログラム", class: soundK},
	"センチ":     {reading: "センチ", class: soundS},
	"センチメートル": {reading: "センチメートル", class: soundS},
	"セット":     {reading: "セット", class: soundS},
	"トン":      {reading: "トン", class: soundT},

	// Counters counted with native Japanese numbers. Only one and two become ヒト and フタ;
	// from three on they go back to Sino-Japanese numbers.
	"組":  {reading: "クミ", class: soundK, irregular: map[uint64]string{1: "ヒトクミ", 2: "フタクミ"}},
	"株":  {reading: "カブ", class: soundK, irregular: map[uint64]string{1: "ヒトカブ", 2: "フタカブ"}},
	"切れ": {reading: "キレ", class: soundK, irregular: map[uint64]string{1: "ヒトキレ", 2: "フタキレ"}},
	"皿":  {reading: "サラ", class: soundS, irregular: map[uint64]string{1: "ヒトサラ", 2: "フタサラ"}},
	"粒":  {reading: "ツブ", class: soundT, irregular: map[uint64]string{1: "ヒトツブ", 2: "フタツブ"}},
	"袋":  {reading: "フクロ", class: soundH, irregular: map[uint64]string{1: "ヒトフクロ", 2: "フタフクロ"}},

	// Of the place-value kanji, only 兆, which geminates. 万 and 億 are correct with the
	// dictionary reading, and 京 is left out because it is hardly ever used as a number and
	// would misfire on place names and compounds far more often.
	"兆": {reading: "チョウ", class: soundT},

	// Symbols. The dictionary has no reading for the half-width %, so it is given one only
	// after a number (the full-width ％ is read by the dictionary and needs no entry).
	"%": {reading: "パーセント"},
}

// dayReadings are the irregular readings of dates. Days 1-10 and 14, 20 and 24 cannot be
// built from the number's reading.
var dayReadings = map[uint64]string{
	1: "ツイタチ", 2: "フツカ", 3: "ミッカ", 4: "ヨッカ", 5: "イツカ",
	6: "ムイカ", 7: "ナノカ", 8: "ヨウカ", 9: "ココノカ", 10: "トオカ",
	14: "ジュウヨッカ", 20: "ハツカ", 24: "ニジュウヨッカ",
}

// dayDurationReadings are the readings of 「N日間」. They add カン to the irregular date
// readings, except that 1日間 is イチニチカン, not ツイタチカン (ツイタチ is for dates
// only).
func dayDurationReadings() map[uint64]string {
	readings := make(map[uint64]string, len(dayReadings))
	for value, reading := range dayReadings {
		readings[value] = reading + "カン"
	}
	readings[1] = "イチニチカン"
	return readings
}

// durationSuffixes are the words that, right after 「1日」, make it a duration (イチニチ)
// rather than a date.
var durationSuffixes = map[string]bool{
	"中": true, "目": true, "間": true, "後": true, "前": true, "半": true,
	"おき": true, "置き": true, "あたり": true, "当たり": true, "当り": true,
	"ごと": true, "毎": true, "以内": true, "以上": true, "以下": true, "分": true,
}

// readFirstDayAsDuration tells the contexts where 「1日」 is a duration (イチニチ) rather
// than a date (ツイタチ). 「1日」 alone or in 「8月1日」 is a date and stays ツイタチ; only
// when a duration word (1日中, 1日目, 1日おき) or a number (1日3回) follows is it read
// イチニチ. The kanji 「一日」 never gets here because the dictionary reads it as one word
// (the reading overrides take care of it).
func readFirstDayAsDuration(n numeral, following *tokenizer.Token) (string, bool) {
	if !n.hasValue || n.value != 1 || following == nil {
		return "", false
	}
	if durationSuffixes[following.Surface] || isDigitToken(*following) {
		return "イチニチ", true
	}
	return "", false
}

// monthReadings are the readings of months 1-12. 4月シガツ, 7月シチガツ and 9月クガツ
// cannot be built from the default number readings (ヨン, ナナ, キュウ).
func monthReadings() map[uint64]string {
	names := [...]string{
		1: "イチ", 2: "ニ", 3: "サン", 4: "シ", 5: "ゴ", 6: "ロク",
		7: "シチ", 8: "ハチ", 9: "ク", 10: "ジュウ", 11: "ジュウイチ", 12: "ジュウニ",
	}
	readings := make(map[uint64]string, len(names)-1)
	for month := uint64(1); month < uint64(len(names)); month++ {
		readings[month] = names[month] + "ガツ"
	}
	return readings
}

// counterIndex looks up counter keys by longest match.
var counterIndex = newPrefixIndex(slices.Collect(maps.Keys(counters)))

// matchNumberAt reads the run of digits starting at tokens[i] together with the counter
// that follows it. It returns the reading, the position of the token after what it read,
// and whether it read a number.
//
// The dictionary has no readings for Arabic numerals, so they are read whether or not a
// counter follows. Kanji numerals on their own are read correctly by the dictionary, so
// they are read here only when a counter follows and a sound change is needed.
func (c *Converter) matchNumberAt(input string, tokens []tokenizer.Token, i int, boundaries map[int]struct{}) (string, int, bool) {
	if !c.numberReading {
		return "", 0, false
	}

	if digits, end, next := scanDigits(tokens, i); digits != "" {
		number := readNumeral(digits)
		if key, found := counterIndex.match(input, end, boundaries); found {
			return readWithCounter(number, key, tokens, end, next)
		}
		return number.reading, next, true
	}

	number, end, next, ok := scanKanjiNumeral(tokens, i)
	if !ok {
		return "", 0, false
	}
	key, found := counterIndex.match(input, end, boundaries)
	if !found {
		return "", 0, false
	}
	return readWithCounter(number, key, tokens, end, next)
}

// readWithCounter reads the number together with the counter key and returns the position
// of the token after the counter. end is the byte offset where the number ends, next the
// position of the token after the number.
func readWithCounter(number numeral, key string, tokens []tokenizer.Token, end, next int) (string, int, bool) {
	// Skip every token the counter covers. Some counters, such as "ヶ月", split into several
	// tokens, so how far to advance is decided by byte offset rather than token count.
	counterEnd := end + len(key)
	for next < len(tokens) && tokens[next].Position < counterEnd {
		next++
	}
	var following *tokenizer.Token
	if next < len(tokens) {
		following = &tokens[next]
	}
	return counters[key].read(number, following), next, true
}

// kanjiDigitValues are the values of single kanji digits, kanjiUnitValues those of the
// place-value kanji.
var (
	kanjiDigitValues = map[rune]uint64{
		'〇': 0, '零': 0, '一': 1, '二': 2, '三': 3, '四': 4,
		'五': 5, '六': 6, '七': 7, '八': 8, '九': 9,
	}
	kanjiUnitValues = map[rune]uint64{
		'十': 10, '百': 100, '千': 1000,
		'万': 10_000, '億': 100_000_000, '兆': 1_000_000_000_000,
	}
)

// maxKanjiNumeralRunes caps the length of a run of kanji numerals interpreted as a number,
// keeping it within what fits in a uint64 when read as a digit sequence (一二三…).
const maxKanjiNumeralRunes = 18

// scanKanjiNumeral reads the run of kanji numerals starting at tokens[i] as a number.
// It returns the number, the byte offset where the run ends, the position of the token
// after it, and whether it could be interpreted as a number. The analyser emits kanji
// numerals one character per token, so the run is gathered before it is evaluated.
func scanKanjiNumeral(tokens []tokenizer.Token, i int) (number numeral, end, next int, ok bool) {
	var sb strings.Builder
	for next = i; next < len(tokens) && isKanjiNumeralToken(tokens[next]); next++ {
		sb.WriteString(tokens[next].Surface)
		end = tokens[next].Position + len(tokens[next].Surface)
	}
	if next == i {
		return numeral{}, 0, 0, false
	}
	value, ok := parseKanjiNumeral(sb.String())
	if !ok {
		return numeral{}, 0, 0, false
	}
	return numeral{reading: readInteger(value), value: value, hasValue: true}, end, next, true
}

// parseKanjiNumeral converts a run of kanji numerals into a number.
// It accepts both place-value notation (二千二十六) and digit sequences (二〇二六).
//
// A run starting with 万, 億 or 兆 ("万円", "億") is a unit word, not a number, so it is
// not treated as one. A run where a place unit follows consecutive digits ("三二十")
// cannot be read as a number and is rejected the same way.
func parseKanjiNumeral(s string) (uint64, bool) {
	runes := []rune(s)
	if len(runes) == 0 || len(runes) > maxKanjiNumeralRunes {
		return 0, false
	}

	if !slices.ContainsFunc(runes, isKanjiUnit) {
		var value uint64
		for _, r := range runes {
			value = value*10 + kanjiDigitValues[r]
		}
		return value, true
	}

	var (
		total, section, digit uint64
		hasDigit              bool
	)
	for _, r := range runes {
		if d, ok := kanjiDigitValues[r]; ok {
			if hasDigit {
				return 0, false
			}
			digit, hasDigit = d, true
			continue
		}
		unit := kanjiUnitValues[r]
		if unit >= kanjiUnitValues['万'] {
			section += digit
			if section == 0 {
				return 0, false
			}
			total += section * unit
			section, digit, hasDigit = 0, 0, false
			continue
		}
		multiplier := uint64(1)
		if hasDigit {
			multiplier = digit
		}
		section += multiplier * unit
		digit, hasDigit = 0, false
	}
	return total + section + digit, true
}

// isKanjiNumeralToken reports whether a token consists only of kanji digits and
// place-value kanji.
func isKanjiNumeralToken(token tokenizer.Token) bool {
	if token.Surface == "" {
		return false
	}
	for _, r := range token.Surface {
		if _, ok := kanjiDigitValues[r]; ok {
			continue
		}
		if !isKanjiUnit(r) {
			return false
		}
	}
	return true
}

// isKanjiUnit reports whether r is a place-value kanji (十, 百, 千, 万, 億, 兆).
func isKanjiUnit(r rune) bool {
	_, ok := kanjiUnitValues[r]
	return ok
}

// scanDigits extracts the run of digits starting at tokens[i].
// Thousands separators and decimal points are taken as part of the number only when there
// are digits on both sides.
func scanDigits(tokens []tokenizer.Token, i int) (digits string, end, next int) {
	if !isDigitToken(tokens[i]) {
		return "", 0, 0
	}

	var sb strings.Builder
	appendDigits := func(index int) {
		sb.WriteString(normalizeDigits(tokens[index].Surface))
		end = tokens[index].Position + len(tokens[index].Surface)
	}

	appendDigits(i)
	next = i + 1
	hasFraction := false

	for next < len(tokens) {
		switch {
		// A thousands separator counts as part of the number only when exactly 3 digits
		// follow; otherwise a listing comma as in "1,2月" would be swallowed too.
		// The analyser emits full-width digits one character per token, so the digits
		// are counted across all the digit tokens that follow.
		case isSeparator(tokens[next], ",", "，") && digitRunAfter(tokens, next) == 3:
			for next++; next < len(tokens) && isDigitToken(tokens[next]); next++ {
				appendDigits(next)
			}
		case !hasFraction && isSeparator(tokens[next], ".", "．") && followedByDigits(tokens, next, 0):
			hasFraction = true
			sb.WriteString(".")
			appendDigits(next + 1)
			next += 2
		case isDigitToken(tokens[next]):
			appendDigits(next)
			next++
		default:
			return sb.String(), end, next
		}
	}
	return sb.String(), end, next
}

// followedByDigits reports whether the token after tokens[i] is a digit token.
// When digitCount is greater than 0, the digit count must match as well.
func followedByDigits(tokens []tokenizer.Token, i, digitCount int) bool {
	if i+1 >= len(tokens) || !isDigitToken(tokens[i+1]) {
		return false
	}
	return digitCount == 0 || utf8.RuneCountInString(tokens[i+1].Surface) == digitCount
}

// digitRunAfter returns the total number of digits in the run of digit tokens after
// tokens[i]. Half-width "234" is one token of 3 digits and full-width "２３４" is 3 tokens of
// 1 digit each; both give 3.
func digitRunAfter(tokens []tokenizer.Token, i int) int {
	digits := 0
	for j := i + 1; j < len(tokens) && isDigitToken(tokens[j]); j++ {
		digits += utf8.RuneCountInString(tokens[j].Surface)
	}
	return digits
}

// isSeparator reports whether a token is one of the given separators.
func isSeparator(token tokenizer.Token, separators ...string) bool {
	return slices.Contains(separators, token.Surface)
}

// isDigitToken reports whether a token consists only of Arabic digits.
func isDigitToken(token tokenizer.Token) bool {
	if token.Surface == "" {
		return false
	}
	for _, r := range token.Surface {
		if !isDigit(r) {
			return false
		}
	}
	return true
}

// isDigit reports whether r is an Arabic digit, half-width or full-width.
func isDigit(r rune) bool {
	return (r >= '0' && r <= '9') || (r >= '０' && r <= '９')
}

// fullWidthDigitOffset is the distance between the half-width and full-width digit blocks
// in Unicode.
const fullWidthDigitOffset = '０' - '0'

// normalizeDigits converts full-width Arabic digits to half-width.
func normalizeDigits(s string) string {
	if !strings.ContainsFunc(s, func(r rune) bool { return r >= '０' && r <= '９' }) {
		return s
	}
	return strings.Map(func(r rune) rune {
		if r >= '０' && r <= '９' {
			return r - fullWidthDigitOffset
		}
		return r
	}, s)
}
