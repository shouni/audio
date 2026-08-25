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
			name:  "通常の漢字まじり文",
			input: "放課後のチャイムが鳴る",
			want:  "ホウカゴノチャイムガナル",
		},
		{
			name:  "「並列」の読みチェック",
			input: "並列回路のディストーション",
			want:  "ヘイレツカイロノディストーション",
		},
		{
			name:  "助詞「は」の歌唱補正",
			input: "私は閃光",
			want:  "ワタシワヒカリ",
		},
		{
			name:  "歌唱用の漢字読み補正",
			input: "荒野に刃と閃光",
			want:  "コウヤニヤイバトヒカリ",
		},
		{
			name:  "助詞「へ」の歌唱補正",
			input: "武道館へ行こう",
			want:  "ブドウカンエイコウ",
		},
		{
			name:  "助詞「を」の歌唱補正",
			input: "絆を奏でる",
			want:  "キズナオカナデル",
		},
		{
			name:  "挨拶の発音例外",
			input: "こんにちは、こんばんは",
			want:  "コンニチワ、コンバンワ",
		},
		{
			name:  "英語タグの保持",
			input: "[Verse] 昨日の空は",
			want:  "[Verse] キノウノソラワ",
		},
		{
			name:  "カタカナ・英数字の混在",
			input: "BPM180で疾走する",
			want:  "BPM180デシッソウスル",
		},
		{
			name:  "改行の保持",
			input: "絆の音\n響け",
			want:  "キズナノオト\nヒビケ",
		},
		{
			name:  "未知語（当て字）の挙動",
			input: "夜露死苦",
			want:  "ヨロシク",
		},
		{
			name:  "挨拶補正の副作用回避",
			input: "コンニチハム",
			want:  "コンニチハム",
		},
		// 形態素解析器は「一歩」を 一/歩 に分割して辞書読みを連結するため「イチホ」になる。
		// 促音便は解析側では直せないので読み上書きで補正する。
		{
			name:  "一+助数詞の促音便",
			input: "僕が一歩を踏み出す",
			want:  "ボクガイッポオフミダス",
		},
		{
			name:  "促音便の上書きは複合語を壊さない",
			input: "一本気な一匹狼",
			want:  "イッポンギナイッピキオオカミ",
		},
		// 「掌」の上書きが「掌握」の「握」を巻き込んで欠落させないこと。
		// 読み上書きは形態素境界で終わる一致だけを採用する。
		//
		// かつては「世界」と「世界観」で確かめていたが、「世界」は辞書がそのまま
		// セカイと読むため上書きを削除した。実在する上書きで確かめないと、
		// 境界判定が壊れてもテストが素通りする。
		{
			name:  "形態素境界をまたぐ上書きは適用しない",
			input: "掌握が変わる",
			want:  "ショウアクガカワル",
		},
		{
			name:  "境界が一致すれば上書きを適用する",
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
			name:  "助詞「が」の後にスペース",
			input: "空が青い",
			want:  "ソラガ アオイ",
		},
		{
			name:  "助詞「は」補正後にスペース",
			input: "私は歌う",
			want:  "ワタシワ ウタウ",
		},
		{
			name:  "助詞「を」補正後にスペース",
			input: "絆を奏でる",
			want:  "キズナオ カナデル",
		},
		{
			name:  "末尾の助詞後スペースはトリム",
			input: "夜の",
			want:  "ヨルノ",
		},
		{
			name:  "行末の助詞後スペースは行ごとにトリム",
			input: "夜の\n朝が来る",
			want:  "ヨルノ\nアサガ クル",
		},
		// 読み上書きを先に適用して入力を分断すると、残った「が」が単独で解析されて
		// 助詞ではなく接続詞と判定され、文節スペースが落ちる。
		{
			name:  "読み上書きに挟まれた助詞の後にもスペース",
			input: "運命の閃光が瞳を灼く",
			want:  "サダメノ ヒカリガ ヒトミオ ヤク",
		},
		// 上書きが隣り合う場合。「絆と翼を」で確かめていたが、どちらも辞書が正しく
		// 読むため上書きを削除した。上書きが実在する語でないと連続を確かめられない。
		{
			name:  "読み上書きが連続しても文節が保たれる",
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

// TestDefaultReadingOverridesJSON_NoDuplicateKeys は、同梱JSONに重複キーがないことを
// 確認します。JSON の重複キーはエラーにならず後勝ちで上書きされるため、
// 追記を重ねる運用では気付かないまま片方が死にます。
func TestDefaultReadingOverridesJSON_NoDuplicateKeys(t *testing.T) {
	dec := json.NewDecoder(bytes.NewReader(defaultReadingOverridesJSON))
	if _, err := dec.Token(); err != nil { // 先頭の '{'
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
			t.Errorf("重複キー %q があります。後のエントリが前のエントリを上書きします", key)
		}
		seen[key] = true

		if _, err := dec.Token(); err != nil { // 値を読み飛ばす
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

// TestDefaultReadingOverrides_KeepsPlainCompoundsUnoverridden は、辞書が正しく読める
// 複合語に当て字を当てないことを確認します。
//
// 「設計図」を「レシピ」と読ませる上書きが入っていたことがあります。当て字は読みではなく
// 語の置き換えなので、歌詞に「未来の設計図」と書いた利用者の意図した意味が失われます。
// 当て字を足すなら、その語が当て字表記で流通しているもの（運命→サダメ 等）に限ります。
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
		// 「音響」をアコースティックと読ませる上書きが入っていたことがあります。
		// アコースティックは「生楽器の」の意味で、音響とは指すものが違います。
		// 加えて当て字は派生語に漏れます（音響効果→アコースティックコウカ）。
		// 当て字が許されるのは、その表記でその読みが流通している語（運命→サダメ、
		// 永遠→トワ）に限ります。
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

// TestDefaultReadingOverrides_FixesMisreadings は、辞書が別語の読みを当ててしまう表記を
// 上書きで正しく読ませることを確認します。
//
// 「瞬く」は辞書が「しばたく」を採用するため、歌詞に書くと シバタク と歌われます。連用形は
// さらに崩れて シバタタイテ になります。活用形は語幹をキーにすると まとめて拾えます。
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
		// 燦めく は 燦|めく に割れて 燦メク (素の辞書) / サンメク (燦→サン 上書き) になる
		{input: "燦めく星座", want: "キラメクセイザ"},
		{input: "夜空に燦めきを", want: "ヨゾラニキラメキオ"},
		{input: "星が燦めいて", want: "ホシガキラメイテ"},
		// 誰そ彼 は辞書が ダレソカレ と読む。時が付くと連濁で ドキ になる
		{input: "誰そ彼の空", want: "タソガレノソラ"},
		{input: "誰そ彼時の街", want: "タソガレドキノマチ"},
		// 仄暗い は 仄|暗い に割れて 仄クライ になる
		{input: "仄暗い部屋で", want: "ホノグライヘヤデ"},
		{input: "仄暗く沈む", want: "ホノグラクシズム"},
		// 檸檬 は辞書に読みがなく漢字のまま残る
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

// TestDefaultReadingOverrides_FixesImperatives は、接尾辞や別動詞と同形の命令形が
// 化けないことを確認します。
//
// 「立て」は単独だと接尾辞の ダテ（二本立て等）として解析され、「打て」は記号の
// 直後で 打つ（ブツ）側の ブテ に化け、「絶て」は名詞の直後で 絶っ＋て（ゼッテ）に
// 割れます。命令形は歌詞では行頭・単独で置かれることが多く、この化け方は目立ちます。
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
		// 絶て の上書きで ゼッテ 化けは直る。直前の「今」が名詞連続の解析に
		// 引きずられて コン と読まれる問題はラティス由来で、上書きでは直せない。
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

// TestDefaultReadingOverrides_KeepsCompoundsIntact は、訓読み用の上書きが同じ漢字を含む
// 音読みの熟語を壊さないことを確認します。
//
// 「瞬く」「拘る」「掌」「燈」のような短い上書きは、形態素境界で終わる一致だけを採用する
// 仕組みに守られています。境界判定が壊れると 一瞬 が イチマタタク のように崩れます。
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
		{input: "国境を守り抜く", want: "コッキョウオマモリヌク"},
		{input: "種を宿した苗", want: "タネオヤドシタナエ"},
		// 上書きの照合はトークンの開始位置からしか行わない。「非同期」は1トークンなので
		// 内側の「同期」は照合対象にならず、上書き (同期→シンクロ) が発火しない。
		// かつては「非同期」専用の上書きでヒドウキにしていたが、辞書がそう読むため
		// 不要だった。この性質が崩れるとヒシンクロになるので、上書きを削除した側の
		// 保険としてここで固定する。
		{input: "非同期", want: "ヒドウキ"},
		{input: "非同期処理", want: "ヒドウキショリ"},
		// 仄暗い の上書きが、辞書が正しく読める 仄か を巻き込まないこと
		{input: "仄かな光", want: "ホノカナヒカリ"},
		// 立て→タテ の上書きが、接尾辞 ダテ の複合語や別読みの熟語を巻き込まないこと
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

// TestDefaultReadingOverrides_NoRedundantEntries は、辞書が既に正しく読める語に
// 上書きを置いていないことを確認します。
//
// 上書きは「辞書が間違える語」の一覧であることに価値があります。辞書と同じ読みを
// 書いたエントリは挙動を変えないまま増え続け、本当に危ない語を見分けられなくします。
// 実際、一度は 120 件中 51 件が辞書と同一でした。
//
// 単語単体だけでなく複数の文脈で比較します。「何時」のように、単体では辞書も
// イツと読むが文中ではナンジになる語があり、単体比較だけでは必要な上書きまで
// 不要と判定してしまうためです。
func TestDefaultReadingOverrides_NoRedundantEntries(t *testing.T) {
	tok, err := tokenizer.New(ipa.Dict(), tokenizer.OmitBosEos())
	if err != nil {
		t.Fatalf("failed to create tokenizer: %v", err)
	}
	plain := &Converter{t: tok, readingOverrides: map[string]string{}}
	plain.rebuildOverrideKeys()

	overridden, err := NewConverter()
	if err != nil {
		t.Fatalf("failed to create converter: %v", err)
	}

	// "[Chorus] %s" は同一行に英語タグがある文脈。記号・未知語の直後は
	// ラティスの経路が変わり、単独では正しく読める語が割れることがある
	// （重なる → 重/なる でオモナル）。行頭の改行文脈は行分割で吸収されるため、
	// ここで確かめるのは同一行の記号直後だけでよい。
	// "今%s" は名詞が直前に来る文脈。命令形が接尾辞として解釈される誤読
	// （今絶て → コンゼッテ）はこの形でだけ現れる。
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
					return // どれか1つでも辞書と違えば、この上書きは仕事をしている。
				}
			}
			t.Errorf("上書き %q (%q) は全文脈で辞書の読みと一致します。辞書が正しく読めるので削除してください",
				surface, defaultReadingOverrides[surface])
		})
	}
}

