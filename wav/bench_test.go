package wav

import (
	"fmt"
	"io"
	"testing"
	"time"
)

// benchParts builds count WAVs of one second each at 24kHz mono 16-bit.
func benchParts(count int) [][]byte {
	audio := make([]byte, 48000)
	parts := make([][]byte, count)
	for i := range parts {
		parts[i] = buildWAV(defaultSpec(audio))
	}
	return parts
}

func BenchmarkCombineWavData(b *testing.B) {
	parts := benchParts(64)
	b.ReportAllocs()
	b.SetBytes(int64(len(parts) * 48000))

	for b.Loop() {
		if _, err := CombineWavData(parts); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCombineWavDataWithGap(b *testing.B) {
	parts := benchParts(64)
	b.ReportAllocs()
	b.SetBytes(int64(len(parts) * 48000))

	for b.Loop() {
		if _, err := CombineWavData(parts, WithGap(200*time.Millisecond)); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkCombineTo measures the allocations of the path that allocates no output buffer.
// Per input the allocations are a constant for the scan result and io.CopyN's LimitReader,
// independent of the audio's length. It checks that B/op grows no more than 8 times when
// the number of inputs grows 8 times.
func BenchmarkCombineTo(b *testing.B) {
	for _, count := range []int{8, 64} {
		b.Run(fmt.Sprintf("%d inputs", count), func(b *testing.B) {
			parts := benchParts(count)
			sources := readSeekers(parts...)
			b.ReportAllocs()
			b.SetBytes(int64(count * 48000))

			for b.Loop() {
				for _, source := range sources {
					if _, err := source.Seek(0, io.SeekStart); err != nil {
						b.Fatal(err)
					}
				}
				if err := CombineTo(io.Discard, sources); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkInspect(b *testing.B) {
	data := buildWAV(defaultSpec(make([]byte, 48000)))
	b.ReportAllocs()

	for b.Loop() {
		if _, err := Inspect(data); err != nil {
			b.Fatal(err)
		}
	}
}
