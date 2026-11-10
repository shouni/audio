package wav

import (
	"encoding/binary"
	"math"
)

// factSampleCountSize is the size in bytes of the sample count field of a fact chunk.
const factSampleCountSize = 4

// buildCombinedHeader assembles the header of the combined WAV.
//
// formatHeader is the first file's bytes up to just before its data chunk. It rewrites the
// RIFF chunk size to the combined value, rewrites the sample count of a fact chunk if
// there is one, and appends the combined data chunk header. The fmt chunk and other
// accompanying chunks such as LIST stay as the first file had them.
func buildCombinedHeader(formatHeader []byte, format Format, totalAudioSize uint64) ([]byte, error) {
	headerSize := uint64(len(formatHeader)) + chunkHeaderSize
	// RIFF chunk size = (all headers + all data) - 8
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

// updateFactChunk rewrites the sample count of a fact chunk in the carried header, if
// there is one, to the combined value.
//
// A fact chunk holds the per-channel sample count separately from the data chunk, so
// carried over with the first file's value it would keep describing the length of the
// first file after combining. Like Duration and WithGap, the count treats one block align
// as one sample. A format whose block align is 0 has no count to compute, so the chunk is
// left alone.
//
// header ends just before the data chunk, so the scan never reaches data. The sample
// count never exceeds the byte count, and the caller has already rejected anything over
// 4GB, so it fits in a uint32.
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

// buildCombinedWav rebuilds a WAV file by copying a slice of audio parts in one pass.
// The output buffer is allocated once and everything is written straight into it.
func buildCombinedWav(first wavParts, audioParts [][]byte, totalAudioSize uint64) ([]byte, error) {
	header, err := buildCombinedHeader(first.formatHeader, first.format, totalAudioSize)
	if err != nil {
		return nil, err
	}
	// On 32-bit platforms anything over 2GB does not fit in an int. Reject it before make.
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
