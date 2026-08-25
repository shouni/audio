package wav

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strings"
)

// RIFF 構造の解析に使用するサイズ定数です。
const (
	// chunkIDSize は、チャンクID ("RIFF", "fmt ", "data" 等) のサイズ（バイト）です。
	chunkIDSize = 4
	// chunkSizeSize は、チャンクサイズフィールドのサイズ（バイト）です。
	chunkSizeSize = 4
	// waveIDSize は "WAVE" 識別子のサイズ（バイト）です。
	waveIDSize = 4
)

// WAV ファイルのヘッダー計算で使用される複合サイズ定数です。
const (
	// standardHeaderSize は、拡張チャンクを持たない WAV のヘッダーサイズ（44バイト）です。
	standardHeaderSize = 44
	// chunkHeaderSize は、チャンクヘッダー (ID + サイズ) の合計サイズ（8バイト）です。
	chunkHeaderSize = chunkIDSize + chunkSizeSize
	// minFormatChunkSize は PCM の "fmt " チャンクのペイロードサイズ（16バイト）です。
	minFormatChunkSize = 16
	// extensibleFormatChunkSize は WAVE_FORMAT_EXTENSIBLE の "fmt " チャンクの
	// ペイロードサイズ（40バイト）です。
	extensibleFormatChunkSize = 40
	// wavRiffHeaderSize は RIFF ヘッダーの合計サイズ（12バイト）です。
	wavRiffHeaderSize = chunkIDSize + chunkSizeSize + waveIDSize
)

// riffChunkSizeOffset は、結合時に RIFF チャンクサイズを書き換えるオフセット位置（4バイト目）です。
const riffChunkSizeOffset = chunkIDSize

// extensibleFormatTag は WAVE_FORMAT_EXTENSIBLE を表す AudioFormat の値です。
// この値のときだけ、実際の符号化方式は fmt チャンク末尾の SubFormat GUID 側にあります。
const extensibleFormatTag = 0xFFFE

type wavChunk struct {
	id     string
	offset int
	size   uint32
}

// wavParts は1つの WAV から取り出した、結合に必要な部品です。
type wavParts struct {
	// formatHeader は data チャンク直前までの、そのまま出力へ引き継ぐヘッダーです。
	formatHeader []byte
	format       Format
	audioData    []byte
}

// extractAudioData は WAV ファイルからフォーマット情報と音声データ部分を抽出します。
// fmt および data チャンクを動的に探索し、data チャンクの直前までを formatHeader とします。
// index はエラーメッセージに含めるファイル位置で、一覧に属さない検証では -1 を渡します。
func extractAudioData(wavBytes []byte, index int) (wavParts, error) {
	if err := validateRiffHeader(wavBytes, index); err != nil {
		return wavParts{}, err
	}

	format, dataChunk, err := scanWavChunks(wavBytes, index)
	if err != nil {
		return wavParts{}, err
	}

	return wavParts{
		formatHeader: wavBytes[0:dataChunk.offset],
		format:       format,
		audioData:    chunkPayload(wavBytes, dataChunk),
	}, nil
}

// validateRiffHeader は WAV データの RIFF/WAVE 識別子を検証します。
func validateRiffHeader(wavBytes []byte, index int) error {
	if len(wavBytes) < wavRiffHeaderSize {
		return &ErrInvalidWAVHeader{
			Index:   index,
			Details: fmt.Sprintf("WAVファイルサイズが短すぎます (RIFFヘッダー不足: %dバイト)", len(wavBytes)),
		}
	}
	if !bytes.Equal(wavBytes[0:chunkIDSize], []byte("RIFF")) || !bytes.Equal(wavBytes[chunkHeaderSize:wavRiffHeaderSize], []byte("WAVE")) {
		return &ErrInvalidWAVHeader{
			Index:   index,
			Details: "RIFF/WAVE識別子が不正です",
		}
	}
	return nil
}

// scanWavChunks は WAV チャンク列を走査し、fmt チャンクの内容と data チャンクを返します。
func scanWavChunks(wavBytes []byte, index int) (Format, wavChunk, error) {
	var (
		format        Format
		fmtChunkFound bool
	)

	for offset := wavRiffHeaderSize; offset < len(wavBytes); {
		if offset+chunkHeaderSize > len(wavBytes) {
			break
		}

		chunk := wavChunk{
			id:     string(wavBytes[offset : offset+chunkIDSize]),
			offset: offset,
			size:   binary.LittleEndian.Uint32(wavBytes[offset+chunkIDSize : offset+chunkHeaderSize]),
		}

		if chunk.id == "fmt " {
			parsed, err := parseFormatChunk(wavBytes, chunk, index)
			if err != nil {
				return Format{}, wavChunk{}, err
			}
			format = parsed
			fmtChunkFound = true
		}
		if chunk.id == "data" {
			if !fmtChunkFound {
				return Format{}, wavChunk{}, missingChunkError(index, "'fmt '")
			}
			dataChunk, err := validateDataChunk(wavBytes, chunk, index)
			if err != nil {
				return Format{}, wavChunk{}, err
			}
			return format, dataChunk, nil
		}

		nextOffset := nextChunkOffset(uint64(offset), chunk.size)
		if nextOffset > uint64(len(wavBytes)) {
			break
		}
		offset = int(nextOffset)
	}

	if fmtChunkFound {
		return Format{}, wavChunk{}, missingChunkError(index, "'data'")
	}
	return Format{}, wavChunk{}, missingChunkError(index, "'fmt '", "'data'")
}

