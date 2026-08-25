package wav

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

// readSeekers は WAV バイト列を CombineTo へ渡せる形に包みます。
func readSeekers(wavDataList ...[]byte) []io.ReadSeeker {
	sources := make([]io.ReadSeeker, len(wavDataList))
	for i, data := range wavDataList {
		sources[i] = bytes.NewReader(data)
	}
	return sources
}

// TestCombineToMatchesCombineWavData は、ストリーミング結合が一括結合と 1 バイトも
// 違わない出力を作ることを確認します。2 つの経路が食い違うと、どちらを使うかで
// 音が変わってしまいます。
func TestCombineToMatchesCombineWavData(t *testing.T) {
	tests := []struct {
		name  string
		parts [][]byte
		opts  []CombineOption
	}{
		{
			name:  "1本",
			parts: [][]byte{buildWAV(defaultSpec([]byte{1, 2, 3, 4}))},
		},
		{
			name: "複数本",
			parts: [][]byte{
				buildWAV(defaultSpec([]byte{1, 2, 3, 4})),
				buildWAV(defaultSpec([]byte{5, 6})),
				buildWAV(defaultSpec([]byte{7, 8, 9, 10})),
			},
		},
		{
			name: "無音を挟む",
			parts: [][]byte{
				buildWAV(defaultSpec([]byte{1, 2, 3, 4})),
				buildWAV(defaultSpec([]byte{5, 6, 7, 8})),
			},
			opts: []CombineOption{WithGap(20 * time.Millisecond)},
		},
		{
			name: "dataの前に任意チャンクがある",
			parts: [][]byte{
				insertChunkBeforeData(buildWAV(defaultSpec([]byte{1, 2, 3})), "LIST", []byte("abcdef")),
				buildWAV(defaultSpec([]byte{4, 5})),
			},
		},
		{
			name: "奇数サイズのチャンクがある",
			parts: [][]byte{
				insertChunkBeforeData(buildWAV(defaultSpec([]byte{1, 2, 3})), "LIST", []byte("abcde")),
				buildWAV(defaultSpec([]byte{4, 5})),
			},
		},
		{
			name: "拡張形式",
			parts: [][]byte{
				buildExtensibleWAV(defaultExtensibleSpec([]byte{1, 2, 3, 4})),
				buildExtensibleWAV(defaultExtensibleSpec([]byte{5, 6, 7, 8})),
			},
			opts: []CombineOption{WithGap(5 * time.Millisecond)},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want, err := CombineWavData(tt.parts, tt.opts...)
			if err != nil {
				t.Fatalf("CombineWavData() error = %v", err)
			}

			var got bytes.Buffer
			if err := CombineTo(&got, readSeekers(tt.parts...), tt.opts...); err != nil {
				t.Fatalf("CombineTo() error = %v", err)
			}
			if !bytes.Equal(got.Bytes(), want) {
				t.Errorf("CombineTo() の出力が CombineWavData() と一致しません (%dバイト / %dバイト)", got.Len(), len(want))
			}
		})
	}
}

func TestCombineToReturnsErrorOnEmptyInput(t *testing.T) {
	err := CombineTo(io.Discard, nil)
	if _, ok := errors.AsType[*ErrNoAudioData](err); !ok {
		t.Fatalf("error type = %T, want *ErrNoAudioData", err)
	}
}

func TestCombineToRejectsMismatchedFormat(t *testing.T) {
	second := defaultSpec([]byte{5, 6, 7, 8})
	second.sampleRate = 48000

	err := CombineTo(io.Discard, readSeekers(
		buildWAV(defaultSpec([]byte{1, 2, 3, 4})),
		buildWAV(second),
	))
	mismatch, ok := errors.AsType[*ErrMismatchedWAVFormat](err)
	if !ok {
		t.Fatalf("error type = %T, want *ErrMismatchedWAVFormat", err)
	}
	if mismatch.Index != 1 || mismatch.Field != "サンプルレート" {
		t.Errorf("Index/Field = %d/%q, want 1/サンプルレート", mismatch.Index, mismatch.Field)
	}
}

// TestCombineToRejectsMismatchedFormatBeforeWriting は、フォーマット不一致を
// 検出したときに w へ何も書かないことを確認します。途中まで書いてから失敗すると、
// 呼び出し側の出力先に壊れた WAV の断片が残ります。
func TestCombineToRejectsMismatchedFormatBeforeWriting(t *testing.T) {
	second := defaultSpec([]byte{5, 6, 7, 8})
	second.numChannels = 2

	var out bytes.Buffer
	if err := CombineTo(&out, readSeekers(
		buildWAV(defaultSpec([]byte{1, 2, 3, 4})),
		buildWAV(second),
	)); err == nil {
		t.Fatal("CombineTo() error = nil, want ErrMismatchedWAVFormat")
	}
	if out.Len() != 0 {
		t.Errorf("失敗時に %d バイト書き出しています。何も書かないこと", out.Len())
	}
}

func TestCombineToRejectsBrokenHeaders(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{"RIFFヘッダーより短い", []byte("RIF")},
		{"空", nil},
		{"RIFF識別子が不正", func() []byte { b := buildWAV(defaultSpec([]byte{1})); copy(b[0:], "RIFX"); return b }()},
		{"fmtチャンクがない", func() []byte { b := buildWAV(defaultSpec([]byte{1})); copy(b[12:], "junk"); return b }()},
		{"dataチャンクがない", func() []byte { b := buildWAV(defaultSpec([]byte{1})); copy(b[36:], "junk"); return b }()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := CombineTo(io.Discard, readSeekers(tt.data)); err == nil {
				t.Fatal("CombineTo() error = nil, want error")
			}
		})
	}
}

// TestCombineToRejectsOversizedCarriedHeader は、data の前に巨大なメタデータを持つ
// ファイルを弾くことを確認します。読み込んでしまうと、入力サイズに依存しないという
// ストリーミング結合の前提が崩れます。
func TestCombineToRejectsOversizedCarriedHeader(t *testing.T) {
	huge := insertChunkBeforeData(buildWAV(defaultSpec([]byte{1, 2})), "LIST", make([]byte, maxCarriedHeaderSize))

	err := CombineTo(io.Discard, readSeekers(huge))
	invalid, ok := errors.AsType[*ErrInvalidWAVHeader](err)
	if !ok {
		t.Fatalf("error type = %T, want *ErrInvalidWAVHeader", err)
	}
	if !strings.Contains(invalid.Details, "CombineWavData") {
		t.Errorf("Details = %q, 代替手段の案内を含むこと", invalid.Details)
	}
}

// TestCombineToPropagatesWriteError は、出力先のエラーをそのまま返すことを確認します。
func TestCombineToPropagatesWriteError(t *testing.T) {
	wantErr := errors.New("書き出し失敗")

	err := CombineTo(failingWriter{err: wantErr}, readSeekers(buildWAV(defaultSpec([]byte{1, 2}))))
	if !errors.Is(err, wantErr) {
		t.Fatalf("CombineTo() error = %v, want %v を含むこと", err, wantErr)
	}
}

type failingWriter struct{ err error }

func (w failingWriter) Write([]byte) (int, error) { return 0, w.err }
