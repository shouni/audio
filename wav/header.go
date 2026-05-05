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
	// dataChunkHeaderSize は "data" チャンクヘッダーの合計サイズ（8バイト）です。
	dataChunkHeaderSize = dataChunkIDSize + dataChunkSizeSize
	// wavRiffHeaderSize は RIFF ヘッダーの合計サイズ（12バイト）です。
	wavRiffHeaderSize = riffChunkIDSize + riffChunkSizeSize + waveIDSize
	// wavTotalHeaderSize は一般的な WAV ファイルの最小ヘッダーサイズ（44バイト）です。
	wavTotalHeaderSize = 44
)

// ファイルのバイナリ操作時に使用されるオフセット定数です。
const (
	// riffChunkSizeOffset は、ファイル結合時に RIFF チャンクサイズを更新するために必要な、
	// RIFF チャンクサイズが書き込まれるオフセット位置（4バイト目）です。
	riffChunkSizeOffset = riffChunkIDSize
)

// extractAudioData は WAV ファイルからフォーマットヘッダー情報と音声データ部分を抽出します。
// fmt および data チャンクを動的に探索し、data チャンクの直前までを formatHeader とします。
func extractAudioData(wavBytes []byte, index int) (formatHeader []byte, audioData []byte, err error) {
	if len(wavBytes) < wavRiffHeaderSize {
		return nil, nil, &ErrInvalidWAVHeader{
			Index:   index,
			Details: fmt.Sprintf("WAVファイルサイズが短すぎます (RIFFヘッダー不足: %dバイト)", len(wavBytes)),
		}
	}
	if !bytes.Equal(wavBytes[0:riffChunkIDSize], []byte("RIFF")) || !bytes.Equal(wavBytes[riffChunkIDSize+riffChunkSizeSize:wavRiffHeaderSize], []byte("WAVE")) {
		return nil, nil, &ErrInvalidWAVHeader{
			Index:   index,
			Details: "RIFF/WAVE識別子が不正です",
		}
	}

	var fmtChunkFound, dataChunkFound bool
	var dataChunkStart int

	offset := wavRiffHeaderSize

	for offset < len(wavBytes) {
		if offset+dataChunkHeaderSize > len(wavBytes) {
			break
		}

		chunkID := string(wavBytes[offset : offset+dataChunkIDSize])
		chunkSize := binary.LittleEndian.Uint32(wavBytes[offset+dataChunkIDSize : offset+dataChunkHeaderSize])

		if chunkID == "fmt " {
			fmtChunkFound = true
		}

		if chunkID == "data" {
			dataChunkFound = true
			dataChunkStart = offset

			audioDataStart := offset + dataChunkHeaderSize
			// int の加算オーバーフローを避けるため、残量との比較は uint64 で行う。
			remainingBytes := uint64(len(wavBytes) - audioDataStart)
			if uint64(chunkSize) > remainingBytes {
				return nil, nil, &ErrInvalidWAVHeader{
					Index:   index,
					Details: "dataチャンクのデータ長が実際のファイルサイズを超過しています",
				}
			}

			audioDataEnd := audioDataStart + int(chunkSize)
			audioData = wavBytes[audioDataStart:audioDataEnd]
			break
		}

		// 次のチャンクへ移動
		// chunkSize が巨大な場合のオーバーフローを考慮し、ここでもチェックが必要です
		nextOffset := uint64(offset) + uint64(dataChunkHeaderSize) + uint64(chunkSize)
		if chunkSize%2 != 0 {
			nextOffset++
		}

		if nextOffset > uint64(len(wavBytes)) {
			// dataチャンクが見つかる前に末尾を超えてしまう場合
			break
		}
		offset = int(nextOffset)
	}

	if !fmtChunkFound || !dataChunkFound {
		missingChunk := ""
		if !fmtChunkFound {
			missingChunk += "'fmt '"
		}
		if !dataChunkFound {
			if missingChunk != "" {
				missingChunk += " and "
			}
			missingChunk += "'data'"
		}
		return nil, nil, &ErrInvalidWAVHeader{
			Index:   index,
			Details: fmt.Sprintf("WAVファイル内に必要なチャンク (%s) が見つかりませんでした", missingChunk),
		}
	}

	formatHeader = wavBytes[0:dataChunkStart]

	// 抽出されたデータサイズがヘッダーの記載と一致するか最終確認
	headerDataSize := binary.LittleEndian.Uint32(wavBytes[dataChunkStart+dataChunkIDSize : dataChunkStart+dataChunkHeaderSize])
	if uint64(len(audioData)) != uint64(headerDataSize) {
		return nil, nil, &ErrInvalidWAVHeader{
			Index:   index,
			Details: "最終的な抽出データサイズがヘッダー記載サイズと一致しません",
		}
	}

	return formatHeader, audioData, nil
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
