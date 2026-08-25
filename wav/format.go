package wav

import (
	"encoding/binary"
	"fmt"
)

// Format は WAV の fmt チャンクから読み取った再生フォーマットです。
type Format struct {
	// AudioFormat は音声データの符号化方式です (1 = リニア PCM、3 = IEEE float、
	// 0xFFFE = WAVE_FORMAT_EXTENSIBLE)。
	AudioFormat uint16
	// NumChannels はチャンネル数です (1 = モノラル、2 = ステレオ)。
	NumChannels uint16
	// SampleRate は1秒あたりのサンプル数 (Hz) です。
	SampleRate uint32
	// BitsPerSample は1サンプルあたりのビット数です。
	BitsPerSample uint16
	// ChannelMask は各チャンネルのスピーカー配置を表すビットマスクです。
	// WAVE_FORMAT_EXTENSIBLE のときだけ値を持ち、それ以外の形式では 0 です。
	ChannelMask uint32
	// SubFormat は WAVE_FORMAT_EXTENSIBLE における実際の符号化方式を表す GUID です。
	// それ以外の形式では零値です。
	SubFormat [16]byte
}

// AudioFormatTag は実際の符号化方式を返します。
//
// WAVE_FORMAT_EXTENSIBLE では AudioFormat が常に 0xFFFE になり、符号化方式は
// SubFormat GUID の先頭 2 バイトに入っています。この違いを吸収して、拡張形式でも
// 通常形式と同じ値 (1 = リニア PCM、3 = IEEE float) で比較できるようにします。
func (f Format) AudioFormatTag() uint16 {
	if f.AudioFormat != extensibleFormatTag {
		return f.AudioFormat
	}
	return binary.LittleEndian.Uint16(f.SubFormat[0:2])
}

// byteRate は1秒あたりのバイト数を返します。
func (f Format) byteRate() uint64 {
	return uint64(f.SampleRate) * uint64(f.blockAlign())
}

// blockAlign は全チャンネル分の1サンプルのバイト数を返します。
// 無音を挿入する際は、この単位で区切らないとチャンネルの並びがずれます。
func (f Format) blockAlign() uint32 {
	return uint32(f.NumChannels) * uint32(f.BitsPerSample) / 8
}

// silenceByte は無音を表すバイト値を返します。
//
// 8bit のリニア PCM だけは符号なし (0〜255 の中央が無音) なので 0x80 になります。
// 16bit 以上の整数 PCM と IEEE float はいずれも符号付きで、無音は 0x00 です。
func (f Format) silenceByte() byte {
	if f.AudioFormatTag() == pcmFormatTag && f.BitsPerSample == 8 {
		return 0x80
	}
	return 0x00
}

// pcmFormatTag はリニア PCM を表す AudioFormat の値です。
const pcmFormatTag = 1

// formatGUID は 16 バイトのサブフォーマット GUID を正準表記の文字列にします。
// 先頭 3 グループはリトルエンディアン、残り 2 グループはバイト順のままです。
func formatGUID(guid [16]byte) string {
	return fmt.Sprintf("{%08X-%04X-%04X-%04X-%012X}",
		binary.LittleEndian.Uint32(guid[0:4]),
		binary.LittleEndian.Uint16(guid[4:6]),
		binary.LittleEndian.Uint16(guid[6:8]),
		binary.BigEndian.Uint16(guid[8:10]),
		guid[10:16])
}
