package wav

import (
	"encoding/binary"
	"fmt"
	"io"
	"strings"
)

// Size constants used to parse the RIFF structure.
const (
	// chunkIDSize is the size in bytes of a chunk ID ("RIFF", "fmt ", "data" and so on).
	chunkIDSize = 4
	// chunkSizeSize is the size in bytes of a chunk size field.
	chunkSizeSize = 4
	// waveIDSize is the size in bytes of the "WAVE" identifier.
	waveIDSize = 4
)

// Composite size constants used in WAV header arithmetic.
const (
	// standardHeaderSize is the header size of a WAV with no extra chunks (44 bytes).
	standardHeaderSize = 44
	// chunkHeaderSize is the total size of a chunk header, ID plus size (8 bytes).
	chunkHeaderSize = chunkIDSize + chunkSizeSize
	// minFormatChunkSize is the payload size of a PCM "fmt " chunk (16 bytes).
	minFormatChunkSize = 16
	// extensibleFormatChunkSize is the payload size of a WAVE_FORMAT_EXTENSIBLE
	// "fmt " chunk (40 bytes).
	extensibleFormatChunkSize = 40
	// wavRiffHeaderSize is the total size of the RIFF header (12 bytes).
	wavRiffHeaderSize = chunkIDSize + chunkSizeSize + waveIDSize
)

// riffChunkSizeOffset is the offset of the RIFF chunk size rewritten when combining (byte 4).
const riffChunkSizeOffset = chunkIDSize

// extensibleFormatTag is the AudioFormat value for WAVE_FORMAT_EXTENSIBLE.
// Only with this value does the actual encoding live in the SubFormat GUID at the end of
// the fmt chunk.
const extensibleFormatTag = 0xFFFE

// wavParts are the pieces of one WAV that combining needs.
type wavParts struct {
	// formatHeader is the header up to just before the data chunk, carried into the
	// output as is.
	formatHeader []byte
	format       Format
	audioData    []byte
}

// wavLayout is what scanning found out about one WAV: positions and format.
type wavLayout struct {
	format Format
	// headerSize is where the data chunk header starts, which is also the length of the
	// carried header.
	headerSize int64
	// dataOffset is where the data chunk payload starts.
	dataOffset int64
	// dataSize is the number of bytes in the data chunk payload.
	dataSize int64
}

// chunkSource is a WAV to scan, held either as an in-memory byte slice or as an
// io.ReadSeeker.
//
// Bringing both into this shape puts chunk scanning and validation in the single scanWAV,
// so CombineWavData and CombineTo cannot disagree on which inputs they accept or which
// errors they return.
//
// It is one struct rather than an interface because of allocations. An interface method
// call makes its arguments escape to the heap, so every scan would allocate a read
// buffer, and Inspect would lose the property of running without allocating.
type chunkSource struct {
	// data is the in-memory input. It is used when stream is nil.
	data   []byte
	stream io.ReadSeeker
	// index is the file position included in error messages.
	index int
	// total is the size of the whole WAV in bytes.
	total int64
	// buf is used for fixed-length reads from stream; the fmt chunk payload is the largest.
	// Passing it to io.ReadFull makes it escape to the heap, so rather than embedding an
	// array in the struct it is allocated only for streams. That keeps scanning a byte
	// slice allocation-free.
	buf []byte
}

// bytesSource returns a chunkSource over an in-memory byte slice.
func bytesSource(data []byte, index int) chunkSource {
	return chunkSource{data: data, index: index, total: int64(len(data))}
}

// streamSource returns a chunkSource over an io.ReadSeeker.
// It seeks to the end to learn the total size of the WAV.
func streamSource(r io.ReadSeeker, index int) (chunkSource, error) {
	total, err := r.Seek(0, io.SeekEnd)
	if err != nil {
		return chunkSource{}, streamIOError(index, err)
	}
	return chunkSource{stream: r, index: index, total: total, buf: make([]byte, extensibleFormatChunkSize)}, nil
}

