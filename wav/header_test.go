package wav

import (
	"bytes"
	"encoding/binary"
	"errors"
	"testing"
)

// wavSpec は、テスト用 WAV の fmt チャンクに書き込む値です。
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

// buildWAV は spec のとおりの 44 バイトヘッダーを持つ WAV を組み立てます。
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

// TestCombineWavDataRejectsMismatchedFormat は、フォーマットの違う WAV を結合しないことを
// 確認します。
//
// 結合はデコードせず data チャンクを連結するだけで、出力ヘッダーは先頭ファイルのものを
// 使います。そのため 48kHz ステレオを 24kHz モノラルのヘッダーで繋ぐと、音は出るものの
// 速度・音程・チャンネル割り当てがすべて狂います。エラーにしないと呼び出し側は気付けません。
func TestCombineWavDataRejectsMismatchedFormat(t *testing.T) {
	tests := []struct {
		name      string
		mutate    func(*wavSpec)
		wantField string
	}{
		{
			name:      "サンプルレート違い",
			mutate:    func(s *wavSpec) { s.sampleRate = 48000 },
			wantField: "サンプルレート",
		},
		{
			name:      "チャンネル数違い",
			mutate:    func(s *wavSpec) { s.numChannels = 2 },
			wantField: "チャンネル数",
		},
		{
			name:      "量子化ビット数違い",
			mutate:    func(s *wavSpec) { s.bitsPerSample = 24 },
			wantField: "量子化ビット数",
		},
		{
			name:      "フォーマット種別違い",
			mutate:    func(s *wavSpec) { s.audioFormat = 3 },
			wantField: "フォーマット種別",
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

// TestCombineWavDataPreservesChunksBeforeData は、fmt と data の間にある任意チャンクを
// ヘッダーとして引き継ぐことを確認します。LIST/INFO を持つファイルでも壊れないこと。
func TestCombineWavDataPreservesChunksBeforeData(t *testing.T) {
	withList := insertChunkBeforeData(buildWAV(defaultSpec([]byte{1, 2, 3})), "LIST", []byte("abcdef"))

	combined, err := CombineWavData([][]byte{withList, buildWAV(defaultSpec([]byte{4, 5}))})
	if err != nil {
		t.Fatalf("CombineWavData() error = %v", err)
	}
	if !bytes.Contains(combined, []byte("LIST")) {
		t.Error("LIST チャンクがヘッダーから失われています")
	}
	gotAudio := combined[len(combined)-5:]
	if !bytes.Equal(gotAudio, []byte{1, 2, 3, 4, 5}) {
		t.Errorf("audio payload = %v, want [1 2 3 4 5]", gotAudio)
	}
}

// TestCombineWavDataHandlesOddSizedChunk は、奇数サイズのチャンクに続くパディング 1 バイトを
// 読み飛ばして次のチャンクへ進めることを確認します。読み飛ばしを誤ると data を見失います。
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
			name: "RIFFヘッダーより短い",
			data: []byte("RIF"),
		},
		{
			name: "RIFF識別子が不正",
			data: func() []byte { b := buildWAV(defaultSpec([]byte{1})); copy(b[0:], "RIFX"); return b }(),
		},
		{
			name: "WAVE識別子が不正",
			data: func() []byte { b := buildWAV(defaultSpec([]byte{1})); copy(b[8:], "WAVX"); return b }(),
		},
		{
			name: "fmtチャンクがない",
			data: func() []byte { b := buildWAV(defaultSpec([]byte{1})); copy(b[12:], "junk"); return b }(),
		},
		{
			name: "fmtチャンクが短すぎる",
			data: func() []byte {
				b := buildWAV(defaultSpec([]byte{1}))
				binary.LittleEndian.PutUint32(b[16:], 8)
				return b
			}(),
		},
		{
			name: "dataチャンクがない",
			data: func() []byte { b := buildWAV(defaultSpec([]byte{1})); copy(b[36:], "junk"); return b }(),
		},
		{
			name: "dataチャンクのサイズがファイル長を超える",
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

// insertChunkBeforeData は fmt チャンクの直後に任意のチャンクを差し込みます。
// サイズが奇数のときは RIFF の規約どおりパディング 1 バイトを付けます。
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
