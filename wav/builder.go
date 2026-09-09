package wav

import (
	"encoding/binary"
	"math"
)

// factSampleCountSize は fact チャンクのサンプル数フィールドのサイズ（バイト）です。
const factSampleCountSize = 4

// buildCombinedHeader は結合後の WAV ヘッダーを組み立てます。
//
// formatHeader は先頭ファイルの data チャンク直前までのバイト列です。RIFF チャンクサイズを
// 結合後の値へ書き換え、fact チャンクがあればそのサンプル数も書き換えて、末尾に結合後の
// data チャンクヘッダーを付け足します。fmt チャンクや LIST などの付随チャンクは
// 先頭ファイルのものがそのまま残ります。
func buildCombinedHeader(formatHeader []byte, format Format, totalAudioSize uint64) ([]byte, error) {
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
	updateFactChunk(header[:len(formatHeader)], format, totalAudioSize)
	return header, nil
}

// updateFactChunk は引き継いだヘッダーに fact チャンクがあれば、そのサンプル数を
// 結合後の値へ書き換えます。
//
// fact チャンクは data チャンクとは別にチャンネルあたりのサンプル数を持つため、
// 先頭ファイルの値のまま引き継ぐと、結合後も 1 本目の長さを指し続けます。
// サンプル数は Duration や WithGap と同じく、ブロックアライン 1 つを 1 サンプルとして
// 求めます。ブロックアラインが 0 になるフォーマットでは求められないので手を付けません。
//
// header は data チャンク直前までなので、走査が data に届くことはありません。
// サンプル数がバイト数を超えることはなく、呼び出し元で 4GB 超過を弾いた後なので
// uint32 に収まります。
func updateFactChunk(header []byte, format Format, totalAudioSize uint64) {
	blockAlign := uint64(format.blockAlign())
	if blockAlign == 0 {
		return
	}
	sampleCount := uint32(totalAudioSize / blockAlign)

	for offset := wavRiffHeaderSize; offset+chunkHeaderSize <= len(header); {
		id := string(header[offset : offset+chunkIDSize])
		size := binary.LittleEndian.Uint32(header[offset+chunkIDSize : offset+chunkHeaderSize])
		if id == "fact" {
			payloadStart := offset + chunkHeaderSize
			if size < factSampleCountSize || payloadStart+factSampleCountSize > len(header) {
				return
			}
			binary.LittleEndian.PutUint32(header[payloadStart:], sampleCount)
			return
		}
		nextOffset := nextChunkOffset(uint64(offset), size)
		if nextOffset > uint64(len(header)) {
			return
		}
		offset = int(nextOffset)
	}
}

// buildCombinedWav はオーディオパーツのスライスを一括でコピーして WAV ファイルを再構築します。
// 出力バッファの確保は一度だけで、以降はその中へ直接書き込みます。
func buildCombinedWav(first wavParts, audioParts [][]byte, totalAudioSize uint64) ([]byte, error) {
	header, err := buildCombinedHeader(first.formatHeader, first.format, totalAudioSize)
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
