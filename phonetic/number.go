package phonetic

import (
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/ikawaha/kagome/v2/tokenizer"
)

// 数詞の読みは、形態素解析器の辞書だけでは作れません。IPA 辞書は算用数字に読みを持たず
// （"2026" の読みは "*" になります）、助数詞は数と切り離された単独の読み（"月" なら "ツキ"）
// しか持たないためです。そのまま出力すると "2026年8月" が "2026ネン8ツキ" になり、
// 「出力はカタカナ」という契約も、月の読み（ハチガツ）も同時に崩れます。
//
// このファイルは、数の並びを日本語の読みに直し、続く助数詞との間に起きる促音便・連濁・
// 半濁音化（一回→イッカイ、三本→サンボン、六分→ロップン）を当てる処理をまとめます。

// digitReadings は算用数字 1 文字ぶんの読みです。
var digitReadings = [10]string{"ゼロ", "イチ", "ニ", "サン", "ヨン", "ゴ", "ロク", "ナナ", "ハチ", "キュウ"}

// numeral は数の並びから読み取った読みと値です。
type numeral struct {
	// reading は数そのものの読みです。
	reading string
	// value は数値です。hasValue が false のときは意味を持ちません。
	value uint64
	// hasValue は value を助数詞の特殊読み（4日→ヨッカ等）の判定に使えるかを表します。
	// 小数、先頭が 0 の並び、uint64 に収まらない桁数では false になります。
	hasValue bool
}

// readNumeral は算用数字の並び（"1234"、"3.5"）を読みに変換します。
func readNumeral(digits string) numeral {
	integer, fraction, hasFraction := strings.Cut(digits, ".")

	// 先頭の 0 は数量ではなく符号や識別子（"007"、"0120"）のことが多いので、
	// 桁として読まずに 1 文字ずつ読みます。
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
	// 小数点以下は桁ではなく 1 文字ずつ読む（3.14 → サンテンイチヨン）。
	return numeral{reading: reading + "テン" + readDigitsOneByOne(fraction)}
}

// readDigitsOneByOne は数字を 1 文字ずつ読みます。
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

// scaleUnits は 4 桁ごとの位取りの単位です。索引は下から数えた桁グループの番号です。
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

// readInteger は整数を読みに変換します。
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
	for i := len(groups) - 1; i >= 0; i-- {
		if groups[i] == 0 {
			continue
		}
		group := readGroup(int(groups[i]))
		if i == 0 {
			sb.WriteString(group)
			continue
		}
		sb.WriteString(joinWithSound(group, scaleUnits[i].reading, scaleUnits[i].class))
	}
	return sb.String()
}

