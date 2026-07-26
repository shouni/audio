package phonetic

import (
	"testing"
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
		// 「世界」の上書きが「世界観」の「観」を巻き込んで欠落させないこと。
		// 読み上書きは形態素境界で終わる一致だけを採用する。
		{
			name:  "形態素境界をまたぐ上書きは適用しない",
			input: "世界観が変わる",
			want:  "セカイカンガカワル",
		},
		{
			name:  "境界が一致すれば上書きを適用する",
			input: "世界が変わる",
			want:  "セカイガカワル",
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
		// 読み上書きを先に適用して入力を分断すると、残った「が」が単独で解析されて
		// 助詞ではなく接続詞と判定され、文節スペースが落ちる。
		{
			name:  "読み上書きに挟まれた助詞の後にもスペース",
			input: "運命の閃光が瞳を灼く",
			want:  "サダメノ ヒカリガ ヒトミオ ヤク",
		},
		{
			name:  "読み上書きが連続しても文節が保たれる",
			input: "絆と翼を",
			want:  "キズナト ツバサオ",
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
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := converter.ConvertToReading(tt.input); got != tt.want {
				t.Errorf("ConvertToReading(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
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