// readAt returns n bytes starting at offset. For a byte slice it returns a subslice, for
// a stream it returns what was read into the internal buffer, so the result is valid
// until the next readAt. The caller guarantees that offset+n does not exceed total and
// that n fits in buf.
func (s *chunkSource) readAt(offset, n int64) ([]byte, error) {
	if s.stream == nil {
		if offset < 0 || offset+n > int64(len(s.data)) {
			return nil, io.ErrUnexpectedEOF
		}
		return s.data[offset : offset+n], nil
	}
	buf := s.buf[:n]
	if err := readFull(s.stream, offset, buf, s.index); err != nil {
		return nil, err
	}
	return buf, nil
}

// readFull reads from r at offset until buf is full.
func readFull(r io.ReadSeeker, offset int64, buf []byte, index int) error {
	if _, err := r.Seek(offset, io.SeekStart); err != nil {
		return streamIOError(index, err)
	}
	if _, err := io.ReadFull(r, buf); err != nil {
		return streamIOError(index, err)
	}
	return nil
}

func streamIOError(index int, err error) error {
	return fmt.Errorf("read WAV file #%d: %w", index, err)
}

// extractAudioData extracts the format and the audio data from a WAV file.
// It searches for the fmt and data chunks dynamically and takes everything before the
// data chunk as formatHeader. index is the file position included in error messages;
// pass -1 for a check that does not belong to a list.
func extractAudioData(wavBytes []byte, index int) (wavParts, error) {
	src := bytesSource(wavBytes, index)
	layout, err := scanWAV(&src)
	if err != nil {
		return wavParts{}, err
	}
	dataStart := int(layout.dataOffset)
	return wavParts{
		formatHeader: wavBytes[:int(layout.headerSize)],
		format:       layout.format,
		audioData:    wavBytes[dataStart : dataStart+int(layout.dataSize)],
	}, nil
}

// scanWAV scans a WAV and finds its format and the position of its data chunk.
//
// It never reads the audio data itself, only the RIFF header, each chunk header and the
// fmt chunk payload. Every read is of fixed length, so the memory scanning needs does not
// depend on the size of the input.
func scanWAV(src *chunkSource) (wavLayout, error) {
	index, size := src.index, src.total
	if size < wavRiffHeaderSize {
		return wavLayout{}, &ErrInvalidWAVHeader{
			Index:   index,
			Details: fmt.Sprintf("WAV file too short (incomplete RIFF header: %d bytes)", size),
		}
	}
	riffHeader, err := src.readAt(0, wavRiffHeaderSize)
	if err != nil {
		return wavLayout{}, err
	}
	if err := validateRiffHeader(riffHeader, index); err != nil {
		return wavLayout{}, err
	}

	var (
		format        Format
		fmtChunkFound bool
	)
	for offset := int64(wavRiffHeaderSize); offset+chunkHeaderSize <= size; {
		chunkHeader, err := src.readAt(offset, chunkHeaderSize)
		if err != nil {
			return wavLayout{}, err
		}
		chunkSize := binary.LittleEndian.Uint32(chunkHeader[chunkIDSize:])
		payloadStart := offset + chunkHeaderSize

		switch string(chunkHeader[:chunkIDSize]) {
		case "fmt ":
			// 40 bytes even with the extension. Read up to the shorter of the declared size
			// and what actually remains.
			payload, err := src.readAt(payloadStart, min(int64(chunkSize), extensibleFormatChunkSize, size-payloadStart))
			if err != nil {
				return wavLayout{}, err
			}
			parsed, err := parseFormatPayload(payload, chunkSize, index)
			if err != nil {
				return wavLayout{}, err
			}
			format = parsed
			fmtChunkFound = true
		case "data":
			if !fmtChunkFound {
				return wavLayout{}, missingChunkError(index, "'fmt '")
			}
			if int64(chunkSize) > size-payloadStart {
				return wavLayout{}, &ErrInvalidWAVHeader{
					Index:   index,
					Details: "data chunk length exceeds the actual file size",
				}
			}
			return wavLayout{
				format:     format,
				headerSize: offset,
				dataOffset: payloadStart,
				dataSize:   int64(chunkSize),
			}, nil
		}

		nextOffset := nextChunkOffset(uint64(offset), chunkSize)
		if nextOffset > uint64(size) {
			break
		}
		offset = int64(nextOffset)
	}

	if fmtChunkFound {
		return wavLayout{}, missingChunkError(index, "'data'")
	}
	return wavLayout{}, missingChunkError(index, "'fmt '", "'data'")
}

