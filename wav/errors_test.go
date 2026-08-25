package wav

import (
	"strings"
	"testing"
)

// TestErrorMessages は、各エラーが原因と対処を読み取れる文言を返すことを確認します。
// エラー型だけを検査するテストではメッセージが壊れても気付けません。
func TestErrorMessages(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want []string
	}{
		{
			name: "音声データなし",
			err:  &ErrNoAudioData{},
			want: []string{"結合対象の音声データがありません"},
		},
		{
			name: "ヘッダー不正（一覧の中）",
			err:  &ErrInvalidWAVHeader{Index: 2, Details: "RIFF/WAVE識別子が不正です"},
			want: []string{"#2", "RIFF/WAVE識別子が不正です"},
		},
		{
			// Inspect のように一覧に属さない検証では、存在しないファイル番号を出さない。
			name: "ヘッダー不正（単体）",
			err:  &ErrInvalidWAVHeader{Index: -1, Details: "RIFF/WAVE識別子が不正です"},
			want: []string{"WAVヘッダーが無効です", "RIFF/WAVE識別子が不正です"},
		},
		{
			name: "フォーマット不一致",
			err:  &ErrMismatchedWAVFormat{Index: 1, Field: "サンプルレート", First: 24000, Got: 48000},
			want: []string{"#1", "サンプルレート", "48000 ≠ 24000", "フォーマットが揃っている必要があります"},
		},
		{
			name: "サブフォーマット不一致",
			err:  &ErrMismatchedSubFormat{Index: 1, First: subFormatPCM, Got: subFormatFloat},
			want: []string{"#1", "サブフォーマット", "{00000003-0000-0010-8000-00AA00389B71}", "{00000001-0000-0010-8000-00AA00389B71}"},
		},
		{
			name: "4GB超過",
			err:  &ErrWAVTooLarge{Size: 5_000_000_000},
			want: []string{"4GBを超過", "5000000000", "4294967295"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.err.Error()
			for _, want := range tt.want {
				if !strings.Contains(got, want) {
					t.Errorf("Error() = %q, %q を含むこと", got, want)
				}
			}
		})
	}
}
