package wav

import (
	"bytes"
	"encoding/binary"
	"errors"
	"testing"
)

// wavSpec holds the values written into the fmt chunk of a test WAV.
type wavSpec struct {
	audioFormat   uint16
	numChannels   uint16
	sampleRate    uint32
	bitsPerSample uint16
	audio         []byte
}

func defaultSpec(audio []byte) wavSpec {
	return wavSpec{audioFormat: 1, numChannels: 1, sampleRate: 24000, bitsPerSample: 16, audio: audio}
}

// buildWAV assembles a WAV with a 44-byte header as described by spec.
func buildWAV(spec wavSpec) []byte {
	blockAlign := spec.numChannels * spec.bitsPerSample / 8
	header := make([]byte, standardHeaderSize)
	copy(header[0:], "RIFF")
	binary.LittleEndian.PutUint32(header[4:], uint32(36+len(spec.audio)))
	copy(header[8:], "WAVE")
	copy(header[12:], "fmt ")
	binary.LittleEndian.PutUint32(header[16:], minFormatChunkSize)
	binary.LittleEndian.PutUint16(header[20:], spec.audioFormat)
	binary.LittleEndian.PutUint16(header[22:], spec.numChannels)
	binary.LittleEndian.PutUint32(header[24:], spec.sampleRate)
	binary.LittleEndian.PutUint32(header[28:], spec.sampleRate*uint32(blockAlign))
	binary.LittleEndian.PutUint16(header[32:], blockAlign)
	binary.LittleEndian.PutUint16(header[34:], spec.bitsPerSample)
	copy(header[36:], "data")
	binary.LittleEndian.PutUint32(header[40:], uint32(len(spec.audio)))
	return append(header, spec.audio...)
}

// TestCombineWavDataRejectsMismatchedFormat checks that WAVs of different formats are not
// combined.
//
// Combining only concatenates data chunks without decoding and uses the first file's
// header for the output. Joining 48kHz stereo under a 24kHz mono header therefore still
// plays, but with the speed, pitch and channel assignment all wrong. Without an error the
// caller cannot notice.
func TestCombineWavDataRejectsMismatchedFormat(t *testing.T) {
	tests := []struct {
		name      string
		mutate    func(*wavSpec)
		wantField string
	}{
		{
			name:      "different sample rate",
			mutate:    func(s *wavSpec) { s.sampleRate = 48000 },
			wantField: "sample rate",
		},
		{
			name:      "different channel count",
			mutate:    func(s *wavSpec) { s.numChannels = 2 },
			wantField: "channel count",
		},
		{
			name:      "different bit depth",
			mutate:    func(s *wavSpec) { s.bitsPerSample = 24 },
			wantField: "bits per sample",
		},
		{
			name: "different format type",
			// Only 32/64-bit float is supported, so change the bit depth along with it.
			// AudioFormat is compared first, so the mismatch is reported as the format tag.
			mutate:    func(s *wavSpec) { s.audioFormat = floatFormatTag; s.bitsPerSample = 32 },
			wantField: "format tag",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			second := defaultSpec([]byte{5, 6, 7, 8})
			tt.mutate(&second)

			_, err := CombineWavData([][]byte{
				buildWAV(defaultSpec([]byte{1, 2, 3, 4})),
				buildWAV(second),
			})
			if err == nil {
				t.Fatal("CombineWavData() error = nil, want ErrMismatchedWAVFormat")
			}
			mismatch, ok := errors.AsType[*ErrMismatchedWAVFormat](err)
			if !ok {
				t.Fatalf("error type = %T, want *ErrMismatchedWAVFormat", err)
			}
			if mismatch.Field != tt.wantField {
				t.Errorf("Field = %q, want %q", mismatch.Field, tt.wantField)
			}
			if mismatch.Index != 1 {
				t.Errorf("Index = %d, want 1", mismatch.Index)
			}
		})
	}
}

func TestCombineWavDataAcceptsIdenticalFormat(t *testing.T) {
	combined, err := CombineWavData([][]byte{
		buildWAV(defaultSpec([]byte{1, 2, 3, 4})),
		buildWAV(defaultSpec([]byte{5, 6, 7, 8})),
	})
	if err != nil {
		t.Fatalf("CombineWavData() error = %v", err)
	}

	wantAudio := []byte{1, 2, 3, 4, 5, 6, 7, 8}
	gotAudio := combined[standardHeaderSize:]
	if !bytes.Equal(gotAudio, wantAudio) {
		t.Errorf("audio payload = %v, want %v", gotAudio, wantAudio)
	}
	if got := binary.LittleEndian.Uint32(combined[40:]); got != uint32(len(wantAudio)) {
		t.Errorf("data size = %d, want %d", got, len(wantAudio))
	}
	if got := binary.LittleEndian.Uint32(combined[4:]); got != uint32(len(combined)-8) {
		t.Errorf("RIFF size = %d, want %d", got, len(combined)-8)
	}
}

