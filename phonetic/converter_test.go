package phonetic

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ikawaha/kagome-dict/ipa"
	"github.com/ikawaha/kagome/v2/tokenizer"
)

func TestConverter_ConvertToReading(t *testing.T) {
	converter, err := NewConverter()
	if err != nil {
		t.Fatalf("failed to create converter: %v", err)
	}

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "ordinary kanji-kana text",
			input: "放課後のチャイムが鳴る",
			want:  "ホウカゴノチャイムガナル",
		},
		{
			name:  "reading of heiretsu",
			input: "並列回路のディストーション",
			want:  "ヘイレツカイロノディストーション",
		},
		{
			name:  "singing fix for particle ha",
			input: "私は閃光",
			want:  "ワタシワヒカリ",
		},
		{
			name:  "singing kanji reading fix",
			input: "荒野に刃と閃光",
			want:  "コウヤニヤイバトヒカリ",
		},
		{
			name:  "singing fix for particle he",
			input: "武道館へ行こう",
			want:  "ブドウカンエイコウ",
		},
		{
			name:  "singing fix for particle wo",
			input: "絆を奏でる",
			want:  "キズナオカナデル",
		},
		{
			name:  "greeting pronunciation exception",
			input: "こんにちは、こんばんは",
			want:  "コンニチワ、コンバンワ",
		},
		{
			name:  "keeps English tags",
			input: "[Verse] 昨日の空は",
			want:  "[Verse] キノウノソラワ",
		},
		{
			name:  "mixed katakana and alphanumerics",
			input: "BPM180で疾走する",
			want:  "BPM180デシッソウスル",
		},
		{
			name:  "keeps newlines",
			input: "絆の音\n響け",
			want:  "キズナノオト\nヒビケ",
		},
		{
			name:  "unknown word (ateji)",
			input: "夜露死苦",
			want:  "ヨロシク",
		},
		{
			name:  "greeting fix has no side effect",
			input: "コンニチハム",
			want:  "コンニチハム",
		},
		// The override for 「掌」 must not swallow and drop the 「握」 of 「掌握」.
		// Reading overrides take only matches that end on a morpheme boundary.
		//
		// This used to be checked with 「世界」 and 「世界観」, but the dictionary reads
		// 「世界」 as セカイ on its own, so that override was removed. Unless the check
		// uses an override that actually exists, the test passes even with broken
		// boundary handling.
		{
			name:  "override across a morpheme boundary is not applied",
			input: "掌握が変わる",
			want:  "ショウアクガカワル",
		},
		{
			name:  "override on a matching boundary is applied",
			input: "掌が熱い",
			want:  "テノヒラガアツイ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := converter.ConvertToReading(tt.input)
			if got != tt.want {
				t.Errorf("%s: ConvertToReading() = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}

func TestConverter_ConvertToReading_WithPhraseSpacing(t *testing.T) {
	converter, err := NewConverter(WithPhraseSpacing())
	if err != nil {
		t.Fatalf("failed to create converter: %v", err)
	}

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "space after particle ga",
			input: "空が青い",
			want:  "ソラガ アオイ",
		},
		{
			name:  "space after corrected particle ha",
			input: "私は歌う",
			want:  "ワタシワ ウタウ",
		},
		{
			name:  "space after corrected particle wo",
			input: "絆を奏でる",
			want:  "キズナオ カナデル",
		},
		// A run of particles and auxiliary verbs is one phrase: the space goes after the last
		// of them. A space after each one put a pause inside the phrase (ダレニ モ).
		{
			name:  "stacked particles ni mo stay together",
			input: "誰にも見せない",
			want:  "ダレニモ ミセナイ",
		},
		{
			name:  "stacked particles he to stay together",
			input: "夜の街へと走り出す",
			want:  "ヨルノ マチエト ハシリダス",
		},
		{
			name:  "stacked particles kara ha stay together",
			input: "ここからは歩く",
			want:  "ココカラワ アルク",
		},
		{
			name:  "conjunctive particle te mo stays together",
			input: "風が吹いても",
			want:  "カゼガ フイテモ",
		},
		{
			name:  "trailing particle space is trimmed",
			input: "夜の",
			want:  "ヨルノ",
		},
		{
			name:  "line-end particle space is trimmed per line",
			input: "夜の\n朝が来る",
			want:  "ヨルノ\nアサガ クル",
		},
		// Applying the overrides first would cut the input so that the remaining 「が」 is
		// analysed alone, taken as a conjunction rather than a particle, and loses its
		// phrase space.
		{
			name:  "space after a particle between overrides",
			input: "運命の閃光が瞳を灼く",
			want:  "サダメノ ヒカリガ ヒトミオ ヤク",
		},
		// Adjacent overrides. This used to be checked with 「絆と翼を」, but the dictionary
		// reads both correctly, so those overrides were removed. Adjacency can only be
		// checked with words that actually have overrides.
		{
			name:  "adjacent overrides keep phrases",
			input: "刃と閃光を",
			want:  "ヤイバト ヒカリオ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := converter.ConvertToReading(tt.input)
			if got != tt.want {
				t.Errorf("%s: ConvertToReading() = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}

func TestConverter_ConvertToReading_WithReadingOverrides(t *testing.T) {
	converter, err := NewConverter(WithReadingOverrides(map[string]string{
		"閃光": "センコウ",
	}))
	if err != nil {
		t.Fatalf("failed to create converter: %v", err)
	}

	got := converter.ConvertToReading("私は閃光")
	want := "ワタシワセンコウ"
	if got != want {
		t.Errorf("ConvertToReading() = %q, want %q", got, want)
	}
}

// TestDefaultReadingOverridesJSON_NoDuplicateKeys checks that the bundled JSON has no
// duplicate keys. A duplicate JSON key is not an error; the later one silently wins, so
// as entries keep being appended one of them dies unnoticed.
func TestDefaultReadingOverridesJSON_NoDuplicateKeys(t *testing.T) {
	dec := json.NewDecoder(bytes.NewReader(defaultReadingOverridesJSON))
	if _, err := dec.Token(); err != nil { // the opening '{'
		t.Fatalf("failed to read JSON: %v", err)
	}

	seen := make(map[string]bool)
	for dec.More() {
		keyToken, err := dec.Token()
		if err != nil {
			t.Fatalf("failed to read JSON key: %v", err)
		}
		key, ok := keyToken.(string)
		if !ok {
			t.Fatalf("unexpected JSON token: %v", keyToken)
		}
		if seen[key] {
			t.Errorf("duplicate key %q; the later entry overwrites the earlier one", key)
		}
		seen[key] = true

		if _, err := dec.Token(); err != nil { // skip the value
			t.Fatalf("failed to read JSON value: %v", err)
		}
	}
}

func TestDefaultReadingOverrides_Validation(t *testing.T) {
	overrides, err := loadReadingOverridesJSON(defaultReadingOverridesJSON)
	if err != nil {
		t.Fatalf("failed to load embedded reading overrides JSON: %v", err)
	}
	if len(overrides) == 0 {
		t.Fatal("embedded reading overrides should not be empty")
	}
}

// TestDefaultReadingOverrides_KeepsPlainCompoundsUnoverridden checks that compounds the
// dictionary reads correctly get no ateji reading.
//
// At one point an override made 「設計図」 read 「レシピ」. Ateji is not a reading but a
// substitution of the word, so the meaning a user intended by writing 「未来の設計図」 in a
// lyric is lost. Add ateji only for words that circulate with that reading in that
// spelling (運命→サダメ and the like).
func TestDefaultReadingOverrides_KeepsPlainCompoundsUnoverridden(t *testing.T) {
	converter, err := NewConverter()
	if err != nil {
		t.Fatalf("failed to create converter: %v", err)
	}

	tests := []struct {
		input string
		want  string
	}{
		{input: "設計図", want: "セッケイズ"},
		{input: "未来の設計図", want: "ミライノセッケイズ"},
		// At one point an override made 「音響」 read アコースティック. アコースティック
		// means "of acoustic instruments", which is not what 音響 refers to. Ateji also
		// leaks into derived words (音響効果→アコースティックコウカ). Ateji is allowed only
		// for words that circulate with that reading in that spelling (運命→サダメ,
		// 永遠→トワ).
		{input: "音響", want: "オンキョウ"},
		{input: "音響効果", want: "オンキョウコウカ"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := converter.ConvertToReading(tt.input); got != tt.want {
				t.Errorf("ConvertToReading(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestDefaultReadingOverrides_FixesMisreadings checks that spellings to which the
// dictionary assigns another word's reading are read correctly through overrides.
//
// The dictionary takes 「瞬く」 as 「しばたく」, so written in a lyric it is sung シバタク.
// The continuative form breaks further into シバタタイテ. Keying an inflecting word by its
// stem catches all its inflected forms at once.
func TestDefaultReadingOverrides_FixesMisreadings(t *testing.T) {
	converter, err := NewConverter()
	if err != nil {
		t.Fatalf("failed to create converter: %v", err)
	}

	tests := []struct {
		input string
		want  string
	}{
		{input: "瞬く信号", want: "マタタクシンゴウ"},
		{input: "此の建物", want: "コノタテモノ"},
		{input: "薪を焚べる", want: "タキギオクベル"},
		{input: "天穹の観測", want: "テンキュウノカンソク"},
		{input: "燦たる成果", want: "サンタルセイカ"},
		{input: "爆速で処理する", want: "バクソクデショリスル"},
		{input: "馬が奔る", want: "ウマガハシル"},
		{input: "守りを固める", want: "マモリオカタメル"},
		{input: "種を宿す", want: "タネオヤドス"},
		// The dictionary returns 「ワレル」 even for 「割る」 alone (dragged by the
		// intransitive verb's reading). In a real take (`music-20260904-180324-15a54300f4a5`)
		// it was sung as 「割れる」.
		{input: "高架の下を割る", want: "コウカノシタオワル"},
		// 「露」 alone reads 「ロ」. Compounds (朝露, 露出) are correct, so only the standalone form
		// is overridden.
		{input: "搾った露は", want: "シボッタツユワ"},
		{input: "電球が瞬いている", want: "デンキュウガマタタイテイル"},
		{input: "水面が揺蕩う", want: "スイメンガタユタウ"},
		{input: "彷徨いを続ける", want: "サマヨイオツヅケル"},
		{input: "細部に拘って作る", want: "サイブニコダワッテツクル"},
		{input: "部下を労る", want: "ブカオイタワル"},
		{input: "町が黄昏れる", want: "マチガタソガレル"},
		{input: "空が黄昏れて", want: "ソラガタソガレテ"},
		{input: "黄昏の町", want: "タソガレノマチ"},
		{input: "此方に置く", want: "コチラニオク"},
		{input: "僕等と我等", want: "ボクラトワレラ"},
		{input: "夜風が入る", want: "ヨカゼガハイル"},
		{input: "一途な性格", want: "イチズナセイカク"},
		{input: "十六夜の月", want: "イザヨイノツキ"},
		{input: "掌を返す", want: "テノヒラオカエス"},
		// 燦めく splits into 燦|めく and becomes 燦メク (plain dictionary) / サンメク (with the
		// 燦→サン override)
		{input: "燦めく星座", want: "キラメクセイザ"},
		{input: "夜空に燦めきを", want: "ヨゾラニキラメキオ"},
		{input: "星が燦めいて", want: "ホシガキラメイテ"},
		// The dictionary reads 誰そ彼 as ダレソカレ. With 時 attached, rendaku makes it ドキ
		{input: "誰そ彼の空", want: "タソガレノソラ"},
		{input: "誰そ彼時の街", want: "タソガレドキノマチ"},
		// 仄暗い splits into 仄|暗い and becomes 仄クライ
		{input: "仄暗い部屋で", want: "ホノグライヘヤデ"},
		{input: "仄暗く沈む", want: "ホノグラクシズム"},
		// The dictionary has no reading for 檸檬, so it stays in kanji
		{input: "檸檬の香り", want: "レモンノカオリ"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := converter.ConvertToReading(tt.input); got != tt.want {
				t.Errorf("ConvertToReading(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestDefaultReadingOverrides_FixesImperatives checks that imperatives spelled like a
// suffix or another verb are not garbled.
//
// 「立て」 alone is analysed as the suffix ダテ (as in 二本立て), 「打て」 right after a
// symbol turns into ブテ from 打つ read ブツ, and 「絶て」 right after a noun splits into
// 絶っ＋て (ゼッテ). In lyrics imperatives often stand alone or at the start of a line,
// where these garblings stand out.
func TestDefaultReadingOverrides_FixesImperatives(t *testing.T) {
	converter, err := NewConverter()
	if err != nil {
		t.Fatalf("failed to create converter: %v", err)
	}

	tests := []struct {
		input string
		want  string
	}{
		{input: "立て", want: "タテ"},
		{input: "さあ立て", want: "サアタテ"},
		{input: "君よ立て", want: "キミヨタテ"},
		{input: "[Chorus] 打て", want: "[Chorus] ウテ"},
		{input: "打て…", want: "ウテ…"},
		// The 絶て override fixes the ゼッテ garbling. That the preceding 「今」 is read
		// コン, dragged by the noun-sequence analysis, comes from the lattice and cannot
		// be fixed by an override.
		{input: "今絶て", want: "コンタテ"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := converter.ConvertToReading(tt.input); got != tt.want {
				t.Errorf("ConvertToReading(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestDefaultReadingOverrides_KeepsCompoundsIntact checks that overrides for kun readings
// do not break on-reading compounds containing the same kanji.
//
// Short overrides such as 「瞬く」「拘る」「掌」「燈」 are protected by taking only matches
// that end on a morpheme boundary. If the boundary check broke, 一瞬 would collapse into
// something like イチマタタク.
func TestDefaultReadingOverrides_KeepsCompoundsIntact(t *testing.T) {
	converter, err := NewConverter()
	if err != nil {
		t.Fatalf("failed to create converter: %v", err)
	}

	tests := []struct {
		input string
		want  string
	}{
		{input: "一瞬", want: "イッシュン"},
		{input: "瞬間", want: "シュンカン"},
		{input: "拘束", want: "コウソク"},
		{input: "掌握", want: "ショウアク"},
		{input: "燈台", want: "トウダイ"},
		{input: "動揺", want: "ドウヨウ"},
		{input: "黄金", want: "オウゴン"},
		{input: "苦労", want: "クロウ"},
		{input: "燦然と輝く", want: "サンゼントカガヤク"},
		{input: "爆発の速度", want: "バクハツノソクド"},
		{input: "奔流に飲まれ", want: "ホンリュウニノマレ"},
		{input: "天空の城", want: "テンクウノシロ"},
		{input: "お守りを買う", want: "オマモリオカウ"},
		// The 「露」 and 「割る」 overrides must not reach compounds and inflected forms.
		{input: "露出", want: "ロシュツ"},
		{input: "朝露", want: "アサツユ"},
		{input: "夜露に濡れる", want: "ヨツユニヌレル"},
		{input: "割れる", want: "ワレル"},
		{input: "割った", want: "ワッタ"},
		{input: "国境を守り抜く", want: "コッキョウオマモリヌク"},
		{input: "種を宿した苗", want: "タネオヤドシタナエ"},
		// Overrides are matched only from the start of a token. 「非同期」 is one token,
		// so the 「同期」 inside it is never matched and the override (同期→シンクロ) does
		// not fire. There used to be an override just for 「非同期」 making it ヒドウキ,
		// but the dictionary already reads it that way, so it was unnecessary. If this
		// property broke the result would be ヒシンクロ, so it is pinned here as the
		// safeguard for removing that override.
		{input: "非同期", want: "ヒドウキ"},
		{input: "非同期処理", want: "ヒドウキショリ"},
		// The 仄暗い override must not drag in 仄か, which the dictionary reads correctly
		{input: "仄かな光", want: "ホノカナヒカリ"},
		// The 立て→タテ override must not drag in compounds with the suffix ダテ or compounds read
		// differently
		{input: "二本立て", want: "ニホンダテ"},
		{input: "献立", want: "コンダテ"},
		{input: "旅立て", want: "タビダテ"},
		{input: "仕立て", want: "シタテ"},
		{input: "夕立", want: "ユウダチ"},
		{input: "打ち上げ", want: "ウチアゲ"},
		{input: "博打", want: "バクチ"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := converter.ConvertToReading(tt.input); got != tt.want {
				t.Errorf("ConvertToReading(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestDefaultReadingOverrides_NoRedundantEntries checks that no override sits on a word
// the dictionary already reads correctly.
//
// The overrides are valuable as a list of the words the dictionary and the number rules
// get wrong. Entries restating the same reading keep piling up without changing behaviour
// and make the genuinely dangerous words harder to spot. In fact, at one point 51 of 120
// entries matched the dictionary.
//
// The baseline includes the number rules (WithNumberReading), so that words the rules
// read correctly, such as 「一本」, stay out of the overrides and only words the rules
// misread (「一日」 → イチニチ) go in.
//
// The comparison runs in several contexts, not only on the bare word. Some words, such as
// 「何時」, are read イツ by the dictionary alone but ナンジ mid-sentence, and a bare
// comparison would judge even a needed override redundant.
func TestDefaultReadingOverrides_NoRedundantEntries(t *testing.T) {
	tok, err := tokenizer.New(ipa.Dict(), tokenizer.OmitBosEos())
	if err != nil {
		t.Fatalf("failed to create tokenizer: %v", err)
	}
	plain := &Converter{t: tok, readingOverrides: map[string]string{}, numberReading: true}
	plain.rebuildOverrideKeys()

	overridden, err := NewConverter(WithNumberReading())
	if err != nil {
		t.Fatalf("failed to create converter: %v", err)
	}

	// "[Chorus] %s" is the context of an English tag on the same line. Right after a
	// symbol or unknown word the lattice path changes, and a word read correctly on its own
	// can split (重なる → 重/なる, read オモナル). The line-initial context after a newline
	// is absorbed by splitting into lines, so only the same-line position after a symbol
	// needs checking here.
	// "今%s" is the context of a preceding noun. The misreading of an imperative as a
	// suffix (今絶て → コンゼッテ) appears only in this form.
	contexts := []string{
		"%s", "%sが好き", "%sを抱いて", "%sの向こう", "君の%s",
		"遠い%sへ", "%sだけが残る", "静かな%sと", "%sは終わらない",
		"[Chorus] %s", "今%s",
	}

	for surface := range defaultReadingOverrides {
		t.Run(surface, func(t *testing.T) {
			for _, format := range contexts {
				input := strings.ReplaceAll(format, "%s", surface)
				if plain.ConvertToReading(input) != overridden.ConvertToReading(input) {
					return // If any one differs from the dictionary, this override is doing its job.
				}
			}
			t.Errorf("override %q (%q) matches the dictionary reading in every context; the dictionary reads it correctly, so remove it",
				surface, defaultReadingOverrides[surface])
		})
	}
}

// TestConverter_ConvertToReading_LineInitialWords checks that the reading of a
// line-initial word does not change after a newline or an English tag.
//
// Analysing across newlines lets a line-initial word be dragged by the context of the
// preceding symbols or unknown words and split differently (「重なる」 splits into 重/なる,
// read オモナル). Analysing the input line by line prevents this. When a symbol is on the
// same line, a reading override corrects it.
func TestConverter_ConvertToReading_LineInitialWords(t *testing.T) {
	converter, err := NewConverter()
	if err != nil {
		t.Fatalf("failed to create converter: %v", err)
	}

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "right after a tag line",
			input: "[Chorus]\n重なる鼓動",
			want:  "[Chorus]\nカサナルコドウ",
		},
		{
			name:  "right after a leading newline",
			input: "\n重なる",
			want:  "\nカサナル",
		},
		{
			name:  "across lines",
			input: "声が\n重なる",
			want:  "コエガ\nカサナル",
		},
		{
			name:  "after a tag on the same line is fixed by override",
			input: "[Chorus] 重なる鼓動",
			want:  "[Chorus] カサナルコドウ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := converter.ConvertToReading(tt.input); got != tt.want {
				t.Errorf("ConvertToReading(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestHiraganaToKatakana(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"ぴえん", "ピエン"},
		{"abcぴえん123", "abcピエン123"},
		{"ピエン", "ピエン"},
		{"ゔ", "ヴ"},
		{"漢字とかな", "漢字トカナ"},
		{"、。ー[Verse]", "、。ー[Verse]"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := hiraganaToKatakana(tt.input); got != tt.want {
				t.Errorf("hiraganaToKatakana(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestConverter_ConvertToReading_NormalizesUnknownHiragana checks that the surface-form
// fallback for words the dictionary cannot read is not output as hiragana.
// The output contract is katakana, so hiragana in unknown words is normalized to katakana.
func TestConverter_ConvertToReading_NormalizesUnknownHiragana(t *testing.T) {
	converter, err := NewConverter()
	if err != nil {
		t.Fatalf("failed to create converter: %v", err)
	}

	for _, input := range []string{"ぴえんな夜", "うぇーいと叫ぶ"} {
		t.Run(input, func(t *testing.T) {
			got := converter.ConvertToReading(input)
			if strings.ContainsFunc(got, isConvertibleHiragana) {
				t.Errorf("ConvertToReading(%q) = %q, which still contains hiragana", input, got)
			}
		})
	}
}

// Pins the contract that words whose reading the caller has decided are passed in katakana
// (the music generation service writes furigana in katakana before passing it on). Passed
// in hiragana, 「へ」 and 「を」 are split off as particles, which breaks the reading and
// changes the following word's reading too, so this checks that a katakana word changes
// neither its own reading nor the words around it.
func TestConverter_ConvertToReading_KeepsKatakanaReadings(t *testing.T) {
	converter, err := NewConverter()
	if err != nil {
		t.Fatalf("failed to create converter: %v", err)
	}

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "word starting with katakana he", input: "流れる星とヘイソウする", want: "ナガレルホシトヘイソウスル"},
		{name: "following word reading unchanged", input: "ヘイワな街", want: "ヘイワナマチ"},
		{name: "right after a particle", input: "君はヘイキだと言った", want: "キミワヘイキダトイッタ"},
		{name: "word starting with katakana wo", input: "ヲトメの祈り", want: "ヲトメノイノリ"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := converter.ConvertToReading(tt.input); got != tt.want {
				t.Errorf("ConvertToReading(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestConverter_WithoutDefaultReadingOverrides(t *testing.T) {
	withDefaults, err := NewConverter()
	if err != nil {
		t.Fatalf("failed to create converter: %v", err)
	}
	without, err := NewConverter(WithoutDefaultReadingOverrides())
	if err != nil {
		t.Fatalf("failed to create converter: %v", err)
	}

	const input = "こんにちは"
	if got := withDefaults.ConvertToReading(input); got != "コンニチワ" {
		t.Errorf("with default overrides = %q, want コンニチワ", got)
	}
	if got := without.ConvertToReading(input); got != "コンニチハ" {
		t.Errorf("without default overrides = %q, want コンニチハ (the dictionary reading)", got)
	}

	// Custom overrides alone can be loaded once the defaults are dropped (options apply in order)
	custom, err := NewConverter(
		WithoutDefaultReadingOverrides(),
		WithReadingOverrides(map[string]string{"こんにちは": "チーッス"}),
	)
	if err != nil {
		t.Fatalf("failed to create converter: %v", err)
	}
	if got := custom.ConvertToReading(input); got != "チーッス" {
		t.Errorf("custom override = %q, want チーッス", got)
	}
}

func TestLoadReadingOverridesJSON_RejectsEmptyValues(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{
			name: "empty surface",
			data: []byte(`{"": "コウヤ"}`),
		},
		{
			name: "empty reading",
			data: []byte(`{"守護神": ""}`),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := loadReadingOverridesJSON(tt.data); err == nil {
				t.Fatal("loadReadingOverridesJSON() error = nil, want error")
			}
		})
	}
}

func BenchmarkConverter_ConvertToReading(b *testing.B) {
	// Error handling for the setup before the benchmark runs
	converter, err := NewConverter()
	if err != nil {
		b.Fatalf("failed to create converter: %v", err)
	}
	input := "長い放課後の廊下を全力で疾走する少女たちは、武道館のステージへ向かって絆を奏でる。"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		converter.ConvertToReading(input)
	}
}

// TestConverter_ConvertToReading_PreservesLineTerminators checks that line terminators are
// carried over as in the input and that the \r of a CRLF does not leak into the reading.
//
// Analysing \r as part of the line leaks it into the reading as an unknown word, and the
// \r also blocks the trailing trim of phrase spacing, leaving a space as in
// 「ソラガ \r\n」.
func TestConverter_ConvertToReading_PreservesLineTerminators(t *testing.T) {
	tests := []struct {
		name    string
		options []Option
		input   string
		want    string
	}{
		{
			name:  "LF",
			input: "空が\n青い",
			want:  "ソラガ\nアオイ",
		},
		{
			name:  "CRLF",
			input: "空が\r\n青い",
			want:  "ソラガ\r\nアオイ",
		},
		{
			name:    "CRLF with phrase spacing",
			options: []Option{WithPhraseSpacing()},
			input:   "空が\r\n青い",
			want:    "ソラガ\r\nアオイ",
		},
		{
			name:  "trailing newline",
			input: "空が青い\n",
			want:  "ソラガアオイ\n",
		},
		{
			name:  "blank line",
			input: "空\n\n青",
			want:  "ソラ\n\nアオ",
		},
		{
			name:  "no newline",
			input: "空が青い",
			want:  "ソラガアオイ",
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			converter, err := NewConverter(tt.options...)
			if err != nil {
				t.Fatalf("NewConverter() error = %v", err)
			}
			if got := converter.ConvertToReading(tt.input); got != tt.want {
				t.Errorf("ConvertToReading(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestConverter_WithReadingOverridesJSON(t *testing.T) {
	t.Run("adds overrides from JSON", func(t *testing.T) {
		converter, err := NewConverter(WithReadingOverridesJSON([]byte(`{"閃光": "センコウ"}`)))
		if err != nil {
			t.Fatalf("NewConverter() error = %v", err)
		}
		if got, want := converter.ConvertToReading("私は閃光"), "ワタシワセンコウ"; got != want {
			t.Errorf("ConvertToReading() = %q, want %q", got, want)
		}
	})

	t.Run("combines with default overrides", func(t *testing.T) {
		converter, err := NewConverter(WithReadingOverridesJSON([]byte(`{"閃光": "センコウ"}`)))
		if err != nil {
			t.Fatalf("NewConverter() error = %v", err)
		}
		if got, want := converter.ConvertToReading("こんにちは"), "コンニチワ"; got != want {
			t.Errorf("ConvertToReading() = %q, want %q", got, want)
		}
	})

	// An Option cannot return a value, so NewConverter returns the error. Silently ignoring
	// it would produce a Converter whose overrides do not work, without anyone noticing the
	// dictionary failed to load.
	t.Run("broken JSON makes NewConverter fail", func(t *testing.T) {
		tests := []struct {
			name string
			data string
		}{
			{"invalid JSON", `{`},
			{"empty key", `{"": "センコウ"}`},
			{"empty value", `{"閃光": ""}`},
			{"non-string value", `{"閃光": 1}`},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				converter, err := NewConverter(WithReadingOverridesJSON([]byte(tt.data)))
				if err == nil {
					t.Fatal("NewConverter() error = nil, want error")
				}
				if converter != nil {
					t.Errorf("NewConverter() = %v, want nil", converter)
				}
			})
		}
	})
}