// parseFormatChunk は fmt チャンクから、結合可否の判定に使う値を読み出します。
func parseFormatChunk(wavBytes []byte, chunk wavChunk, index int) (Format, error) {
	payloadStart := chunk.offset + chunkHeaderSize
	if payloadStart > len(wavBytes) {
		payloadStart = len(wavBytes)
	}
	payloadEnd := min(payloadStart+extensibleFormatChunkSize, len(wavBytes))
	return parseFormatPayload(wavBytes[payloadStart:payloadEnd], chunk.size, index)
}

// parseFormatPayload は fmt チャンクのペイロードからフォーマットを読み出します。
// declaredSize はチャンクヘッダーが宣言しているサイズで、payload はその先頭部分
// （最大 extensibleFormatChunkSize バイト）です。
//
// WAVE_FORMAT_EXTENSIBLE (AudioFormat = 0xFFFE) では実際の符号化方式もチャンネル配置も
// 先頭 16 バイトには入っていないため、拡張部（チャンネルマスクとサブフォーマット GUID）
// まで読み出します。ここを読まないと、リニア PCM と IEEE float が「どちらも 0xFFFE」
// として一致扱いになります。
func parseFormatPayload(payload []byte, declaredSize uint32, index int) (Format, error) {
	// PCM の fmt チャンクは 16 バイト。拡張形式はより長いが、先頭 16 バイトの並びは共通。
	if uint64(declaredSize) < minFormatChunkSize || len(payload) < minFormatChunkSize {
		return Format{}, &ErrInvalidWAVHeader{
			Index:   index,
			Details: fmt.Sprintf("fmtチャンクが短すぎます (%dバイト、最低%dバイト必要)", declaredSize, minFormatChunkSize),
		}
	}

	format := Format{
		AudioFormat:   binary.LittleEndian.Uint16(payload[0:]),
		NumChannels:   binary.LittleEndian.Uint16(payload[2:]),
		SampleRate:    binary.LittleEndian.Uint32(payload[4:]),
		BitsPerSample: binary.LittleEndian.Uint16(payload[14:]),
	}
	if format.AudioFormat != extensibleFormatTag {
		return format, nil
	}

	if uint64(declaredSize) < extensibleFormatChunkSize || len(payload) < extensibleFormatChunkSize {
		return Format{}, &ErrInvalidWAVHeader{
			Index:   index,
			Details: fmt.Sprintf("拡張形式(WAVE_FORMAT_EXTENSIBLE)のfmtチャンクが短すぎます (%dバイト、最低%dバイト必要)", declaredSize, extensibleFormatChunkSize),
		}
	}
	format.ChannelMask = binary.LittleEndian.Uint32(payload[20:])
	copy(format.SubFormat[:], payload[24:extensibleFormatChunkSize])
	return format, nil
}

// nextChunkOffset は WAV チャンクのパディングを考慮して次のチャンク位置を返します。
func nextChunkOffset(offset uint64, chunkSize uint32) uint64 {
	nextOffset := offset + uint64(chunkHeaderSize) + uint64(chunkSize)
	if chunkSize%2 != 0 {
		nextOffset++
	}
	return nextOffset
}

// validateDataChunk は data チャンクのサイズを検証します。
func validateDataChunk(wavBytes []byte, chunk wavChunk, index int) (wavChunk, error) {
	audioDataStart := chunk.offset + chunkHeaderSize
	// int の加算オーバーフローを避けるため、残量との比較は uint64 で行う。
	remainingBytes := uint64(len(wavBytes) - audioDataStart)
	if uint64(chunk.size) > remainingBytes {
		return wavChunk{}, &ErrInvalidWAVHeader{
			Index:   index,
			Details: "dataチャンクのデータ長が実際のファイルサイズを超過しています",
		}
	}

	return chunk, nil
}

// missingChunkError は不足している必須チャンクを示すエラーを作成します。
func missingChunkError(index int, missing ...string) error {
	return &ErrInvalidWAVHeader{
		Index:   index,
		Details: fmt.Sprintf("WAVファイル内に必要なチャンク (%s) が見つかりませんでした", strings.Join(missing, "と")),
	}
}

// chunkPayload は WAV チャンクに対応するデータ部分を返します。
func chunkPayload(wavBytes []byte, chunk wavChunk) []byte {
	audioDataStart := chunk.offset + chunkHeaderSize
	audioDataEnd := audioDataStart + int(chunk.size)
	return wavBytes[audioDataStart:audioDataEnd]
}