// readGroup は 1〜9999 の読みを組み立てます。
func readGroup(n int) string {
	var sb strings.Builder
	if d := n / 1000; d > 0 {
		sb.WriteString(readThousands(d))
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

// readThousands は千の位を読みます。1000 は「イチセン」ではなく「セン」です。
func readThousands(d int) string {
	switch d {
	case 1:
		return "セン"
	case 3:
		return "サンゼン"
	case 8:
		return "ハッセン"
	default:
		return digitReadings[d] + "セン"
	}
}

// readHundreds は百の位を読みます。
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

// readTens は十の位を読みます。10 は「イチジュウ」ではなく「ジュウ」です。
func readTens(d int) string {
	if d == 1 {
		return "ジュウ"
	}
	return digitReadings[d] + "ジュウ"
}

// soundClass は、直前の数によって助数詞の頭がどう変わるかの分類です。
type soundClass int

const (
	// soundPlain は音が変わらない助数詞です（枚、年、円、台）。
	soundPlain soundClass = iota
	// soundK はカ行の助数詞です（回、個、曲）。一回→イッカイ、六回→ロッカイ。
	soundK
	// soundKVoiced はカ行のうち、三で連濁する助数詞です（軒、階）。三軒→サンゲン、三階→サンガイ。
	soundKVoiced
	// soundS はサ行の助数詞です（冊、歳、週）。一冊→イッサツ、六冊はロクサツのまま。
	soundS
	// soundT はタ行の助数詞です（点、通、兆）。促音便の条件はサ行と同じです。
	soundT
	// soundH はハ行のうち、三で連濁する助数詞です（本、匹、杯）。一本→イッポン、三本→サンボン。
	soundH
	// soundP はハ行のうち、三で半濁音になる助数詞です（発、泊、編）。一発→イッパツ、三発→サンパツ。
	soundP
	// soundPYon は soundP に加えて四でも半濁音になる助数詞です（分）。四分→ヨンプン。
	soundPYon
)

// tailKind は助数詞の直前に来る数の語尾の分類です。
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
)

// numberTails は、促音便・連濁を起こす数の語尾です。
// 百の位は連濁・半濁を受けた後の形（サンビャク、ロッピャク）でも語尾として拾えるように、
// 濁点付きの綴りも並べます。
var numberTails = []struct {
	tail string
	// geminated は促音便の形です。空文字は促音便を起こさない語尾です。
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
}

// voicing は助数詞の頭に当てる濁り方です。
type voicing int

const (
	voicingNone voicing = iota
	// voicingSemi は半濁音化です（ホン → ポン）。
	voicingSemi
	// voicingFull は連濁です（ホン → ボン）。
	voicingFull
)

// joinWithSound は数の読みと助数詞をつなぎ、境目に起きる音の変化を当てます。
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

// findNumberTail は数の読みの語尾を返します。
func findNumberTail(number string) (tail, geminated string, kind tailKind) {
	for _, candidate := range numberTails {
		if strings.HasSuffix(number, candidate.tail) {
			return candidate.tail, candidate.geminated, candidate.kind
		}
	}
	return "", "", tailNone
}

// soundChange は語尾と助数詞の組み合わせに対する音の変化を返します。
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
		// 一冊イッサツ 八冊ハッサツ 十冊ジュッサツ。六冊・百冊は変化しない。
		return kind == tailOne || kind == tailEight || kind == tailTen, voicingNone
	case soundH:
		if geminatesKRow(kind) {
			return true, voicingSemi // 一本イッポン 六本ロッポン 百本ヒャッポン
		}
		if kind == tailThree || kind == tailThousand {
			return false, voicingFull // 三本サンボン 千本センボン
		}
		return false, voicingNone
	case soundP, soundPYon:
		if geminatesKRow(kind) {
			return true, voicingSemi // 一発イッパツ 六発ロッパツ 十分ジュップン
		}
		if kind == tailThree || kind == tailThousand {
			return false, voicingSemi // 三発サンパツ 千発センパツ
		}
		if kind == tailFour && class == soundPYon {
			return false, voicingSemi // 四分ヨンプン。四発はヨンハツのまま。
		}
		return false, voicingNone
	}
	return false, voicingNone
}

// geminatesKRow は、カ行・ハ行の助数詞が促音便を起こす語尾かを判定します。
// 一・六・八・十・百が該当し、サ行・タ行より条件がひとつ広い（六回はロッカイ、六冊はロクサツ）。
func geminatesKRow(kind tailKind) bool {
	return kind == tailOne || kind == tailSix || kind == tailEight || kind == tailTen || kind == tailHundred
}

// voicedForms は連濁後のカタカナ、semiVoicedForms は半濁音化後のカタカナです。
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

// applyVoicing は助数詞の先頭 1 文字に濁り方を当てます。
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

// counter は助数詞 1 つぶんの読み方です。
type counter struct {
	reading string
	class   soundClass
	// irregular は、音の変化では説明できない数ごとの読みです（4日→ヨッカ、1人→ヒトリ）。
	// キーは数値、値は数と助数詞をまとめた読み全体です。
	irregular map[uint64]string
	// digitOverrides は、この助数詞の直前でだけ変わる一の位の読みです（4人→ヨニン の「ヨ」）。
	// この変化は一の位で決まるので、14人もジュウヨニンになります。値ごとの irregular では
	// 4・14・24…と際限なく並べることになるため、桁ではなく一の位で持ちます。
	digitOverrides map[uint64]string
}

// read は数と助数詞をつないだ読みを返します。
func (c counter) read(n numeral) string {
	if n.hasValue {
		if irregular, ok := c.irregular[n.value]; ok {
			return irregular
		}
	}
	return joinWithSound(c.overrideLastDigit(n), c.reading, c.class)
}

