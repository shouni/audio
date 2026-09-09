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
		{"年月日", "2026年8月25日", "ニセンニジュウロクネンハチガツニジュウゴニチ"},
		{"時刻", "12時30分", "ジュウニジサンジュップン"},
		{"桁区切りのカンマ", "1,234,567円", "ヒャクニジュウサンマンヨンセンゴヒャクロクジュウナナエン"},
		{"小数", "3.5秒", "サンテンゴビョウ"},
		{"小数と記号", "2.5%", "ニテンゴパーセント"},
		{"パーセント", "100%の愛", "ヒャクパーセントノアイ"},
		{"README の例", "1,234人が100%を3ヶ月で", "センニヒャクサンジュウヨニンガヒャクパーセントオサンカゲツデ"},
		{"README の例（助数詞の音便）", "1本と3本と6本", "イッポントサンボントロッポン"},
		{"全角数字", "１２３", "ヒャクニジュウサン"},
		{"ゼロ", "0", "ゼロ"},
		{"助数詞なし", "BPM180で疾走する", "BPMヒャクハチジュウデシッソウスル"},
		{"接頭辞と組み合わせ", "第3回", "ダイサンカイ"},
		{"位取りの漢字", "1兆と3億と1万", "イッチョウトサンオクトイチマン"},
		{"人の特殊読み", "1人と2人と4人", "ヒトリトフタリトヨニン"},
		// 四人ヨニンの「ヨ」は一の位で決まるので、十四人もジュウヨニンになる。
		{"一の位で決まる読み", "14人と24人と11人", "ジュウヨニントニジュウヨニントジュウイチニン"},
		{"時刻の一の位", "4時と17時と19時と24時", "ヨジトジュウシチジトジュウクジトニジュウヨジ"},
		{"年と円の一の位", "14年で14円", "ジュウヨネンデジュウヨエン"},
		{"日の特殊読み", "1日と20日と14日", "ツイタチトハツカトジュウヨッカ"},
		{"つの特殊読み", "3つの願い", "ミッツノネガイ"},
		{"歳の特殊読み", "20歳", "ハタチ"},
		{"月の特殊読み", "4月と7月と9月", "シガツトシチガツトクガツ"},
		{"ヶ月", "3ヶ月と1ヶ月", "サンカゲツトイッカゲツ"},
		// 先頭の 0 は数量ではなく識別子のことが多いので、桁として読まない。
		{"先頭がゼロ", "007", "ゼロゼロナナ"},
		{"電話番号のような並び", "0120", "ゼロイチニゼロ"},
		// uint64 に収まらない桁数も 1 文字ずつ読む。
		{"桁あふれ", "12345678901234567890123", "イチニサンヨンゴロクナナハチキュウゼロイチニサンヨンゴロクナナハチキュウゼロイチニサン"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := converter.ConvertToReading(tt.input); got != tt.want {
				t.Errorf("ConvertToReading(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestConverter_NumberReadingIsOptIn は、Option を付けなければ既存の挙動が変わらない
// ことを確認します。数字の読みは既定の解釈にすぎず、勝手に変えると呼び出し側の
// 期待していた出力が黙って書き換わります。
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

// TestKanjiNumerals は、漢数字に助数詞が続くときだけ数の規則で読むことを確認します。
//
// 辞書は漢数字そのものは読めますが（三 → サン）、助数詞との境目の音の変化を持たないため
// "三本" が サンホン になります。かつては「一本」「二人」などを読み上書きで個別に直して
// いましたが、数が変わるたびに外れる（三本・十本は誰も直していない）ので規則に寄せました。
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
			name: "助数詞との境目で音が変わる",
			want: map[string]string{
				"三本": "サンボン", "十本": "ジュッポン", "一本の矢": "イッポンノヤ",
				"一階": "イッカイ", "一杯": "イッパイ", "一発": "イッパツ", "一回": "イッカイ",
				"一週間": "イッシュウカン", "一分": "イップン", "十分": "ジュップン",
			},
		},
		{
			name: "特殊読み",
			want: map[string]string{
				"一人": "ヒトリ", "二人で": "フタリデ", "一粒": "ヒトツブ",
				"三日": "ミッカ", "四日": "ヨッカ", "二十日": "ハツカ", "二十歳": "ハタチ",
				"四人": "ヨニン", "四年": "ヨネン",
			},
		},
		{
			name: "促音便を上書きに頼っていた語",
			want: map[string]string{
				"僕が一歩を踏み出す": "ボクガイッポオフミダス",
				"一本気な一匹狼":   "イッポンギナイッピキオオカミ",
			},
		},
		{
			name: "位取りと桁の列",
			want: map[string]string{
				"二千二十六年": "ニセンニジュウロクネン", "二〇二六年": "ニセンニジュウロクネン",
				"十万人": "ジュウマンニン", "百万本": "ヒャクマンボン", "一千万人": "イッセンマンニン",
				"一〇〇本": "ヒャッポン", "何十本": "ナンジュッポン", "数十人": "スウジュウニン",
			},
		},
		{
			name: "助数詞が続かない漢数字は辞書読みのまま",
			want: map[string]string{
				"三": "サン", "千の風": "センノカゼ", "三三七拍子": "サンサンナナヒョウシ",
				"一途": "イチズ", "四季": "シキ", "十分な": "ジュウブンナ", "一人前": "イチニンマエ",
			},
		},
		{
			name: "単位語で始まる並びは数ではない",
			want: map[string]string{
				"万円": "マンエン", "数万円": "スウマンエン", "万感の思い": "バンカンノオモイ",
			},
		},
		{
			// "一日" は規則だと ツイタチ になるが、歌詞や文中ではほぼ イチニチ（一日中、一日の終わり）。
			// 上書きが規則より先に効くので、こちらは読み上書きで守る。算用数字の "1日" は日付なので ツイタチ のまま。
			name: "一日は上書きでイチニチ",
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

// TestCounterSoundChanges は、数と助数詞の境目に起きる促音便・連濁・半濁音化を
// 助数詞の分類ごとに確認します。分類を取り違えると三本がサンホン、三発がサンバツに
// なるなど、日本語として成立しない読みが出ます。
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
			name: "カ行は促音便だけ",
			want: map[string]string{
				"1回": "イッカイ", "3回": "サンカイ", "6回": "ロッカイ",
				"8回": "ハッカイ", "10回": "ジュッカイ", "100回": "ヒャッカイ",
			},
		},
		{
			name: "カ行のうち軒と階は三で連濁",
			want: map[string]string{
				"1軒": "イッケン", "3軒": "サンゲン", "6軒": "ロッケン",
				"3階": "サンガイ", "13階": "ジュウサンガイ",
			},
		},
		{
			name: "サ行は六と百で変化しない",
			want: map[string]string{
				"1冊": "イッサツ", "3冊": "サンサツ", "6冊": "ロクサツ",
				"8冊": "ハッサツ", "10冊": "ジュッサツ", "100冊": "ヒャクサツ",
			},
		},
		{
			name: "タ行はサ行と同じ条件",
			want: map[string]string{
				"1点": "イッテン", "6点": "ロクテン", "8点": "ハッテン",
				"100点": "ヒャクテン", "1兆": "イッチョウ", "10兆": "ジュッチョウ",
			},
		},
		{
			name: "ハ行は三で連濁",
			want: map[string]string{
				"1本": "イッポン", "3本": "サンボン", "4本": "ヨンホン",
				"6本": "ロッポン", "10本": "ジュッポン", "100本": "ヒャッポン",
				"1000本": "センボン", "10000本": "イチマンボン", "3杯": "サンバイ", "3匹": "サンビキ",
			},
		},
		{
			name: "ハ行でも発と泊は三で半濁音",
			want: map[string]string{
				"1発": "イッパツ", "3発": "サンパツ", "4発": "ヨンハツ",
				"6発": "ロッパツ", "3泊": "サンパク", "3編": "サンペン", "10000発": "イチマンパツ",
			},
		},
		{
			name: "場所と単位も促音便",
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
			name: "和語で数える助数詞は一つ二つがヒト・フタ",
			want: map[string]string{
				"1組": "ヒトクミ", "2組": "フタクミ", "3組": "サンクミ", "10組": "ジュックミ",
				"1株": "ヒトカブ", "2切れ": "フタキレ", "1皿": "ヒトサラ", "8皿": "ハッサラ",
				"1粒": "ヒトツブ", "3粒": "サンツブ", "1袋": "ヒトフクロ", "3袋": "サンブクロ", "10袋": "ジュップクロ",
			},
		},
		{
			name: "分だけは四でも半濁音",
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

// TestNumberReadingRespectsTokenBoundaries は、助数詞に見える文字がその位置で
// 助数詞として使われていないときに手を出さないことを確認します。形態素境界で
// 終わる一致だけを採るので、"3人称" の "人" は助数詞になりません。
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
		// 辞書が 1 語で持つ漢数字入りの語は分割されないので触らない。
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

// TestNumberReadingCombinesWithOtherOptions は、数の読みが読み上書きや文節スペースと
// 併用できることを確認します。
func TestNumberReadingCombinesWithOtherOptions(t *testing.T) {
	t.Run("文節スペースと併用", func(t *testing.T) {
		converter, err := NewConverter(WithNumberReading(), WithPhraseSpacing())
		if err != nil {
			t.Fatalf("NewConverter() error = %v", err)
		}
		if got, want := converter.ConvertToReading("2026年に3人が集まる"), "ニセンニジュウロクネンニ サンニンガ アツマル"; got != want {
			t.Errorf("ConvertToReading() = %q, want %q", got, want)
		}
	})

	// 利用者が明示した読みは、数の既定の解釈より優先される。
	t.Run("読み上書きが数より優先される", func(t *testing.T) {
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
		// 千の位は万・億が続くと イッセン になる。
		{10000000, "イッセンマン"},
		{100000000, "イチオク"},
		// 位取りの単位にも促音便が起きる。
		{1000000000000, "イッチョウ"},
		{10000000000000, "ジュッチョウ"},
		{10000000000000000, "イッケイ"},
		// 途中の桁グループが 0 なら読み飛ばす。
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

// TestCountersHaveKatakanaReadings は、助数詞表の読みがすべてカタカナであることを
// 確認します。ひらがなや漢字が混ざると、出力の契約（カタカナ）が崩れます。
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
			t.Errorf("助数詞 %q の読み %q がカタカナではありません", surface, counter.reading)
		}
		for value, reading := range counter.irregular {
			if !isKatakana(reading) {
				t.Errorf("助数詞 %q の %d に対する読み %q がカタカナではありません", surface, value, reading)
			}
		}
	}
}

// TestCounterIndexIsLongestFirst は、助数詞の索引が長い順に並んでいることを確認します。
// 短い順だと "ヶ月" より先に "月" が一致し、"3ヶ月" がサンヶガツになります。
func TestCounterIndexIsLongestFirst(t *testing.T) {
	for first, keys := range counterIndex {
		for i := 1; i < len(keys); i++ {
			if len(keys[i-1]) < len(keys[i]) {
				t.Errorf("先頭 %q のキーが長さ順ではありません: %v", string(first), keys)
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

// TestCounters_NoRedundantEntries は、辞書の読みをなぞるだけの助数詞が表に残って
// いないことを確認します。
//
// 音の変化も特殊読みもない助数詞は、表に載せなくても辞書の読みで正しく読めます。
// 載せても挙動は変わらないうえ、本当に補正が要る助数詞を見分けにくくします。
// 読み上書き辞書に対する TestDefaultReadingOverrides_NoRedundantEntries と同じ考え方です。
func TestCounters_NoRedundantEntries(t *testing.T) {
	tok, err := tokenizer.New(ipa.Dict(), tokenizer.OmitBosEos())
	if err != nil {
		t.Fatalf("failed to create tokenizer: %v", err)
	}

	// 促音便・連濁・特殊読みが起きうる代表的な数を通す。
	values := []uint64{1, 2, 3, 4, 6, 7, 8, 9, 10, 14, 20, 24, 100, 1000}

	for surface, counter := range counters {
		t.Run(surface, func(t *testing.T) {
			dictionary := dictionaryReadingOf(tok, surface)
			for _, value := range values {
				number := readNumeral(strconv.FormatUint(value, 10))
				if counter.read(number) != number.reading+dictionary {
					return // どれか1つでも辞書と違えば、この項目は仕事をしている。
				}
			}
			t.Errorf("助数詞 %q (%q) は全ての数で辞書の読み (%q) と一致します。辞書が正しく読めるので削除してください",
				surface, counter.reading, dictionary)
		})
	}
}

// dictionaryReadingOf は、表層形を単独で解析したときの読みを返します。
func dictionaryReadingOf(tok *tokenizer.Tokenizer, surface string) string {
	var sb strings.Builder
	for _, token := range tok.Tokenize(surface) {
		sb.WriteString(dictionaryReading(token))
	}
	return sb.String()
}
