package wav

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"math"
	"strings"
	"testing"
	"time"
)

func TestCombineWavDataConcatenatesAudioPayloads(t *testing.T) {
	first := buildWAV(defaultSpec([]byte{1, 2, 3, 4}))
	second := buildWAV(defaultSpec([]byte{5, 6, 7}))

	combined, err := CombineWavData([][]byte{first, second})
	if err != nil {
		t.Fatalf("CombineWavData() error = %v", err)
	}

	if string(combined[:4]) != "RIFF" {
		t.Fatalf("header chunk = %q, want RIFF", string(combined[:4]))
	}
	if !strings.Contains(string(combined[:16]), "WAVE") {
		t.Fatal("combined wav does not contain WAVE header")
	}
	if len(combined) < standardHeaderSize {
		t.Fatalf("combined wav is too short: %d bytes", len(combined))
	}
	gotAudio := combined[len(combined)-7:]
	wantAudio := []byte{1, 2, 3, 4, 5, 6, 7}
	dataSize := binary.LittleEndian.Uint32(combined[len(combined)-7-4 : len(combined)-7])
	expectedSize := uint32(len(wantAudio))
	if dataSize != expectedSize {
		t.Fatalf("data size = %d, want %d", dataSize, expectedSize)
	}
	if !bytes.Equal(gotAudio, wantAudio) {
		t.Fatalf("audio payload = %v, want %v", gotAudio, wantAudio)
	}
}

func TestCombineWavDataReturnsErrorOnEmptyInput(t *testing.T) {
	_, err := CombineWavData(nil)
	if err == nil {
		t.Fatal("CombineWavData() error = nil, want ErrNoAudioData")
	}
	if _, ok := errors.AsType[*ErrNoAudioData](err); !ok {
		t.Fatalf("error type = %T, want *ErrNoAudioData", err)
	}
}

// TestCombineWavDataWithGap checks that silence goes only between WAVs. Silence at the
// start or end would stretch the whole by as many gaps as there are joins.
func TestCombineWavDataWithGap(t *testing.T) {
	// 24kHz mono 16-bit = 48000 bytes/s, so 100ms is exactly 4800 bytes.
	const gapSize = 4800

	combined, err := CombineWavData([][]byte{
		buildWAV(defaultSpec([]byte{1, 2, 3, 4})),
		buildWAV(defaultSpec([]byte{5, 6, 7, 8})),
		buildWAV(defaultSpec([]byte{9, 10, 11, 12})),
	}, WithGap(100*time.Millisecond))
	if err != nil {
		t.Fatalf("CombineWavData() error = %v", err)
	}

	wantAudio := make([]byte, 0, 12+gapSize*2)
	wantAudio = append(wantAudio, 1, 2, 3, 4)
	wantAudio = append(wantAudio, make([]byte, gapSize)...)
	wantAudio = append(wantAudio, 5, 6, 7, 8)
	wantAudio = append(wantAudio, make([]byte, gapSize)...)
	wantAudio = append(wantAudio, 9, 10, 11, 12)

	if got := combined[standardHeaderSize:]; !bytes.Equal(got, wantAudio) {
		t.Errorf("audio payload length = %d, want %d", len(got), len(wantAudio))
	}
	if got := binary.LittleEndian.Uint32(combined[40:]); got != uint32(len(wantAudio)) {
		t.Errorf("data size = %d, want %d", got, len(wantAudio))
	}
	info, err := Inspect(combined)
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	if want := 200*time.Millisecond + time.Duration(12)*time.Second/48000; info.Duration() != want {
		t.Errorf("Duration() = %v, want %v", info.Duration(), want)
	}
}

// TestCombineWavDataWithGapUses8BitSilence checks that silence is 0x80 for 8-bit linear
// PCM only. Filling with 0x00 pins the signal to the bottom of the range and clicks at
// each join.
func TestCombineWavDataWithGapUses8BitSilence(t *testing.T) {
	spec := func(audio []byte) wavSpec {
		return wavSpec{audioFormat: 1, numChannels: 1, sampleRate: 8000, bitsPerSample: 8, audio: audio}
	}

	combined, err := CombineWavData([][]byte{
		buildWAV(spec([]byte{1})),
		buildWAV(spec([]byte{2})),
	}, WithGap(time.Millisecond))
	if err != nil {
		t.Fatalf("CombineWavData() error = %v", err)
	}

	// 8000 bytes/s × 1ms = 8 bytes.
	want := []byte{1, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 2}
	if got := combined[standardHeaderSize:]; !bytes.Equal(got, want) {
		t.Errorf("audio payload = %v, want %v", got, want)
	}
}

