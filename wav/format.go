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
	// 拡張形式では、1サンプルを格納する箱の大きさを表します。
	BitsPerSample uint16
	// ValidBitsPerSample は BitsPerSample のうち実際に意味を持つビット数です
	// (例: 32 ビットの箱に 24 ビットの音を入れる)。
	// WAVE_FORMAT_EXTENSIBLE のときだけ値を持ち、それ以外の形式では 0 です。
	ValidBitsPerSample uint16
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

// mulDiv は value×numerator÷denominator を、value を denominator で割った商と余りに
// 分けて計算します。先に掛けると、長い時間や高いレートで uint64 を超えるためです。
// 商は整数なので、余りの側だけ割れば結果は切り捨てと一致します。
func mulDiv(value, numerator, denominator uint64) uint64 {
	return value/denominator*numerator + value%denominator*numerator/denominator
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
// 符号化方式ごとに「振幅ゼロ」を表すバイトが違います。0x00 で埋めてよいのは符号付きの
// 形式だけで、それ以外に 0x00 を書くと無音ではなく雑音になります。
//
//   - 8bit のリニア PCM は符号なし (0〜255 の中央が無音) なので 0x80。
//   - μ-law は符号ビットと振幅を反転して格納するため 0xFF。
//   - A-law は偶数ビットを反転する規則 (0x55 との XOR) があるため 0xD5。
//   - 16bit 以上の整数 PCM と IEEE float は符号付きなので 0x00。
func (f Format) silenceByte() byte {
	switch f.AudioFormatTag() {
	case pcmFormatTag:
		if f.BitsPerSample == 8 {
			return 0x80
		}
	case muLawFormatTag:
		return 0xFF
	case aLawFormatTag:
		return 0xD5
	}
	return 0x00
}

// 符号化方式を表す AudioFormat の値です（WAVE_FORMAT_*）。
const (
	// pcmFormatTag はリニア PCM です。
	pcmFormatTag = 1
	// floatFormatTag は IEEE float です。
	floatFormatTag = 3
	// aLawFormatTag は G.711 A-law です。
	aLawFormatTag = 6
	// muLawFormatTag は G.711 μ-law です。
	muLawFormatTag = 7
)

// subFormatGUIDSuffix は標準のサブフォーマット GUID (KSDATAFORMAT_SUBTYPE_*) に共通する
// 後半 12 バイトです。先頭 4 バイトのうち下位 2 バイトが符号化方式、上位 2 バイトは 0 です。
var subFormatGUIDSuffix = [12]byte{0x00, 0x00, 0x10, 0x00, 0x80, 0x00, 0x00, 0xAA, 0x00, 0x38, 0x9B, 0x71}

// verifySupported は、デコードせずに繋げる形式かどうかを確認します。
//
// 対応するのは、1 サンプルのバイト数が量子化ビット数とチャンネル数だけで決まり、
// 無音を表すバイトが分かる形式（リニア PCM、IEEE float、G.711 A-law / μ-law）です。
// ADPCM や MP3 を格納した WAV などの圧縮形式は、ブロック境界も無音も量子化ビット数からは
// 決まらないため、繋いでも正しい WAV になる保証がありません。
func (f Format) verifySupported(index int) error {
	if f.AudioFormat == extensibleFormatTag &&
		(binary.LittleEndian.Uint16(f.SubFormat[2:4]) != 0 || [12]byte(f.SubFormat[4:]) != subFormatGUIDSuffix) {
		return &ErrUnsupportedWAVFormat{Index: index, Format: f}
	}

	var supported bool
	switch f.AudioFormatTag() {
	case pcmFormatTag:
		supported = f.BitsPerSample > 0 && f.BitsPerSample%8 == 0
	case floatFormatTag:
		supported = f.BitsPerSample == 32 || f.BitsPerSample == 64
	case aLawFormatTag, muLawFormatTag:
		supported = f.BitsPerSample == 8
	}
	if !supported {
		return &ErrUnsupportedWAVFormat{Index: index, Format: f}
	}
	return nil
}

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
