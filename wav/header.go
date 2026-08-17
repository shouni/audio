package wav

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
)

// RIFF 構造および WAV ファイルの解析に必要なサイズ定数です。
const (
	// riffChunkIDSize は "RIFF" チャンクIDのサイズ（バイト）です。
	riffChunkIDSize = 4
	// riffChunkSizeSize はファイルサイズフィールドのサイズ（バイト）です。
	riffChunkSizeSize = 4
	// waveIDSize は "WAVE" 識別子のサイズ（バイト）です。
	waveIDSize = 4

	// dataChunkIDSize は "data" チャンクIDのサイズ（バイト）です。
	dataChunkIDSize = 4
	// dataChunkSizeSize はデータサイズフィールドのサイズ（バイト）です。
	dataChunkSizeSize = 4
)

// WAV ファイルのヘッダー計算やロジックで使用される複合サイズ定数です。
const (
	// TotalHeaderSize は一般的な WAV ファイルの最小ヘッダーサイズ（44バイト）です。
	TotalHeaderSize = 44
	// dataChunkHeaderSize は "data" チャンクヘッダーの合計サイズ（8バイト）です。
	dataChunkHeaderSize = dataChunkIDSize + dataChunkSizeSize
	// minFormatChunkSize は PCM の "fmt " チャンクのペイロードサイズ（16バイト）です。
	minFormatChunkSize = 16
	// wavRiffHeaderSize は RIFF ヘッダーの合計サイズ（12バイト）です。
	wavRiffHeaderSize = riffChunkIDSize + riffChunkSizeSize + waveIDSize
)

// ファイルのバイナリ操作時に使用されるオフセット定数です。
const (
	// riffChunkSizeOffset は、ファイル結合時に RIFF チャンクサイズを更新するために必要な、
	// RIFF チャンクサイズが書き込まれるオフセット位置（4バイト目）です。
	riffChunkSizeOffset = riffChunkIDSize
)

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
	if !bytes.Equal(wavBytes[0:riffChunkIDSize], []byte("RIFF")) || !bytes.Equal(wavBytes[riffChunkIDSize+riffChunkSizeSize:wavRiffHeaderSize], []byte("WAVE")) {
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
		if offset+dataChunkHeaderSize > len(wavBytes) {
			break
		}

		chunk := wavChunk{
			id:     string(wavBytes[offset : offset+dataChunkIDSize]),
			offset: offset,
			size:   binary.LittleEndian.Uint32(wavBytes[offset+dataChunkIDSize : offset+dataChunkHeaderSize]),
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
				return Format{}, wavChunk{}, missingWavChunkError(index, false, true)
			}
			dataChunk, err := validateDataChunk(wavBytes, chunk, index)
			if err != nil {
				return Format{}, wavChunk{}, err
			}
			return format, dataChunk, nil
		}

		nextOffset := nextChunkOffset(offset, chunk.size)
		if nextOffset > uint64(len(wavBytes)) {
			break
		}
		offset = int(nextOffset)
	}

	return Format{}, wavChunk{}, missingWavChunkError(index, fmtChunkFound, false)
}

// parseFormatChunk は fmt チャンクから、結合可否の判定に使う値を読み出します。
func parseFormatChunk(wavBytes []byte, chunk wavChunk, index int) (Format, error) {
	payloadStart := chunk.offset + dataChunkHeaderSize
	// PCM の fmt チャンクは 16 バイト。拡張形式はより長いが、先頭 16 バイトの並びは共通。
	if uint64(chunk.size) < minFormatChunkSize || payloadStart+minFormatChunkSize > len(wavBytes) {
		return Format{}, &ErrInvalidWAVHeader{
			Index:   index,
			Details: fmt.Sprintf("fmtチャンクが短すぎます (%dバイト、最低%dバイト必要)", chunk.size, minFormatChunkSize),
		}
	}

	return Format{
		AudioFormat:   binary.LittleEndian.Uint16(wavBytes[payloadStart:]),
		NumChannels:   binary.LittleEndian.Uint16(wavBytes[payloadStart+2:]),
		SampleRate:    binary.LittleEndian.Uint32(wavBytes[payloadStart+4:]),
		BitsPerSample: binary.LittleEndian.Uint16(wavBytes[payloadStart+14:]),
	}, nil
}

// nextChunkOffset は WAV チャンクのパディングを考慮して次のチャンク位置を返します。
func nextChunkOffset(offset int, chunkSize uint32) uint64 {
	nextOffset := uint64(offset) + uint64(dataChunkHeaderSize) + uint64(chunkSize)
	if chunkSize%2 != 0 {
		nextOffset++
	}
	return nextOffset
}

// validateDataChunk は data チャンクのサイズを検証します。
func validateDataChunk(wavBytes []byte, chunk wavChunk, index int) (wavChunk, error) {
	audioDataStart := chunk.offset + dataChunkHeaderSize
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

// missingWavChunkError は不足している必須チャンクを示すエラーを作成します。
func missingWavChunkError(index int, fmtChunkFound, dataChunkFound bool) error {
	missingChunk := ""
	if !fmtChunkFound {
		missingChunk = "'fmt '"
	}
	if !dataChunkFound {
		missingChunk = appendMissingChunk(missingChunk, "'data'")
	}

	return &ErrInvalidWAVHeader{
		Index:   index,
		Details: fmt.Sprintf("WAVファイル内に必要なチャンク (%s) が見つかりませんでした", missingChunk),
	}
}

// appendMissingChunk は不足チャンク名をエラーメッセージ用に連結します。
func appendMissingChunk(current, next string) string {
	if current == "" {
		return next
	}
	return current + " and " + next
}

// chunkPayload は WAV チャンクに対応するデータ部分を返します。
func chunkPayload(wavBytes []byte, chunk wavChunk) []byte {
	audioDataStart := chunk.offset + dataChunkHeaderSize
	audioDataEnd := audioDataStart + int(chunk.size)
	return wavBytes[audioDataStart:audioDataEnd]
}

// buildCombinedWav はオーディオパーツのスライスを一括でコピーして WAV ファイルを再構築します。
func buildCombinedWav(formatHeader []byte, audioParts [][]byte, totalAudioSize int) ([]byte, error) {
	dataChunkStart := len(formatHeader)
	dataChunkSizeOffset := dataChunkStart + dataChunkIDSize
	finalWavHeaderSize := dataChunkStart + dataChunkHeaderSize

	// RIFFチャンクサイズ = (全ヘッダー + 全データ) - 8
	fileSize := totalAudioSize + finalWavHeaderSize - (riffChunkIDSize + riffChunkSizeSize)

	if uint64(fileSize) > math.MaxUint32 {
		return nil, fmt.Errorf("結合後のWAVファイルサイズが4GBを超過しています")
	}

	// 最終的な出力バッファを一度だけ make
	combinedWav := make([]byte, finalWavHeaderSize+totalAudioSize)

	// ヘッダー情報の書き込み
	copy(combinedWav, formatHeader)
	copy(combinedWav[dataChunkStart:], []byte("data"))

	// サイズメタデータの更新
	binary.LittleEndian.PutUint32(combinedWav[riffChunkSizeOffset:riffChunkSizeOffset+4], uint32(fileSize))
	binary.LittleEndian.PutUint32(combinedWav[dataChunkSizeOffset:dataChunkSizeOffset+4], uint32(totalAudioSize))

	// 各セクションのデータをループで順番にコピー
	currentOffset := finalWavHeaderSize
	for _, part := range audioParts {
		copy(combinedWav[currentOffset:], part)
		currentOffset += len(part)
	}

	return combinedWav, nil
}