// TestConverter_ConvertToReading_LineInitialWords は、改行や英語タグの直後でも
// 行頭の語の読みが変わらないことを確認します。
//
// 改行を跨いで一括解析すると、行頭の語が直前の記号・未知語の文脈に引きずられて
// 分割が変わります（「重なる」が 重/なる に割れてオモナルになる）。入力を行ごとに
// 解析することで防いでいます。同一行に記号がある場合は読み上書きで補正します。
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
			name:  "タグ行の直後",
			input: "[Chorus]\n重なる鼓動",
			want:  "[Chorus]\nカサナルコドウ",
		},
		{
			name:  "行頭の改行直後",
			input: "\n重なる",
			want:  "\nカサナル",
		},
		{
			name:  "行またぎ",
			input: "声が\n重なる",
			want:  "コエガ\nカサナル",
		},
		{
			name:  "同一行のタグ直後は読み上書きで補正",
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

// TestConverter_ConvertToReading_NormalizesUnknownHiragana は、辞書に読みがない語の
// 表層形フォールバックがひらがなのまま出力されないことを確認します。
// 出力の契約はカタカナなので、未知語のひらがなはカタカナへ正規化されます。
func TestConverter_ConvertToReading_NormalizesUnknownHiragana(t *testing.T) {
	converter, err := NewConverter()
	if err != nil {
		t.Fatalf("failed to create converter: %v", err)
	}

	for _, input := range []string{"ぴえんな夜", "うぇーいと叫ぶ"} {
		t.Run(input, func(t *testing.T) {
			got := converter.ConvertToReading(input)
			if strings.ContainsFunc(got, isConvertibleHiragana) {
				t.Errorf("ConvertToReading(%q) = %q にひらがなが残っています", input, got)
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
		t.Errorf("標準上書きあり = %q, want コンニチワ", got)
	}
	if got := without.ConvertToReading(input); got != "コンニチハ" {
		t.Errorf("標準上書きなし = %q, want コンニチハ (辞書読みのまま)", got)
	}

	// 標準を外した上で独自の上書きだけを載せられること（Option は指定順に適用）
	custom, err := NewConverter(
		WithoutDefaultReadingOverrides(),
		WithReadingOverrides(map[string]string{"こんにちは": "チーッス"}),
	)
	if err != nil {
		t.Fatalf("failed to create converter: %v", err)
	}
	if got := custom.ConvertToReading(input); got != "チーッス" {
		t.Errorf("独自上書き = %q, want チーッス", got)
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
	// ベンチマーク実行前の初期化時におけるエラーハンドリング
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

// TestConverter_ConvertToReading_PreservesLineTerminators は、改行文字が入力のまま
// 引き継がれ、CRLF の \r が読みに混ざらないことを確認します。
//
// \r を行の一部として解析すると未知語として読みへ漏れ、文節スペースの末尾トリムも
// \r に阻まれて「ソラガ \r\n」のように空白が残ります。
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
			name:    "CRLFと文節スペース",
			options: []Option{WithPhraseSpacing()},
			input:   "空が\r\n青い",
			want:    "ソラガ\r\nアオイ",
		},
		{
			name:  "末尾の改行",
			input: "空が青い\n",
			want:  "ソラガアオイ\n",
		},
		{
			name:  "空行",
			input: "空\n\n青",
			want:  "ソラ\n\nアオ",
		},
		{
			name:  "改行なし",
			input: "空が青い",
			want:  "ソラガアオイ",
		},
		{
			name:  "空文字",
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
	t.Run("JSONから読み上書きを追加できること", func(t *testing.T) {
		converter, err := NewConverter(WithReadingOverridesJSON([]byte(`{"閃光": "センコウ"}`)))
		if err != nil {
			t.Fatalf("NewConverter() error = %v", err)
		}
		if got, want := converter.ConvertToReading("私は閃光"), "ワタシワセンコウ"; got != want {
			t.Errorf("ConvertToReading() = %q, want %q", got, want)
		}
	})

	t.Run("標準の上書きと併用できること", func(t *testing.T) {
		converter, err := NewConverter(WithReadingOverridesJSON([]byte(`{"閃光": "センコウ"}`)))
		if err != nil {
			t.Fatalf("NewConverter() error = %v", err)
		}
		if got, want := converter.ConvertToReading("こんにちは"), "コンニチワ"; got != want {
			t.Errorf("ConvertToReading() = %q, want %q", got, want)
		}
	})

	// Option は値を返せないため、エラーは NewConverter が返す。ここで黙って無視すると
	// 辞書の読み込み失敗に気付かないまま、上書きの効かない Converter が出来上がる。
	t.Run("壊れたJSONはNewConverterがエラーにすること", func(t *testing.T) {
		tests := []struct {
			name string
			data string
		}{
			{"JSONとして不正", `{`},
			{"キーが空", `{"": "センコウ"}`},
			{"値が空", `{"閃光": ""}`},
			{"値が文字列でない", `{"閃光": 1}`},
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
