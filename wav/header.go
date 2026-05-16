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

// extractAudioData は WAV ファイルからフォーマットヘッダー情報と音声データ部分を抽出します。
// fmt および data チャンクを動的に探索し、data チャンクの直前までを formatHeader とします。
func extractAudioData(wavBytes []byte, index int) (formatHeader []byte, audioData []byte, err error) {
	if err := validateRiffHeader(wavBytes, index); err != nil {
		return nil, nil, err
	}

	dataChunk, err := findAudioDataChunk(wavBytes, index)
	if err != nil {
		return nil, nil, err
	}

	formatHeader = wavBytes[0:dataChunk.offset]
	audioData = chunkPayload(wavBytes, dataChunk)

	// 抽出されたデータサイズがヘッダーの記載と一致するか最終確認
	headerDataSize := binary.LittleEndian.Uint32(wavBytes[dataChunk.offset+dataChunkIDSize : dataChunk.offset+dataChunkHeaderSize])
	if uint64(len(audioData)) != uint64(headerDataSize) {
		return nil, nil, &ErrInvalidWAVHeader{
			Index:   index,
			Details: "最終的な抽出データサイズがヘッダー記載サイズと一致しません",
		}
	}

	return formatHeader, audioData, nil
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

// findAudioDataChunk は WAV チャンク列から data チャンクを探します。
func findAudioDataChunk(wavBytes []byte, index int) (wavChunk, error) {
	var fmtChunkFound bool

	for _, chunk := range scanWavChunks(wavBytes) {
		if chunk.id == "fmt " {
			fmtChunkFound = true
		}
		if chunk.id == "data" {
			return validateDataChunk(wavBytes, chunk, index, fmtChunkFound)
		}
	}

	return wavChunk{}, missingWavChunkError(index, fmtChunkFound, false)
}

// scanWavChunks は RIFF ヘッダー以降のチャンクメタデータを順番に読み取ります。
func scanWavChunks(wavBytes []byte) []wavChunk {
	var chunks []wavChunk

	for offset := wavRiffHeaderSize; offset < len(wavBytes); {
		if offset+dataChunkHeaderSize > len(wavBytes) {
			return chunks
		}

		chunk := wavChunk{
			id:     string(wavBytes[offset : offset+dataChunkIDSize]),
			offset: offset,
			size:   binary.LittleEndian.Uint32(wavBytes[offset+dataChunkIDSize : offset+dataChunkHeaderSize]),
		}
		chunks = append(chunks, chunk)

		nextOffset := nextChunkOffset(offset, chunk.size)
		if nextOffset > uint64(len(wavBytes)) {
			return chunks
		}
		offset = int(nextOffset)
	}

	return chunks
}

// nextChunkOffset は WAV チャンクのパディングを考慮して次のチャンク位置を返します。
func nextChunkOffset(offset int, chunkSize uint32) uint64 {
	nextOffset := uint64(offset) + uint64(dataChunkHeaderSize) + uint64(chunkSize)
	if chunkSize%2 != 0 {
		nextOffset++
	}
	return nextOffset
}

// validateDataChunk は data チャンクのサイズと fmt チャンクの存在を検証します。
func validateDataChunk(wavBytes []byte, chunk wavChunk, index int, fmtChunkFound bool) (wavChunk, error) {
	audioDataStart := chunk.offset + dataChunkHeaderSize
	// int の加算オーバーフローを避けるため、残量との比較は uint64 で行う。
	remainingBytes := uint64(len(wavBytes) - audioDataStart)
	if uint64(chunk.size) > remainingBytes {
		return wavChunk{}, &ErrInvalidWAVHeader{
			Index:   index,
			Details: "dataチャンクのデータ長が実際のファイルサイズを超過しています",
		}
	}
	if !fmtChunkFound {
		return wavChunk{}, missingWavChunkError(index, false, true)
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