// overrideLastDigit は、助数詞の直前でだけ変わる一の位の読みを差し替えます。
func (c counter) overrideLastDigit(n numeral) string {
	if !n.hasValue {
		return n.reading
	}
	lastDigit := n.value % 10
	override, ok := c.digitOverrides[lastDigit]
	if !ok {
		return n.reading
	}
	// 一の位が語尾に現れていない読み（桁を1文字ずつ読んだ場合など）には手を出さない。
	standard := digitReadings[lastDigit]
	if !strings.HasSuffix(n.reading, standard) {
		return n.reading
	}
	return n.reading[:len(n.reading)-len(standard)] + override
}

// counters は、数の直後に置かれたときに読みが変わる助数詞です。
//
// ここに載せるのは「数と組んだときの読みが辞書の単独読みと違う」ものだけです。
// 音の変化も特殊読みもない助数詞は、辞書の読みをそのまま使えば正しく読めます。
var counters = map[string]counter{
	// 時と日付。単独読みと大きく食い違うので特殊読みを持ちます。
	"年":  {reading: "ネン", digitOverrides: map[uint64]string{4: "ヨ"}},
	"月":  {reading: "ガツ", irregular: monthReadings()},
	"ヶ月": {reading: "カゲツ", class: soundK},
	"ケ月": {reading: "カゲツ", class: soundK},
	"か月": {reading: "カゲツ", class: soundK},
	"カ月": {reading: "カゲツ", class: soundK},
	"箇月": {reading: "カゲツ", class: soundK},
	"日": {reading: "ニチ", irregular: map[uint64]string{
		1: "ツイタチ", 2: "フツカ", 3: "ミッカ", 4: "ヨッカ", 5: "イツカ",
		6: "ムイカ", 7: "ナノカ", 8: "ヨウカ", 9: "ココノカ", 10: "トオカ",
		14: "ジュウヨッカ", 20: "ハツカ", 24: "ニジュウヨッカ",
	}},
	"時":  {reading: "ジ", digitOverrides: map[uint64]string{4: "ヨ", 7: "シチ", 9: "ク"}},
	"時間": {reading: "ジカン", digitOverrides: map[uint64]string{4: "ヨ"}},
	"分":  {reading: "フン", class: soundPYon},
	"円":  {reading: "エン", digitOverrides: map[uint64]string{4: "ヨ"}},
	"週":  {reading: "シュウ", class: soundS},
	"週間": {reading: "シュウカン", class: soundS},

	// 人と物。
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

	// カ行。促音便だけが起きます。
	"回": {reading: "カイ", class: soundK},
	"個": {reading: "コ", class: soundK},
	"件": {reading: "ケン", class: soundK},
	"軒": {reading: "ケン", class: soundKVoiced},
	"曲": {reading: "キョク", class: soundK},
	"巻": {reading: "カン", class: soundK},
	"課": {reading: "カ", class: soundK},
	"校": {reading: "コウ", class: soundK},
	"階": {reading: "カイ", class: soundKVoiced},

	// サ行・タ行。
	"冊": {reading: "サツ", class: soundS},
	"隻": {reading: "セキ", class: soundS},
	"足": {reading: "ソク", class: soundS},
	"周": {reading: "シュウ", class: soundS},
	"点": {reading: "テン", class: soundT},
	"通": {reading: "ツウ", class: soundT},
	"着": {reading: "チャク", class: soundT},

	// ハ行。促音便に加えて連濁・半濁音化が起きます。三で濁るか半濁るかは助数詞ごとに違い、
	// 三本サンボン・三杯サンバイに対して三発サンパツ・三泊サンパクになります。
	"本": {reading: "ホン", class: soundH},
	"匹": {reading: "ヒキ", class: soundH},
	"杯": {reading: "ハイ", class: soundH},
	"泊": {reading: "ハク", class: soundP},
	"発": {reading: "ハツ", class: soundP},
	"拍": {reading: "ハク", class: soundP},
	"編": {reading: "ヘン", class: soundP},
	"篇": {reading: "ヘン", class: soundP},

	// 位取りの漢字のうち、促音便が起きる兆だけ。万・億は辞書の読みのままで正しく、
	// 京は数としてまず使われず地名や熟語で誤爆する方が多いため入れていません。
	"兆": {reading: "チョウ", class: soundT},

	// 記号。半角の % は辞書が読みを持たないため、数の後ろでだけ読みを与えます
	// （全角の ％ は辞書が読めるので不要）。
	"%": {reading: "パーセント"},
}

