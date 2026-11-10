package phonetic

import (
	"strconv"
	"strings"
	"testing"

	"github.com/ikawaha/kagome-dict/ipa"
	"github.com/ikawaha/kagome/v2/tokenizer"
)

func TestConverter_WithNumberReading(t *testing.T) {
	converter, err := NewConverter(WithNumberReading())
	if err != nil {
		t.Fatalf("NewConverter() error = %v", err)
	}

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"date", "2026年8月25日", "ニセンニジュウロクネンハチガツニジュウゴニチ"},
		{"time of day", "12時30分", "ジュウニジサンジュップン"},
		{"thousands separators", "1,234,567円", "ヒャクニジュウサンマンヨンセンゴヒャクロクジュウナナエン"},
		{"decimal", "3.5秒", "サンテンゴビョウ"},
		{"decimal with symbol", "2.5%", "ニテンゴパーセント"},
		{"percent", "100%の愛", "ヒャクパーセントノアイ"},
		{"README example", "1,234人が100%を3ヶ月で", "センニヒャクサンジュウヨニンガヒャクパーセントオサンカゲツデ"},
		{"README example (counter sound changes)", "1本と3本と6本", "イッポントサンボントロッポン"},
		{"full-width digits", "１２３", "ヒャクニジュウサン"},
		{"zero", "0", "ゼロ"},
		{"no counter", "BPM180で疾走する", "BPMヒャクハチジュウデシッソウスル"},
		{"with prefix", "第3回", "ダイサンカイ"},
		{"place-value kanji", "1兆と3億と1万", "イッチョウトサンオクトイチマン"},
		{"irregular readings of nin", "1人と2人と4人", "ヒトリトフタリトヨニン"},
		// The 「ヨ」 of 四人ヨニン depends on the ones digit, so 十四人 is ジュウヨニン too.
		{"reading decided by the ones digit", "14人と24人と11人", "ジュウヨニントニジュウヨニントジュウイチニン"},
		{"ones digit of hours", "4時と17時と19時と24時", "ヨジトジュウシチジトジュウクジトニジュウヨジ"},
		{"ones digit of years and yen", "14年で14円", "ジュウヨネンデジュウヨエン"},
		// 「1日」 alone or in a date is ツイタチ, but イチニチ when a duration word or a number
		// follows.
		{"first of the month as a date", "8月1日と毎月1日、1日から3日まで", "ハチガツツイタチトマイツキツイタチ、ツイタチカラミッカマデ"},
		{"one day as a duration", "1日中に1日3回、1日目と1日おきと1日あたり", "イチニチチュウニイチニチサンカイ、イチニチメトイチニチオキトイチニチアタリ"},
		{"day and a half and a day later", "1日半と1日後", "イチニチハントイチニチゴ"},
		// The analyser emits 「日間」 and 「分間」 as one token, so they are read by entries
		// separate from 「日」 and 「分」.
		{"day durations", "1日間と2日間と3日間と14日間", "イチニチカントフツカカントミッカカントジュウヨッカカン"},
		{"minute durations", "1分間と3分間と10分間と十分間", "イップンカントサンプンカントジュップンカントジュップンカン"},
		{"ones digit of hours and durations", "0時と9時間", "レイジトクジカン"},
		{"gemination before sa and ta rows", "1体と1社と1章と1種類と3種類", "イッタイトイッシャトイッショウトイッシュルイトサンシュルイ"},
		// Full-width digits become one token per character, so the digits after a thousands
		// separator are counted together.
		{"full-width thousands separators", "２，０００円と１，２３４，５６７円", "ニセンエントヒャクニジュウサンマンヨンセンゴヒャクロクジュウナナエン"},
		{"full-width comma that is not a separator", "１，２月と１，２３４５", "イチ，ニガツトイチ，ニセンサンビャクヨンジュウゴ"},
		{"full-width decimal", "３．５秒", "サンテンゴビョウ"},
		{"irregular readings of days", "1日と20日と14日", "ツイタチトハツカトジュウヨッカ"},
		{"irregular reading of tsu", "3つの願い", "ミッツノネガイ"},
		{"irregular reading of age", "20歳", "ハタチ"},
		{"irregular readings of months", "4月と7月と9月", "シガツトシチガツトクガツ"},
		{"month counts", "3ヶ月と1ヶ月", "サンカゲツトイッカゲツ"},
		// A leading 0 usually marks an identifier rather than a quantity, so it is not read by
		// place value.
		{"leading zeros", "007", "ゼロゼロナナ"},
		{"phone-number-like sequence", "0120", "ゼロイチニゼロ"},
		// Digit counts that do not fit in a uint64 are read one character at a time too.
		{"digit overflow", "12345678901234567890123", "イチニサンヨンゴロクナナハチキュウゼロイチニサンヨンゴロクナナハチキュウゼロイチニサン"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := converter.ConvertToReading(tt.input); got != tt.want {
				t.Errorf("ConvertToReading(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestConverter_NumberReadingIsOptIn checks that existing behaviour does not change
// without the Option. A number's reading is only the default interpretation, and changing
// it unasked would silently rewrite the output callers expected.
func TestConverter_NumberReadingIsOptIn(t *testing.T) {
	converter, err := NewConverter()
	if err != nil {
		t.Fatalf("NewConverter() error = %v", err)
	}

	for input, want := range map[string]string{
		"2026年8月25日": "2026ネン8ツキ25ニチ",
		"三本と一人":      "サンホントイチニン",
	} {
		if got := converter.ConvertToReading(input); got != want {
			t.Errorf("ConvertToReading(%q) = %q, want %q", input, got, want)
		}
	}
}

// TestKanjiNumerals checks that kanji numerals are read by the number rules only when a
// counter follows.
//
// The dictionary can read a kanji numeral itself (三 → サン) but not the sound change at
// the counter, so "三本" comes out サンホン. 「一本」, 「二人」 and the like used to be fixed
// one by one with reading overrides, but that broke every time the number changed (nobody
// had fixed 三本 or 十本), so it moved to the rules.
func TestKanjiNumerals(t *testing.T) {
	converter, err := NewConverter(WithNumberReading())
	if err != nil {
		t.Fatalf("NewConverter() error = %v", err)
	}

	tests := []struct {
		name string
		want map[string]string
	}{
		{
			name: "sound changes at the counter boundary",
			want: map[string]string{
				"三本": "サンボン", "十本": "ジュッポン", "一本の矢": "イッポンノヤ",
				"一階": "イッカイ", "一杯": "イッパイ", "一発": "イッパツ", "一回": "イッカイ",
				"一週間": "イッシュウカン", "一分": "イップン", "十分": "ジュップン",
			},
		},
		{
			name: "irregular readings",
			want: map[string]string{
				"一人": "ヒトリ", "二人で": "フタリデ", "一粒": "ヒトツブ",
				"三日": "ミッカ", "四日": "ヨッカ", "二十日": "ハツカ", "二十歳": "ハタチ",
				"四人": "ヨニン", "四年": "ヨネン",
			},
		},
		{
			name: "words that relied on overrides for gemination",
			want: map[string]string{
				"僕が一歩を踏み出す": "ボクガイッポオフミダス",
				"一本気な一匹狼":   "イッポンギナイッピキオオカミ",
			},
		},
		{
			name: "place values and digit sequences",
			want: map[string]string{
				"二千二十六年": "ニセンニジュウロクネン", "二〇二六年": "ニセンニジュウロクネン",
				"十万人": "ジュウマンニン", "百万本": "ヒャクマンボン", "一千万人": "イッセンマンニン",
				"一〇〇本": "ヒャッポン", "何十本": "ナンジュッポン", "数十人": "スウジュウニン",
			},
		},
		{
			name: "kanji numerals without a counter keep the dictionary reading",
			want: map[string]string{
				"三": "サン", "千の風": "センノカゼ", "三三七拍子": "サンサンナナヒョウシ",
				"一途": "イチズ", "四季": "シキ", "十分な": "ジュウブンナ", "一人前": "イチニンマエ",
			},
		},
		{
			name: "sequence starting with a unit word is not a number",
			want: map[string]string{
				"万円": "マンエン", "数万円": "スウマンエン", "万感の思い": "バンカンノオモイ",
			},
		},
		{
			// By the rules "一日" would be ツイタチ, but in lyrics and prose it is almost always
			// イチニチ (一日中, 一日の終わり). Overrides take effect before the rules, so a reading
			// override protects it. The Arabic "1日" on its own is a date and stays ツイタチ
			// (readFirstDayAsDuration handles a following duration word).
			name: "ichinichi by override",
			want: map[string]string{
				"一日中": "イチニチチュウ", "一日の終わり": "イチニチノオワリ", "1日": "ツイタチ",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for input, want := range tt.want {
				if got := converter.ConvertToReading(input); got != want {
					t.Errorf("ConvertToReading(%q) = %q, want %q", input, got, want)
				}
			}
		})
	}
}

func TestParseKanjiNumeral(t *testing.T) {
	tests := []struct {
		input string
		want  uint64
		ok    bool
	}{
		{"〇", 0, true},
		{"三", 3, true},
		{"十", 10, true},
		{"十二", 12, true},
		{"二十", 20, true},
		{"二千二十六", 2026, true},
		{"二〇二六", 2026, true},
		{"一〇〇", 100, true},
		{"十万", 100_000, true},
		{"百万", 1_000_000, true},
		{"一千万", 10_000_000, true},
		{"三億二千万", 320_000_000, true},
		{"一兆", 1_000_000_000_000, true},
		{"", 0, false},
		{"万", 0, false},
		{"億", 0, false},
		{"三二十", 0, false},
		{"一二三四五六七八九〇一二三四五六七八九", 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, ok := parseKanjiNumeral(tt.input)
			if ok != tt.ok || got != tt.want {
				t.Errorf("parseKanjiNumeral(%q) = (%d, %v), want (%d, %v)", tt.input, got, ok, tt.want, tt.ok)
			}
		})
	}
}

// TestCounterSoundChanges checks the gemination, rendaku voicing and handakuten at the join
// of number and counter, per counter class. Getting a class wrong produces readings that
// are not Japanese, such as サンホン for 三本 or サンバツ for 三発.
func TestCounterSoundChanges(t *testing.T) {
	converter, err := NewConverter(WithNumberReading())
	if err != nil {
		t.Fatalf("NewConverter() error = %v", err)
	}

	tests := []struct {
		name string
		want map[string]string
	}{
		{
			name: "ka row only geminates",
			want: map[string]string{
				"1回": "イッカイ", "3回": "サンカイ", "6回": "ロッカイ",
				"8回": "ハッカイ", "10回": "ジュッカイ", "100回": "ヒャッカイ",
			},
		},
		{
			name: "ken and kai voice after three in the ka row",
			want: map[string]string{
				"1軒": "イッケン", "3軒": "サンゲン", "6軒": "ロッケン",
				"3階": "サンガイ", "13階": "ジュウサンガイ",
			},
		},
		{
			name: "sa row unchanged after six and hundred",
			want: map[string]string{
				"1冊": "イッサツ", "3冊": "サンサツ", "6冊": "ロクサツ",
				"8冊": "ハッサツ", "10冊": "ジュッサツ", "100冊": "ヒャクサツ",
			},
		},
		{
			name: "ta row follows the sa row rules",
			want: map[string]string{
				"1点": "イッテン", "6点": "ロクテン", "8点": "ハッテン",
				"100点": "ヒャクテン", "1兆": "イッチョウ", "10兆": "ジュッチョウ",
			},
		},
		{
			name: "ha row voices after three",
			want: map[string]string{
				"1本": "イッポン", "3本": "サンボン", "4本": "ヨンホン",
				"6本": "ロッポン", "10本": "ジュッポン", "100本": "ヒャッポン",
				"1000本": "センボン", "10000本": "イチマンボン", "3杯": "サンバイ", "3匹": "サンビキ",
			},
		},
		{
			name: "hatsu and haku take p after three in the ha row",
			want: map[string]string{
				"1発": "イッパツ", "3発": "サンパツ", "4発": "ヨンハツ",
				"6発": "ロッパツ", "3泊": "サンパク", "3編": "サンペン", "10000発": "イチマンパツ",
			},
		},
		{
			name: "places and units geminate",
			want: map[string]string{
				"1ヶ所": "イッカショ", "1箇所": "イッカショ", "6ヶ所": "ロッカショ", "3ヶ所": "サンカショ",
				"1丁目": "イッチョウメ", "8丁目": "ハッチョウメ", "1頭": "イットウ", "3頭": "サントウ",
				"1ページ": "イッページ", "3ページ": "サンページ", "10ページ": "ジュッページ",
				"1キロ": "イッキロ", "1キロメートル": "イッキロメートル", "100キログラム": "ヒャッキログラム",
				"1センチ": "イッセンチ", "1センチメートル": "イッセンチメートル", "6センチ": "ロクセンチ",
				"1セット": "イッセット", "1トン": "イットン",
			},
		},
		{
			name: "native counters read hito and futa for one and two",
			want: map[string]string{
				"1組": "ヒトクミ", "2組": "フタクミ", "3組": "サンクミ", "10組": "ジュックミ",
				"1株": "ヒトカブ", "2切れ": "フタキレ", "1皿": "ヒトサラ", "8皿": "ハッサラ",
				"1粒": "ヒトツブ", "3粒": "サンツブ", "1袋": "ヒトフクロ", "3袋": "サンブクロ", "10袋": "ジュップクロ",
			},
		},
		{
			name: "only fun takes p after four",
			want: map[string]string{
				"1分": "イップン", "2分": "ニフン", "3分": "サンプン",
				"4分": "ヨンプン", "14分": "ジュウヨンプン", "6分": "ロップン",
				"10分": "ジュップン",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for input, want := range tt.want {
				if got := converter.ConvertToReading(input); got != want {
					t.Errorf("ConvertToReading(%q) = %q, want %q", input, got, want)
				}
			}
		})
	}
}

// TestNumberReadingRespectsTokenBoundaries checks that a character that looks like a
// counter is left alone when it is not used as a counter in that position. Only matches
// ending on a morpheme boundary are taken, so the "人" of "3人称" is not a counter.
func TestNumberReadingRespectsTokenBoundaries(t *testing.T) {
	converter, err := NewConverter(WithNumberReading())
	if err != nil {
		t.Fatalf("NewConverter() error = %v", err)
	}

	tests := []struct {
		input string
		want  string
	}{
		{"3人称", "サンニンショウ"},
		{"一人称", "イチニンショウ"},
		{"兆し", "キザシ"},
		{"万感の思い", "バンカンノオモイ"},
		// Words with kanji numerals that the dictionary holds as one word are not split, so they
		// are left alone.
		{"十二月", "ジュウニガツ"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := converter.ConvertToReading(tt.input); got != tt.want {
				t.Errorf("ConvertToReading(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestNumberReadingCombinesWithOtherOptions checks that number reading works together with
// reading overrides and phrase spacing.
func TestNumberReadingCombinesWithOtherOptions(t *testing.T) {
	t.Run("with phrase spacing", func(t *testing.T) {
		converter, err := NewConverter(WithNumberReading(), WithPhraseSpacing())
		if err != nil {
			t.Fatalf("NewConverter() error = %v", err)
		}
		if got, want := converter.ConvertToReading("2026年に3人が集まる"), "ニセンニジュウロクネンニ サンニンガ アツマル"; got != want {
			t.Errorf("ConvertToReading() = %q, want %q", got, want)
		}
	})

	// A reading the user stated takes precedence over the default interpretation of a number.
	t.Run("reading override wins over number reading", func(t *testing.T) {
		converter, err := NewConverter(
			WithNumberReading(),
			WithReadingOverrides(map[string]string{"4649": "ヨロシク"}),
		)
		if err != nil {
			t.Fatalf("NewConverter() error = %v", err)
		}
		if got, want := converter.ConvertToReading("4649"), "ヨロシク"; got != want {
			t.Errorf("ConvertToReading() = %q, want %q", got, want)
		}
	})
}

func TestReadInteger(t *testing.T) {
	tests := []struct {
		value uint64
		want  string
	}{
		{0, "ゼロ"},
		{1, "イチ"},
		{10, "ジュウ"},
		{11, "ジュウイチ"},
		{100, "ヒャク"},
		{300, "サンビャク"},
		{600, "ロッピャク"},
		{800, "ハッピャク"},
		{1000, "セン"},
		{3000, "サンゼン"},
		{8000, "ハッセン"},
		{1234, "センニヒャクサンジュウヨン"},
		{10000, "イチマン"},
		{12345, "イチマンニセンサンビャクヨンジュウゴ"},
		// The thousands digit becomes イッセン when 万 or 億 follows.
		{10000000, "イッセンマン"},
		{100000000, "イチオク"},
		// Place units geminate too.
		{1000000000000, "イッチョウ"},
		{10000000000000, "ジュッチョウ"},
		{10000000000000000, "イッケイ"},
		// A digit group of 0 in the middle is skipped.
		{100000001, "イチオクイチ"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := readInteger(tt.value); got != tt.want {
				t.Errorf("readInteger(%d) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}
}

// TestCountersHaveKatakanaReadings checks that every reading in the counter table is
// katakana. Mixed-in hiragana or kanji would break the output contract (katakana).
func TestCountersHaveKatakanaReadings(t *testing.T) {
	isKatakana := func(s string) bool {
		for _, r := range s {
			if (r < 'ァ' || r > 'ヶ') && r != 'ー' {
				return false
			}
		}
		return s != ""
	}

	for surface, counter := range counters {
		if !isKatakana(counter.reading) {
			t.Errorf("counter %q has reading %q, which is not katakana", surface, counter.reading)
		}
		for value, reading := range counter.irregular {
			if !isKatakana(reading) {
				t.Errorf("counter %q for %d has reading %q, which is not katakana", surface, value, reading)
			}
		}
	}
}

// TestCounterIndexIsLongestFirst checks that the counter index is ordered longest first.
// Shortest first, "月" would match before "ヶ月" and "3ヶ月" would become サンヶガツ.
func TestCounterIndexIsLongestFirst(t *testing.T) {
	for first, keys := range counterIndex {
		for i := 1; i < len(keys); i++ {
			if len(keys[i-1]) < len(keys[i]) {
				t.Errorf("keys starting with %q are not ordered by length: %v", string(first), keys)
				break
			}
		}
	}
}

func BenchmarkConverter_ConvertToReading_WithNumberReading(b *testing.B) {
	converter, err := NewConverter(WithNumberReading())
	if err != nil {
		b.Fatalf("NewConverter() error = %v", err)
	}
	input := strings.Repeat("2026年8月25日、1,234人が3ヶ月かけて100%の曲を4本仕上げた。\n", 8)
	b.ReportAllocs()

	for b.Loop() {
		converter.ConvertToReading(input)
	}
}

// TestCounters_NoRedundantEntries checks that no counter in the table merely restates the
// dictionary's reading.
//
// A counter with neither a sound change nor an irregular reading is read correctly by the
// dictionary without a table entry. Listing it changes no behaviour and makes the counters
// that genuinely need correcting harder to spot. It is the same idea as
// TestDefaultReadingOverrides_NoRedundantEntries for the reading override dictionary.
func TestCounters_NoRedundantEntries(t *testing.T) {
	tok, err := tokenizer.New(ipa.Dict(), tokenizer.OmitBosEos())
	if err != nil {
		t.Fatalf("failed to create tokenizer: %v", err)
	}

	// Run the representative numbers that can trigger gemination, voicing or irregular readings.
	values := []uint64{1, 2, 3, 4, 6, 7, 8, 9, 10, 14, 20, 24, 100, 1000}

	for surface, counter := range counters {
		t.Run(surface, func(t *testing.T) {
			dictionary := dictionaryReadingOf(tok, surface)
			for _, value := range values {
				number := readNumeral(strconv.FormatUint(value, 10))
				if counter.read(number, nil) != number.reading+dictionary {
					return // If any one differs from the dictionary, this entry is doing its job.
				}
			}
			t.Errorf("counter %q (%q) matches the dictionary reading (%q) for every number; the dictionary reads it correctly, so remove it",
				surface, counter.reading, dictionary)
		})
	}
}

// dictionaryReadingOf returns the reading of a surface form analysed on its own.
func dictionaryReadingOf(tok *tokenizer.Tokenizer, surface string) string {
	var sb strings.Builder
	for _, token := range tok.Tokenize(surface) {
		sb.WriteString(dictionaryReading(token))
	}
	return sb.String()
}
