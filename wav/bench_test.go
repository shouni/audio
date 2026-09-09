package wav

import (
	"fmt"
	"io"
	"testing"
	"time"
)

// benchParts は、24kHz モノラル 16bit で 1 秒ぶんの WAV を count 本作ります。
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

// BenchmarkCombineTo は、出力バッファを確保しない経路の割り当て量を測ります。
// 割り当ては 1 本あたり走査結果と io.CopyN の LimitReader ぶんの定数で、音声の長さには
// 依存しません。本数を 8 倍にしても B/op がそれ以上に伸びないことを確かめます。
func BenchmarkCombineTo(b *testing.B) {
	for _, count := range []int{8, 64} {
		b.Run(fmt.Sprintf("%d本", count), func(b *testing.B) {
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
