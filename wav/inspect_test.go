package wav

import (
	"errors"
	"testing"
	"time"
)

func TestInspect(t *testing.T) {
	t.Run("reads format and data size", func(t *testing.T) {
		// 24kHz mono 16-bit = 48000 bytes/s, so 4800 bytes is exactly 100ms.
		audio := make([]byte, 4800)

		info, err := Inspect(buildWAV(defaultSpec(audio)))
		if err != nil {
			t.Fatalf("Inspect() error = %v", err)
		}

		wantFormat := Format{AudioFormat: 1, NumChannels: 1, SampleRate: 24000, BitsPerSample: 16}
		if info.Format != wantFormat {
			t.Errorf("Format = %+v, want %+v", info.Format, wantFormat)
		}
		if info.DataSize != len(audio) {
			t.Errorf("DataSize = %d, want %d", info.DataSize, len(audio))
		}
		if got := info.Duration(); got != 100*time.Millisecond {
			t.Errorf("Duration() = %v, want 100ms", got)
		}
	})

	t.Run("reads with an extra chunk before data", func(t *testing.T) {
		withList := insertChunkBeforeData(buildWAV(defaultSpec([]byte{1, 2, 3, 4})), "LIST", []byte("abcdef"))

		info, err := Inspect(withList)
		if err != nil {
			t.Fatalf("Inspect() error = %v", err)
		}
		if info.DataSize != 4 {
			t.Errorf("DataSize = %d, want 4", info.DataSize)
		}
	})

	t.Run("invalid WAV is a header error with Index -1", func(t *testing.T) {
		_, err := Inspect([]byte("RIF"))
		if err == nil {
			t.Fatal("Inspect() error = nil, want ErrInvalidWAVHeader")
		}
		invalid, ok := errors.AsType[*ErrInvalidWAVHeader](err)
		if !ok {
			t.Fatalf("error type = %T, want *ErrInvalidWAVHeader", err)
		}
		if invalid.Index != -1 {
			t.Errorf("Index = %d, want -1", invalid.Index)
		}
	})
}

func TestInfoDurationReturnsZeroWhenByteRateIsZero(t *testing.T) {
	info := Info{DataSize: 100}
	if got := info.Duration(); got != 0 {
		t.Errorf("Duration() = %v, want 0", got)
	}
}