// validateRiffHeader validates the RIFF/WAVE identifiers of WAV data. header is the first
// 12 bytes.
func validateRiffHeader(header []byte, index int) error {
	if string(header[:chunkIDSize]) != "RIFF" || string(header[chunkHeaderSize:wavRiffHeaderSize]) != "WAVE" {
		return &ErrInvalidWAVHeader{
			Index:   index,
			Details: "invalid RIFF/WAVE identifier",
		}
	}
	return nil
}

// parseFormatPayload reads the format from a fmt chunk payload. declaredSize is the size
// the chunk header declares, and payload is its leading part (at most
// extensibleFormatChunkSize bytes).
//
// Under WAVE_FORMAT_EXTENSIBLE (AudioFormat = 0xFFFE) neither the actual encoding nor the
// channel layout is in the first 16 bytes, so the extension (channel mask and sub-format
// GUID) is read as well. Without it, linear PCM and IEEE float would match as "both
// 0xFFFE".
func parseFormatPayload(payload []byte, declaredSize uint32, index int) (Format, error) {
	// A PCM fmt chunk is 16 bytes. The extensible one is longer, but shares the layout of
	// the first 16 bytes.
	if uint64(declaredSize) < minFormatChunkSize || len(payload) < minFormatChunkSize {
		return Format{}, &ErrInvalidWAVHeader{
			Index:   index,
			Details: fmt.Sprintf("fmt chunk too short (%d bytes, need at least %d bytes)", declaredSize, minFormatChunkSize),
		}
	}

	format := Format{
		AudioFormat:   binary.LittleEndian.Uint16(payload[0:]),
		NumChannels:   binary.LittleEndian.Uint16(payload[2:]),
		SampleRate:    binary.LittleEndian.Uint32(payload[4:]),
		BitsPerSample: binary.LittleEndian.Uint16(payload[14:]),
	}
	if format.AudioFormat != extensibleFormatTag {
		return format, format.verifySupported(index)
	}

	if uint64(declaredSize) < extensibleFormatChunkSize || len(payload) < extensibleFormatChunkSize {
		return Format{}, &ErrInvalidWAVHeader{
			Index:   index,
			Details: fmt.Sprintf("WAVE_FORMAT_EXTENSIBLE fmt chunk too short (%d bytes, need at least %d bytes)", declaredSize, extensibleFormatChunkSize),
		}
	}
	format.ValidBitsPerSample = binary.LittleEndian.Uint16(payload[18:])
	format.ChannelMask = binary.LittleEndian.Uint32(payload[20:])
	copy(format.SubFormat[:], payload[24:extensibleFormatChunkSize])
	// Valid bits are packed into the top of the container, so a value larger than the
	// container is impossible.
	if format.ValidBitsPerSample > format.BitsPerSample {
		return Format{}, &ErrInvalidWAVHeader{
			Index:   index,
			Details: fmt.Sprintf("valid bits per sample (%d) exceed bits per sample (%d)", format.ValidBitsPerSample, format.BitsPerSample),
		}
	}
	return format, format.verifySupported(index)
}

// nextChunkOffset returns the position of the next chunk, accounting for WAV chunk padding.
func nextChunkOffset(offset uint64, chunkSize uint32) uint64 {
	nextOffset := offset + uint64(chunkHeaderSize) + uint64(chunkSize)
	if chunkSize%2 != 0 {
		nextOffset++
	}
	return nextOffset
}

// missingChunkError builds the error naming the required chunks that are missing.
func missingChunkError(index int, missing ...string) error {
	return &ErrInvalidWAVHeader{
		Index:   index,
		Details: fmt.Sprintf("required chunk (%s) not found in WAV file", strings.Join(missing, " and ")),
	}
}
