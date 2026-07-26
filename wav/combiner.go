// Package wav は、WAVデータをデコードなしでバイナリレベルで結合するユーティリティを提供します。
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
	first, err := extractAudioData(wavDataList[0], 0)
	if err != nil {
		return nil, fmt.Errorf("最初のWAVファイルの解析に失敗しました: %w", err)
	}

	// 2. すべてのオーディオデータをスライスに保持（メモリ再確保を防止）
	extractedAudio := make([][]byte, len(wavDataList))
	extractedAudio[0] = first.audioData
	totalAudioSize := len(first.audioData)

	for i := 1; i < len(wavDataList); i++ {
		current, err := extractAudioData(wavDataList[i], i)
		if err != nil {
			return nil, fmt.Errorf("WAVファイル #%d の解析に失敗しました: %w", i, err)
		}
		// 出力ヘッダーは先頭ファイルのものを使うため、フォーマットが違うと
		// 2本目以降が別フォーマットとして再生されてしまう。
		if err := verifySameFormat(first.format, current.format, i); err != nil {
			return nil, err
		}
		extractedAudio[i] = current.audioData
		totalAudioSize += len(current.audioData)
	}

	// 3. 結合されたデータと最初のフォーマットヘッダーから新しいWAVファイルを構築
	combinedWavBytes, err := buildCombinedWav(first.formatHeader, extractedAudio, totalAudioSize)
	if err != nil {
		return nil, fmt.Errorf("最終的なWAVファイルの構築に失敗しました: %w", err)
	}

	return combinedWavBytes, nil
}

// verifySameFormat は、結合対象のフォーマットが先頭ファイルと一致することを確認します。
func verifySameFormat(first, current wavFormat, index int) error {
	fields := []struct {
		name         string
		first, other uint32
	}{
		{"フォーマット種別", uint32(first.audioFormat), uint32(current.audioFormat)},
		{"チャンネル数", uint32(first.numChannels), uint32(current.numChannels)},
		{"サンプルレート", first.sampleRate, current.sampleRate},
		{"量子化ビット数", uint32(first.bitsPerSample), uint32(current.bitsPerSample)},
	}

	for _, f := range fields {
		if f.first != f.other {
			return &ErrMismatchedWAVFormat{
				Index: index,
				Field: f.name,
				First: f.first,
				Got:   f.other,
			}
		}
	}
	return nil
}
