package phonetic

import (
	"testing"
)

func TestConverter_ConvertToReading(t *testing.T) {
	// 本体コードの定義 (want: ()) に合わせて引数を削除したのだ！
	converter, err := NewPhoneticConverter()
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
			want:  "ワタシワセンコウ",
		},
		{
			name:  "助詞「へ」の歌唱補正",
			input: "武道館へ行こう",
			want:  "ブドウカンエイコウ",
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
			want:  "ヨツユシク",
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

func BenchmarkConverter_ConvertToReading(b *testing.B) {
	converter, _ := NewPhoneticConverter()
	input := "長い放課後の廊下を全力で疾走する少女たちは、武道館のステージへ向かって絆を奏でる。"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		converter.ConvertToReading(input)
	}
}
