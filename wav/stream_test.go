package wav

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

// readSeekers wraps WAV byte slices in a form CombineTo accepts.
func readSeekers(wavDataList ...[]byte) []io.ReadSeeker {
	sources := make([]io.ReadSeeker, len(wavDataList))
	for i, data := range wavDataList {
		sources[i] = bytes.NewReader(data)
	}
	return sources
}

// TestCombineToMatchesCombineWavData checks that streaming combination produces output
// identical, byte for byte, to in-memory combination. If the two paths diverged, the
// audio would depend on which one was used.
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
				insertChunkBeforeData(buildWAV(defaultSpec([]byte{1, 2, 3, 4})), "LIST", []byte("abcdef")),
				buildWAV(defaultSpec([]byte{5, 6})),
			},
		},
		{
			name: "奇数サイズのチャンクがある",
			parts: [][]byte{
				insertChunkBeforeData(buildWAV(defaultSpec([]byte{1, 2, 3, 4})), "LIST", []byte("abcde")),
				buildWAV(defaultSpec([]byte{5, 6})),
			},
		},
		{
			name: "factチャンクがある",
			parts: [][]byte{
				insertChunkBeforeData(buildWAV(defaultSpec([]byte{1, 2, 3, 4})), "fact", factChunk(2)),
				buildWAV(defaultSpec([]byte{5, 6})),
			},
			opts: []CombineOption{WithGap(time.Millisecond)},
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
				t.Errorf("CombineTo() output does not match CombineWavData() (%d bytes / %d bytes)", got.Len(), len(want))
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
	if mismatch.Index != 1 || mismatch.Field != "sample rate" {
		t.Errorf("Index/Field = %d/%q, want 1/sample rate", mismatch.Index, mismatch.Field)
	}
}

// TestCombineToRejectsMismatchedFormatBeforeWriting checks that nothing is written to w
// when a format mismatch is detected. Failing after a partial write would leave a broken
// WAV fragment in the caller's destination.
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
		t.Errorf("wrote %d bytes on failure; want nothing written", out.Len())
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

// TestCombineToRejectsOversizedCarriedHeader checks that a file with huge metadata before
// data is rejected. Reading it would break the streaming path's premise of not depending
// on input size.
func TestCombineToRejectsOversizedCarriedHeader(t *testing.T) {
	huge := insertChunkBeforeData(buildWAV(defaultSpec([]byte{1, 2})), "LIST", make([]byte, maxCarriedHeaderSize))

	err := CombineTo(io.Discard, readSeekers(huge))
	invalid, ok := errors.AsType[*ErrInvalidWAVHeader](err)
	if !ok {
		t.Fatalf("error type = %T, want *ErrInvalidWAVHeader", err)
	}
	if !strings.Contains(invalid.Details, "CombineWavData") {
		t.Errorf("Details = %q, want it to point to the alternative", invalid.Details)
	}
}

// TestCombineToPropagatesWriteError checks that an error from the destination is returned as is.
func TestCombineToPropagatesWriteError(t *testing.T) {
	wantErr := errors.New("write failed")

	err := CombineTo(failingWriter{err: wantErr}, readSeekers(buildWAV(defaultSpec([]byte{1, 2}))))
	if !errors.Is(err, wantErr) {
		t.Fatalf("CombineTo() error = %v, want it to wrap %v", err, wantErr)
	}
}

type failingWriter struct{ err error }

func (w failingWriter) Write([]byte) (int, error) { return 0, w.err }
