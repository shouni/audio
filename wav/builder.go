package wav

import (
	"encoding/binary"
	"math"
)

// buildCombinedHeader は結合後の WAV ヘッダーを組み立てます。
//
// formatHeader は先頭ファイルの data チャンク直前までのバイト列です。RIFF チャンクサイズ
// だけを結合後の値へ書き換え、末尾に結合後の data チャンクヘッダーを付け足します。
// fmt チャンクや LIST などの付随チャンクは先頭ファイルのものがそのまま残ります。
func buildCombinedHeader(formatHeader []byte, totalAudioSize uint64) ([]byte, error) {
	headerSize := uint64(len(formatHeader)) + chunkHeaderSize
	// RIFFチャンクサイズ = (全ヘッダー + 全データ) - 8
	riffChunkSize := headerSize + totalAudioSize - chunkHeaderSize
	if riffChunkSize > math.MaxUint32 {
		return nil, &ErrWAVTooLarge{Size: riffChunkSize}
	}

	header := make([]byte, headerSize)
	copy(header, formatHeader)
	copy(header[len(formatHeader):], "data")
	binary.LittleEndian.PutUint32(header[riffChunkSizeOffset:], uint32(riffChunkSize))
	binary.LittleEndian.PutUint32(header[len(formatHeader)+chunkIDSize:], uint32(totalAudioSize))
	return header, nil
}

// buildCombinedWav はオーディオパーツのスライスを一括でコピーして WAV ファイルを再構築します。
// 出力バッファの確保は一度だけで、以降はその中へ直接書き込みます。
func buildCombinedWav(formatHeader []byte, audioParts [][]byte, totalAudioSize uint64) ([]byte, error) {
	header, err := buildCombinedHeader(formatHeader, totalAudioSize)
	if err != nil {
		return nil, err
	}
	// 32bit 環境では 2GB を超えた時点で int に収まらない。make に渡す前に弾く。
	if totalAudioSize > uint64(math.MaxInt)-uint64(len(header)) {
		return nil, &ErrWAVTooLarge{Size: totalAudioSize + uint64(len(header)) - chunkHeaderSize}
	}

	combinedWav := make([]byte, len(header)+int(totalAudioSize))
	offset := copy(combinedWav, header)
	for _, part := range audioParts {
		offset += copy(combinedWav[offset:], part)
	}
	return combinedWav, nil
}
