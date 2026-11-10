package wav

import (
	"strings"
	"testing"
)

// TestErrorMessages checks that each error returns wording from which the cause and the
// remedy can be read. A test that checks only the error type would not notice a broken
// message.
func TestErrorMessages(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want []string
	}{
		{
			name: "音声データなし",
			err:  &ErrNoAudioData{},
			want: []string{"no audio data to combine"},
		},
		{
			name: "ヘッダー不正（一覧の中）",
			err:  &ErrInvalidWAVHeader{Index: 2, Details: "invalid RIFF/WAVE identifier"},
			want: []string{"#2", "invalid RIFF/WAVE identifier"},
		},
		{
			// A check outside a list, such as Inspect, must not print a file number that does not exist.
			name: "ヘッダー不正（単体）",
			err:  &ErrInvalidWAVHeader{Index: -1, Details: "invalid RIFF/WAVE identifier"},
			want: []string{"invalid WAV header", "invalid RIFF/WAVE identifier"},
		},
		{
			name: "フォーマット不一致",
			err:  &ErrMismatchedWAVFormat{Index: 1, Field: "sample rate", First: 24000, Got: 48000},
			want: []string{"#1", "sample rate", "48000 ≠ 24000", "requires matching formats"},
		},
		{
			name: "サブフォーマット不一致",
			err:  &ErrMismatchedSubFormat{Index: 1, First: subFormatPCM, Got: subFormatFloat},
			want: []string{"#1", "sub-format", "{00000003-0000-0010-8000-00AA00389B71}", "{00000001-0000-0010-8000-00AA00389B71}"},
		},
		{
			name: "対応外の形式",
			err:  &ErrUnsupportedWAVFormat{Index: 1, Format: Format{AudioFormat: 0x11, BitsPerSample: 4}},
			want: []string{"#1", "unsupported format", "0x0011", "4 bits", "convert it to PCM first"},
		},
		{
			name: "対応外の形式（拡張形式・単体）",
			err:  &ErrUnsupportedWAVFormat{Index: -1, Format: Format{AudioFormat: extensibleFormatTag, BitsPerSample: 16, SubFormat: subFormatGUID(0x11)}},
			want: []string{"WAV file is", "{00000011-0000-0010-8000-00AA00389B71}"},
		},
		{
			name: "4GB超過",
			err:  &ErrWAVTooLarge{Size: 5_000_000_000},
			want: []string{"exceeds 4GB", "5000000000", "4294967295"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.err.Error()
			for _, want := range tt.want {
				if !strings.Contains(got, want) {
					t.Errorf("Error() = %q, want it to contain %q", got, want)
				}
			}
		})
	}
}
