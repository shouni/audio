package wav

import (
	"bytes"
	"io"
	"testing"
)

// fuzzSeeds are WAV seeds whose structure is worth breaking.
func fuzzSeeds() [][]byte {
	return [][]byte{
		buildWAV(defaultSpec([]byte{1, 2, 3, 4})),
		insertChunkBeforeData(buildWAV(defaultSpec([]byte{1, 2, 3})), "LIST", []byte("abcde")),
		buildExtensibleWAV(defaultExtensibleSpec([]byte{1, 2, 3, 4})),
		[]byte("RIFF"),
		[]byte("RIFF\x00\x00\x00\x00WAVE"),
	}
}

// FuzzExtractAudioData checks that parsing does not panic on arbitrary bytes.
//
// Chunk scanning is full of offset and size additions, and inputs whose declared sizes
// disagree with reality easily drive it out of range. Returning an error is fine;
// crashing is not.
func FuzzExtractAudioData(f *testing.F) {
	for _, seed := range fuzzSeeds() {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		parts, err := extractAudioData(data, 0)
		if err != nil {
			return
		}
		// On success, the extracted ranges must lie inside the input.
		if len(parts.formatHeader)+len(parts.audioData) > len(data) {
			t.Fatalf("extracted header (%d) plus audio (%d) exceeds the input (%d)",
				len(parts.formatHeader), len(parts.audioData), len(data))
		}
		if info, err := Inspect(data); err != nil {
			t.Fatalf("extractAudioData succeeded but Inspect failed: %v", err)
		} else if info.DataSize != len(parts.audioData) {
			t.Fatalf("Inspect DataSize = %d, extractAudioData gave %d", info.DataSize, len(parts.audioData))
		}
	})
}

// FuzzCombineToMatchesCombineWavData checks that the two combine paths always reach the
// same conclusion. If one path accepted an input the other rejects, the result would
// depend on which API the caller used.
func FuzzCombineToMatchesCombineWavData(f *testing.F) {
	for _, seed := range fuzzSeeds() {
		f.Add(seed, seed)
	}

	f.Fuzz(func(t *testing.T, first, second []byte) {
		parts := [][]byte{first, second}

		want, wantErr := CombineWavData(parts)

		var got bytes.Buffer
		gotErr := CombineTo(&got, readSeekers(parts...))

		if (wantErr == nil) != (gotErr == nil) {
			t.Fatalf("CombineWavData error = %v, CombineTo error = %v", wantErr, gotErr)
		}
		if wantErr != nil {
			return
		}
		if !bytes.Equal(got.Bytes(), want) {
			t.Fatalf("CombineTo output (%d bytes) does not match CombineWavData (%d bytes)", got.Len(), len(want))
		}
	})
}

// FuzzCombineToDoesNotPanic checks that the streaming path does not crash on its own either.
func FuzzCombineToDoesNotPanic(f *testing.F) {
	for _, seed := range fuzzSeeds() {
		f.Add(seed)
	}

	f.Fuzz(func(_ *testing.T, data []byte) {
		_ = CombineTo(io.Discard, readSeekers(data), WithGap(1))
	})
}
