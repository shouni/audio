package wav

import (
	"encoding/binary"
	"errors"
	"testing"
)

// KSDATAFORMAT_SUBTYPE_* は WAVE_FORMAT_EXTENSIBLE のサブフォーマット GUID です。
// 先頭 2 バイトが実際の符号化方式で、残りは全ての標準サブタイプで共通の固定値です。
var (
	subFormatPCM   = subFormatGUID(pcmFormatTag)
	subFormatFloat = subFormatGUID(3)
)

func subFormatGUID(tag uint16) [16]byte {
	var guid [16]byte
	binary.LittleEndian.PutUint16(guid[0:], tag)
	copy(guid[4:], []byte{0x00, 0x00, 0x10, 0x00, 0x80, 0x00, 0x00, 0xAA, 0x00, 0x38, 0x9B, 0x71})
	return guid
}

// extensibleSpec は WAVE_FORMAT_EXTENSIBLE のテスト用 WAV を組み立てるための値です。
type extensibleSpec struct {
	numChannels   uint16
	sampleRate    uint32
	bitsPerSample uint16
	channelMask   uint32
	subFormat     [16]byte
	fmtChunkSize  uint32 // 0 のときは extensibleFormatChunkSize
	audio         []byte
}

func defaultExtensibleSpec(audio []byte) extensibleSpec {
	return extensibleSpec{
		numChannels:   2,
		sampleRate:    48000,
		bitsPerSample: 16,
		channelMask:   0x3, // FL | FR
		subFormat:     subFormatPCM,
		audio:         audio,
	}
}

// buildExtensibleWAV は 40 バイトの fmt チャンクを持つ拡張形式の WAV を組み立てます。
func buildExtensibleWAV(spec extensibleSpec) []byte {
	blockAlign := spec.numChannels * spec.bitsPerSample / 8

	payload := make([]byte, extensibleFormatChunkSize)
	binary.LittleEndian.PutUint16(payload[0:], extensibleFormatTag)
	binary.LittleEndian.PutUint16(payload[2:], spec.numChannels)
	binary.LittleEndian.PutUint32(payload[4:], spec.sampleRate)
	binary.LittleEndian.PutUint32(payload[8:], spec.sampleRate*uint32(blockAlign))
	binary.LittleEndian.PutUint16(payload[12:], blockAlign)
	binary.LittleEndian.PutUint16(payload[14:], spec.bitsPerSample)
	binary.LittleEndian.PutUint16(payload[16:], 22) // cbSize
	binary.LittleEndian.PutUint16(payload[18:], spec.bitsPerSample)
	binary.LittleEndian.PutUint32(payload[20:], spec.channelMask)
	copy(payload[24:], spec.subFormat[:])

	declaredSize := spec.fmtChunkSize
	if declaredSize == 0 {
		declaredSize = extensibleFormatChunkSize
	}

	out := make([]byte, 0, wavRiffHeaderSize+chunkHeaderSize+len(payload)+chunkHeaderSize+len(spec.audio))
	out = append(out, "RIFF"...)
	out = append(out, 0, 0, 0, 0)
	out = append(out, "WAVE"...)
	out = append(out, "fmt "...)
	out = binary.LittleEndian.AppendUint32(out, declaredSize)
	out = append(out, payload...)
	out = append(out, "data"...)
	out = binary.LittleEndian.AppendUint32(out, uint32(len(spec.audio)))
	out = append(out, spec.audio...)
	binary.LittleEndian.PutUint32(out[riffChunkSizeOffset:], uint32(len(out)-chunkHeaderSize))
	return out
}

func TestInspectReadsExtensibleFormat(t *testing.T) {
	info, err := Inspect(buildExtensibleWAV(defaultExtensibleSpec([]byte{1, 2, 3, 4})))
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}

	if info.Format.AudioFormat != extensibleFormatTag {
		t.Errorf("AudioFormat = %#x, want %#x", info.Format.AudioFormat, extensibleFormatTag)
	}
	if info.Format.ChannelMask != 0x3 {
		t.Errorf("ChannelMask = %#x, want 0x3", info.Format.ChannelMask)
	}
	if info.Format.SubFormat != subFormatPCM {
		t.Errorf("SubFormat = %s, want %s", formatGUID(info.Format.SubFormat), formatGUID(subFormatPCM))
	}
	if got := info.Format.AudioFormatTag(); got != pcmFormatTag {
		t.Errorf("AudioFormatTag() = %d, want %d", got, pcmFormatTag)
	}
}