func TestCombineWavDataGapEdgeCases(t *testing.T) {
	tests := []struct {
		name string
		gap  time.Duration
	}{
		{"gapが0なら無音を挟まない", 0},
		{"gapが負なら無音を挟まない", -time.Second},
		// One sample of 24kHz 16-bit mono is about 41.6µs. Anything shorter truncates to 0.
		{"1サンプルに満たないgapは切り捨てる", 10 * time.Microsecond},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			combined, err := CombineWavData([][]byte{
				buildWAV(defaultSpec([]byte{1, 2})),
				buildWAV(defaultSpec([]byte{3, 4})),
			}, WithGap(tt.gap))
			if err != nil {
				t.Fatalf("CombineWavData() error = %v", err)
			}
			if got := combined[standardHeaderSize:]; !bytes.Equal(got, []byte{1, 2, 3, 4}) {
				t.Errorf("audio payload = %v, want [1 2 3 4]", got)
			}
		})
	}
}

// TestCombineWavDataWithGapKeepsBlockAlignment checks that the silence is truncated to a
// sample boundary. Inserting a partial byte count would shift every channel after it by
// one byte on playback.
func TestCombineWavDataWithGapKeepsBlockAlignment(t *testing.T) {
	// The block align of 44100Hz stereo 16-bit is 4 bytes.
	spec := func(audio []byte) wavSpec {
		return wavSpec{audioFormat: 1, numChannels: 2, sampleRate: 44100, bitsPerSample: 16, audio: audio}
	}

	combined, err := CombineWavData([][]byte{
		buildWAV(spec([]byte{1, 2, 3, 4})),
		buildWAV(spec([]byte{5, 6, 7, 8})),
	}, WithGap(7*time.Millisecond))
	if err != nil {
		t.Fatalf("CombineWavData() error = %v", err)
	}

	gapSize := len(combined) - standardHeaderSize - 8
	if gapSize%4 != 0 {
		t.Errorf("silence size = %d bytes, want a multiple of the block align (4)", gapSize)
	}
	// 176400 bytes/s × 7ms = 1234.8 → 1234 bytes, truncated to a multiple of 4: 1232.
	if gapSize != 1232 {
		t.Errorf("silence size = %d, want 1232", gapSize)
	}
}

// factSampleCount returns the sample count held by a WAV's fact chunk,
// or -1 when there is no fact chunk.
func factSampleCount(t *testing.T, wavBytes []byte) int64 {
	t.Helper()
	offset := bytes.Index(wavBytes, []byte("fact"))
	if offset < 0 {
		return -1
	}
	return int64(binary.LittleEndian.Uint32(wavBytes[offset+chunkHeaderSize:]))
}

// factPayloadPrefix returns the first 2 bytes of a WAV's fact chunk payload.
func factPayloadPrefix(t *testing.T, wavBytes []byte) []byte {
	t.Helper()
	offset := bytes.Index(wavBytes, []byte("fact"))
	if offset < 0 {
		t.Fatal("fact chunk not found")
	}
	return wavBytes[offset+chunkHeaderSize : offset+chunkHeaderSize+2]
}

// factChunk returns a fact chunk payload holding sampleCount.
func factChunk(sampleCount uint32) []byte {
	return binary.LittleEndian.AppendUint32(nil, sampleCount)
}

// TestCombineWavDataUpdatesFactChunk checks that the sample count of the fact chunk
// carried from the first file becomes the combined length, not the first file's length.
func TestCombineWavDataUpdatesFactChunk(t *testing.T) {
	// defaultSpec is 16-bit mono, so the block align is 2 bytes.
	first := insertChunkBeforeData(buildWAV(defaultSpec([]byte{1, 2, 3, 4})), "fact", factChunk(2))
	second := buildWAV(defaultSpec([]byte{5, 6, 7, 8, 9, 10}))

	tests := []struct {
		name string
		opts []CombineOption
		want int64
	}{
		{name: "無音なし", want: 5},
		// 1ms of silence is 24 samples.
		{name: "無音あり", opts: []CombineOption{WithGap(time.Millisecond)}, want: 5 + 24},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			combined, err := CombineWavData([][]byte{first, second}, tt.opts...)
			if err != nil {
				t.Fatalf("CombineWavData() error = %v", err)
			}
			if got := factSampleCount(t, combined); got != tt.want {
				t.Errorf("fact sample count = %d, want %d", got, tt.want)
			}
		})
	}
}