// TestCombineWavDataPreservesChunksBeforeData checks that arbitrary chunks between fmt and
// data are carried over as part of the header, so files with LIST/INFO do not break.
func TestCombineWavDataPreservesChunksBeforeData(t *testing.T) {
	withList := insertChunkBeforeData(buildWAV(defaultSpec([]byte{1, 2, 3, 4})), "LIST", []byte("abcdef"))

	combined, err := CombineWavData([][]byte{withList, buildWAV(defaultSpec([]byte{5, 6}))})
	if err != nil {
		t.Fatalf("CombineWavData() error = %v", err)
	}
	if !bytes.Contains(combined, []byte("LIST")) {
		t.Error("LIST chunk was lost from the header")
	}
	gotAudio := combined[len(combined)-6:]
	if !bytes.Equal(gotAudio, []byte{1, 2, 3, 4, 5, 6}) {
		t.Errorf("audio payload = %v, want [1 2 3 4 5 6]", gotAudio)
	}
}

// TestCombineWavDataHandlesOddSizedChunk checks that the 1-byte padding after an odd-sized
// chunk is skipped to reach the next chunk. Skipping it wrongly loses the data chunk.
func TestCombineWavDataHandlesOddSizedChunk(t *testing.T) {
	withOdd := insertChunkBeforeData(buildWAV(defaultSpec([]byte{1, 2, 3})), "LIST", []byte("abcde"))

	combined, err := CombineWavData([][]byte{withOdd})
	if err != nil {
		t.Fatalf("CombineWavData() error = %v", err)
	}
	if got := combined[len(combined)-3:]; !bytes.Equal(got, []byte{1, 2, 3}) {
		t.Errorf("audio payload = %v, want [1 2 3]", got)
	}
}

func TestExtractAudioDataRejectsBrokenHeaders(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{
			name: "shorter than RIFF header",
			data: []byte("RIF"),
		},
		{
			name: "invalid RIFF ID",
			data: func() []byte { b := buildWAV(defaultSpec([]byte{1})); copy(b[0:], "RIFX"); return b }(),
		},
		{
			name: "invalid WAVE ID",
			data: func() []byte { b := buildWAV(defaultSpec([]byte{1})); copy(b[8:], "WAVX"); return b }(),
		},
		{
			name: "missing fmt chunk",
			data: func() []byte { b := buildWAV(defaultSpec([]byte{1})); copy(b[12:], "junk"); return b }(),
		},
		{
			name: "fmt chunk too short",
			data: func() []byte {
				b := buildWAV(defaultSpec([]byte{1}))
				binary.LittleEndian.PutUint32(b[16:], 8)
				return b
			}(),
		},
		{
			name: "missing data chunk",
			data: func() []byte { b := buildWAV(defaultSpec([]byte{1})); copy(b[36:], "junk"); return b }(),
		},
		{
			name: "data chunk size exceeds file length",
			data: func() []byte {
				b := buildWAV(defaultSpec([]byte{1, 2, 3}))
				binary.LittleEndian.PutUint32(b[40:], 9999)
				return b
			}(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := extractAudioData(tt.data, 0); err == nil {
				t.Fatal("extractAudioData() error = nil, want error")
			}
		})
	}
}

// insertChunkBeforeData inserts an arbitrary chunk right after the fmt chunk.
// For an odd size it adds 1 byte of padding, as RIFF requires.
func insertChunkBeforeData(wavBytes []byte, id string, payload []byte) []byte {
	const dataChunkStart = 36

	chunk := make([]byte, chunkHeaderSize+len(payload))
	copy(chunk[0:], id)
	binary.LittleEndian.PutUint32(chunk[4:], uint32(len(payload)))
	copy(chunk[chunkHeaderSize:], payload)
	if len(payload)%2 != 0 {
		chunk = append(chunk, 0)
	}

	out := make([]byte, 0, len(wavBytes)+len(chunk))
	out = append(out, wavBytes[:dataChunkStart]...)
	out = append(out, chunk...)
	out = append(out, wavBytes[dataChunkStart:]...)
	binary.LittleEndian.PutUint32(out[4:], uint32(len(out)-8))
	return out
}
