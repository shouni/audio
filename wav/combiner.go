package wav

import (
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

// ErrNoAudioData は、結合対象の音声データがない場合に発生します。
type ErrNoAudioData struct{}

func (e *ErrNoAudioData) Error() string {
	return "結合対象の音声データがありません"
}

// ErrInvalidWAVHeader は、WAV ヘッダーの検証に失敗した場合に発生します。
type ErrInvalidWAVHeader struct {
	// Index はエラーが発生した WAV ファイルのインデックスです。
	Index int
	// Details はエラーの詳細情報です。
	Details string
}

func (e *ErrInvalidWAVHeader) Error() string {
	if e.Index >= 0 {
		return fmt.Sprintf("WAVファイル #%d のヘッダーが無効です: %s", e.Index, e.Details)
	}
	return fmt.Sprintf("WAVヘッダーが無効です: %s", e.Details)
}

// CombineWavData は複数の WAV データを結合し、単一の WAV データを生成します。
// メモリ効率を最適化するため、結合前のオーディオデータをスライスで保持し、
// 最終的なバッファ構築時に一度だけコピーを行います。
func CombineWavData(wavDataList [][]byte) ([]byte, error) {
	if len(wavDataList) == 0 {
		return nil, &ErrNoAudioData{}
	}

	// 1. 最初のWAVからフォーマット情報を抽出
	formatHeader, audioData, err := extractAudioData(wavDataList[0], 0)
	if err != nil {
		return nil, fmt.Errorf("最初のWAVファイルの解析に失敗しました: %w", err)
	}

	// 2. すべてのオーディオデータをスライスに保持（メモリ再確保を防止）
	extractedAudio := make([][]byte, len(wavDataList))
	extractedAudio[0] = audioData
	totalAudioSize := len(audioData)

	for i := 1; i < len(wavDataList); i++ {
		_, currentAudioData, err := extractAudioData(wavDataList[i], i)
		if err != nil {
			return nil, fmt.Errorf("WAVファイル #%d の解析に失敗しました: %w", i, err)
		}
		extractedAudio[i] = currentAudioData
		totalAudioSize += len(currentAudioData)
	}

	// 3. 結合されたデータと最初のフォーマットヘッダーから新しいWAVファイルを構築
	combinedWavBytes, err := buildCombinedWav(formatHeader, extractedAudio, totalAudioSize)
	if err != nil {
		return nil, fmt.Errorf("最終的なWAVファイルの構築に失敗しました: %w", err)
	}

	return combinedWavBytes, nil
}

// extractAudioData は WAV ファイルからフォーマットヘッダー情報と音声データ部分を抽出します。
// fmt および data チャンクを動的に探索し、data チャンクの直前までを formatHeader とします。
func extractAudioData(wavBytes []byte, index int) (formatHeader []byte, audioData []byte, err error) {
	if len(wavBytes) < wavRiffHeaderSize {
		return nil, nil, &ErrInvalidWAVHeader{
			Index:   index,
			Details: fmt.Sprintf("WAVファイルサイズが短すぎます (RIFFヘッダー不足: %dバイト)", len(wavBytes)),
		}
	}
	if string(wavBytes[0:riffChunkIDSize]) != "RIFF" || string(wavBytes[riffChunkIDSize+riffChunkSizeSize:wavRiffHeaderSize]) != "WAVE" {
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