// TestCombineWavDataRejectsMismatchedSubFormat は、拡張形式どうしでサブフォーマットが
// 違う WAV を結合しないことを確認します。
//
// 拡張形式では AudioFormat が常に 0xFFFE なので、GUID を見ないとリニア PCM と
// IEEE float が「どちらも 0xFFFE」として一致扱いになり、無音やノイズになります。
func TestCombineWavDataRejectsMismatchedSubFormat(t *testing.T) {
	second := defaultExtensibleSpec([]byte{5, 6, 7, 8})
	second.subFormat = subFormatFloat

	_, err := CombineWavData([][]byte{
		buildExtensibleWAV(defaultExtensibleSpec([]byte{1, 2, 3, 4})),
		buildExtensibleWAV(second),
	})
	if err == nil {
		t.Fatal("CombineWavData() error = nil, want ErrMismatchedSubFormat")
	}
	mismatch, ok := errors.AsType[*ErrMismatchedSubFormat](err)
	if !ok {
		t.Fatalf("error type = %T, want *ErrMismatchedSubFormat", err)
	}
	if mismatch.Index != 1 {
		t.Errorf("Index = %d, want 1", mismatch.Index)
	}
	if mismatch.First != subFormatPCM || mismatch.Got != subFormatFloat {
		t.Errorf("First/Got = %s/%s, want %s/%s",
			formatGUID(mismatch.First), formatGUID(mismatch.Got),
			formatGUID(subFormatPCM), formatGUID(subFormatFloat))
	}
}

// TestCombineWavDataRejectsMismatchedChannelMask は、チャンネル数が同じでもスピーカー
// 配置が違えば結合しないことを確認します。2ch のまま FL/FR と FC/LFE を繋ぐと、
// 2 本目以降が別のスピーカーへ割り当てられます。
func TestCombineWavDataRejectsMismatchedChannelMask(t *testing.T) {
	second := defaultExtensibleSpec([]byte{5, 6, 7, 8})
	second.channelMask = 0xC // FC | LFE

	_, err := CombineWavData([][]byte{
		buildExtensibleWAV(defaultExtensibleSpec([]byte{1, 2, 3, 4})),
		buildExtensibleWAV(second),
	})
	mismatch, ok := errors.AsType[*ErrMismatchedWAVFormat](err)
	if !ok {
		t.Fatalf("error type = %T, want *ErrMismatchedWAVFormat", err)
	}
	if mismatch.Field != "チャンネルマスク" {
		t.Errorf("Field = %q, want チャンネルマスク", mismatch.Field)
	}
}

func TestCombineWavDataAcceptsIdenticalExtensibleFormat(t *testing.T) {
	combined, err := CombineWavData([][]byte{
		buildExtensibleWAV(defaultExtensibleSpec([]byte{1, 2, 3, 4})),
		buildExtensibleWAV(defaultExtensibleSpec([]byte{5, 6, 7, 8})),
	})
	if err != nil {
		t.Fatalf("CombineWavData() error = %v", err)
	}
	info, err := Inspect(combined)
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	if info.DataSize != 8 {
		t.Errorf("DataSize = %d, want 8", info.DataSize)
	}
	if info.Format.SubFormat != subFormatPCM {
		t.Errorf("SubFormat = %s, want %s", formatGUID(info.Format.SubFormat), formatGUID(subFormatPCM))
	}
}

// TestParseFormatChunkRejectsTruncatedExtensible は、0xFFFE を名乗りながら拡張部を
// 持たない fmt チャンクを弾くことを確認します。読み飛ばすと SubFormat が零値になり、
// 本来別物のファイルが一致扱いになります。
func TestParseFormatChunkRejectsTruncatedExtensible(t *testing.T) {
	spec := defaultExtensibleSpec([]byte{1, 2})
	spec.fmtChunkSize = minFormatChunkSize

	_, err := Inspect(buildExtensibleWAV(spec))
	if _, ok := errors.AsType[*ErrInvalidWAVHeader](err); !ok {
		t.Fatalf("error type = %T, want *ErrInvalidWAVHeader", err)
	}
}

func TestFormatGUID(t *testing.T) {
	if got, want := formatGUID(subFormatPCM), "{00000001-0000-0010-8000-00AA00389B71}"; got != want {
		t.Errorf("formatGUID() = %s, want %s", got, want)
	}
}

func TestFormatSilenceByte(t *testing.T) {
	tests := []struct {
		name   string
		format Format
		want   byte
	}{
		// 8bit のリニア PCM だけが符号なし。中央の 0x80 が無音になる。
		{"8bit PCM", Format{AudioFormat: pcmFormatTag, BitsPerSample: 8}, 0x80},
		{"16bit PCM", Format{AudioFormat: pcmFormatTag, BitsPerSample: 16}, 0x00},
		{"32bit float", Format{AudioFormat: 3, BitsPerSample: 32}, 0x00},
		{"拡張形式の8bit PCM", Format{AudioFormat: extensibleFormatTag, BitsPerSample: 8, SubFormat: subFormatPCM}, 0x80},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.format.silenceByte(); got != tt.want {
				t.Errorf("silenceByte() = %#x, want %#x", got, tt.want)
			}
		})
	}
}
