package wav

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math"
	"strings"
	"testing"
	"time"
)

func TestCombineWavDataConcatenatesAudioPayloads(t *testing.T) {
	first := buildWAV(defaultSpec([]byte{1, 2, 3}))
	second := buildWAV(defaultSpec([]byte{4, 5}))

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
	gotAudio := combined[len(combined)-5:]
	wantAudio := []byte{1, 2, 3, 4, 5}
	dataSize := binary.LittleEndian.Uint32(combined[len(combined)-5-4 : len(combined)-5])
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

// TestCombineWavDataWithGap は、WAV と WAV の間だけに無音が入ることを確認します。
// 先頭や末尾に無音が付くと、繋いだ数だけ全体が間延びします。
func TestCombineWavDataWithGap(t *testing.T) {
	// 24kHz モノラル 16bit = 48000 バイト/秒。100ms はちょうど 4800 バイト。
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

// TestCombineWavDataWithGapUses8BitSilence は、8bit のリニア PCM だけ無音が 0x80 に
// なることを確認します。0x00 で埋めると振幅の底に張り付き、継ぎ目でプチノイズが出ます。
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

	// 8000 バイト/秒 × 1ms = 8 バイト。
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
		// 24kHz 16bit モノラルの 1 サンプルは約 41.6µs。それ未満は切り捨てで 0 になる。
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

// TestCombineWavDataWithGapKeepsBlockAlignment は、無音の長さがサンプル境界で
// 切り捨てられることを確認します。半端なバイト数を挟むと、それ以降の全チャンネルが
// 1 バイトずつずれて再生されます。
func TestCombineWavDataWithGapKeepsBlockAlignment(t *testing.T) {
	// 44100Hz ステレオ 16bit のブロックアラインは 4 バイト。
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
		t.Errorf("無音のサイズ = %d バイト、ブロックアライン (4) の倍数であること", gapSize)
	}
	// 176400 バイト/秒 × 7ms = 1234.8 → 1234 バイト、4 の倍数へ切り捨てて 1232。
	if gapSize != 1232 {
		t.Errorf("無音のサイズ = %d, want 1232", gapSize)
	}
}

// TestBuildCombinedHeaderRejectsOversizedResult は、RIFF が表現できない 4GB 超の
// 結合を専用のエラー型で弾くことを確認します。実際に 4GB を確保せずに検証するため、
// ヘッダー組み立てを直接呼びます。
func TestBuildCombinedHeaderRejectsOversizedResult(t *testing.T) {
	formatHeader := buildWAV(defaultSpec(nil))[:36]

	_, err := buildCombinedHeader(formatHeader, math.MaxUint32)
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
