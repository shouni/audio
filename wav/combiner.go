package wav

import (
	"fmt"
)

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