// monthReadings は 1 月〜12 月の読みです。4 月シガツ・7 月シチガツ・9 月クガツ は、
// 数の既定の読み（ヨン・ナナ・キュウ）では作れません。
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

// counterKeysByFirstRune は助数詞のキーを先頭ルーンごとにまとめ、最長一致のために
// 各グループを長い順で保持します。読み上書きの索引と同じ作りです。
var counterKeysByFirstRune = buildCounterIndex()

func buildCounterIndex() map[rune][]string {
	keys := make([]string, 0, len(counters))
	for key := range counters {
		keys = append(keys, key)
	}
	slices.SortFunc(keys, func(a, b string) int {
		if diff := len(b) - len(a); diff != 0 {
			return diff
		}
		return strings.Compare(a, b)
	})

	index := make(map[rune][]string)
	for _, key := range keys {
		first, _ := utf8.DecodeRuneInString(key)
		index[first] = append(index[first], key)
	}
	return index
}

// matchNumberAt は、tokens[i] から始まる数字の並びと、それに続く助数詞をまとめて読みます。
// 戻り値は読み、読み終えた次のトークン位置、そして数字の並びを見つけたかどうかです。
func (c *Converter) matchNumberAt(input string, tokens []tokenizer.Token, i int, boundaries map[int]struct{}) (string, int, bool) {
	if !c.numberReading {
		return "", 0, false
	}

	digits, end, next := scanDigits(tokens, i)
	if digits == "" {
		return "", 0, false
	}

	number := readNumeral(digits)
	key, found := matchCounterAt(input, end, boundaries)
	if !found {
		return number.reading, next, true
	}

	// 助数詞が覆ったトークンをまとめて読み飛ばす。"ヶ月" のように複数トークンに
	// 割れる助数詞があるため、進めるのはトークン数ではなくバイト位置で判断する。
	counterEnd := end + len(key)
	for next < len(tokens) && tokens[next].Position < counterEnd {
		next++
	}
	return counters[key].read(number), next, true
}

// scanDigits は tokens[i] から続く数字の並びを取り出します。
// 桁区切りのカンマと小数点は、両側が数字のときだけ数の一部として取り込みます。
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
		// 桁区切りのカンマは、直後がちょうど3桁のときだけ数の一部とみなす。
		// そうしないと "1,2月" のような並列の読点まで飲み込んでしまう。
		case isSeparator(tokens[next], ",", "，") && followedByDigits(tokens, next, 3):
			appendDigits(next + 1)
			next += 2
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

// followedByDigits は、tokens[i] の次が数字トークンかを判定します。
// digitCount が 0 より大きいときは、桁数まで一致することを求めます。
func followedByDigits(tokens []tokenizer.Token, i, digitCount int) bool {
	if i+1 >= len(tokens) || !isDigitToken(tokens[i+1]) {
		return false
	}
	return digitCount == 0 || utf8.RuneCountInString(tokens[i+1].Surface) == digitCount
}

// isSeparator は、トークンが指定した区切り文字のいずれかかを判定します。
func isSeparator(token tokenizer.Token, separators ...string) bool {
	return slices.Contains(separators, token.Surface)
}

// isDigitToken は、トークンが算用数字だけで構成されているかを判定します。
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

// isDigit は半角・全角どちらの算用数字かを判定します。
func isDigit(r rune) bool {
	return (r >= '0' && r <= '9') || (r >= '０' && r <= '９')
}

// fullWidthDigitOffset は Unicode 上の半角・全角算用数字ブロック間の距離です。
const fullWidthDigitOffset = '０' - '0'

// normalizeDigits は全角の算用数字を半角へ揃えます。
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

// matchCounterAt は、start から始まり形態素境界で終わる最長の助数詞を返します。
func matchCounterAt(input string, start int, boundaries map[int]struct{}) (string, bool) {
	if start >= len(input) {
		return "", false
	}
	rest := input[start:]
	first, _ := utf8.DecodeRuneInString(rest)
	for _, key := range counterKeysByFirstRune[first] {
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