// TestCombineWavDataLeavesUnusableFactChunk checks that a fact chunk not shaped to hold a
// sample count is left untouched.
func TestCombineWavDataLeavesUnusableFactChunk(t *testing.T) {
	parts := [][]byte{
		insertChunkBeforeData(buildWAV(defaultSpec([]byte{1, 2, 3, 4})), "fact", []byte{2, 0}),
		buildWAV(defaultSpec([]byte{5, 6})),
	}
	combined, err := CombineWavData(parts)
	if err != nil {
		t.Fatalf("CombineWavData() error = %v", err)
	}
	got := factPayloadPrefix(t, combined)
	want := factPayloadPrefix(t, parts[0])
	if !bytes.Equal(got, want) {
		t.Errorf("fact payload = %v, want %v (unchanged)", got, want)
	}
}

// TestUpdateFactChunkSkipsZeroBlockAlign checks that the fact chunk is left untouched for
// a format whose block align is 0, since no sample count can be computed for it.
// Parsing already rejects 4-bit formats, so this calls the header rewrite directly.
func TestUpdateFactChunkSkipsZeroBlockAlign(t *testing.T) {
	wavBytes := insertChunkBeforeData(buildWAV(defaultSpec(nil)), "fact", factChunk(2))
	header := wavBytes[:len(wavBytes)-chunkHeaderSize]
	want := bytes.Clone(header)

	updateFactChunk(header, Format{NumChannels: 1, BitsPerSample: 4}, 100)
	if !bytes.Equal(header, want) {
		t.Errorf("header = %v, want %v (unchanged)", header, want)
	}
}

// TestBuildCombinedHeaderRejectsOversizedResult checks that a combination over 4GB, which
// RIFF cannot express, is rejected with its own error type. To check it without actually
// allocating 4GB, it calls the header assembly directly.
func TestBuildCombinedHeaderRejectsOversizedResult(t *testing.T) {
	formatHeader := buildWAV(defaultSpec(nil))[:36]

	_, err := buildCombinedHeader(formatHeader, Format{}, math.MaxUint32)
	if err == nil {
		t.Fatal("buildCombinedHeader() error = nil, want ErrWAVTooLarge")
	}
	tooLarge, ok := errors.AsType[*ErrWAVTooLarge](err)
	if !ok {
		t.Fatalf("error type = %T, want *ErrWAVTooLarge", err)
	}
	if tooLarge.Size <= math.MaxUint32 {
		t.Errorf("Size = %d, want > %d", tooLarge.Size, uint64(math.MaxUint32))
	}
}

// TestCombineRejectsMisalignedIntermediateInput checks that combining is refused when the
// data length of an intermediate input is not a multiple of the block size. Data is
// concatenated without decoding, so joining it as is shifts every following sample by
// one byte into a roar. The last input is let through since nothing follows it.
func TestCombineRejectsMisalignedIntermediateInput(t *testing.T) {
	// 3 bytes of 16-bit mono = 1.5 samples.
	misaligned := buildWAV(defaultSpec([]byte{1, 2, 3}))
	aligned := buildWAV(defaultSpec([]byte{4, 5, 6, 7}))

	t.Run("途中の入力は拒む", func(t *testing.T) {
		_, err := CombineWavData([][]byte{misaligned, aligned})
		var headerErr *ErrInvalidWAVHeader
		if !errors.As(err, &headerErr) {
			t.Fatalf("CombineWavData() error = %v, want *ErrInvalidWAVHeader", err)
		}
		if headerErr.Index != 0 {
			t.Errorf("Index = %d, want 0 (the input that causes the shift)", headerErr.Index)
		}
		if err := CombineTo(io.Discard, readSeekers(misaligned, aligned)); !errors.As(err, &headerErr) {
			t.Errorf("CombineTo() error = %v, want *ErrInvalidWAVHeader", err)
		}
	})

	t.Run("最後の入力は通す", func(t *testing.T) {
		if _, err := CombineWavData([][]byte{aligned, misaligned}); err != nil {
			t.Errorf("CombineWavData() error = %v, want nil", err)
		}
		if err := CombineTo(io.Discard, readSeekers(aligned, misaligned)); err != nil {
			t.Errorf("CombineTo() error = %v, want nil", err)
		}
	})
}
