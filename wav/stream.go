package wav

import (
	"fmt"
	"io"
)

// maxCarriedHeaderSize caps the header carried over from the first file (everything
// before the data chunk). Streaming combination exists to decouple memory use from input
// size, so this stops a huge metadata chunk from being read whole and breaking that
// premise.
const maxCarriedHeaderSize = 1 << 20 // 1MiB

// CombineTo combines several WAVs and writes the result to w.
//
// Unlike CombineWavData, which holds every input in memory, it passes the audio data
// straight from the inputs to w, so the memory it needs does not depend on the audio's
// length. Per input it adds only the scan result and io.CopyN's copy buffer (about 33 KB
// when writing to an *os.File, a few hundred bytes for io.Discard). Use it for combining
// files or writing long output.
//
// sources are scanned twice: the first pass finds the format and the position of the data
// chunk, the second transfers the audio. The output's RIFF and data chunk sizes go at the
// front and are not known until the whole length has been measured, which is why the
// inputs must be io.ReadSeeker.
//
// Its validation and errors are the same as CombineWavData's.
func CombineTo(w io.Writer, sources []io.ReadSeeker, opts ...CombineOption) error {
	if len(sources) == 0 {
		return &ErrNoAudioData{}
	}
	cfg := newCombineConfig(opts)

	parts := make([]wavLayout, len(sources))
	var totalAudioSize uint64
	for i, source := range sources {
		src, err := streamSource(source, i)
		if err != nil {
			return err
		}
		part, err := scanWAV(&src)
		if err != nil {
			return err
		}
		// The output header is the first file's, so a different format would make the
		// second and later files play back as something they are not.
		if i > 0 {
			if err := verifySameFormat(parts[0].format, part.format, i); err != nil {
				return err
			}
			if err := verifyBlockAligned(parts[i-1].format, parts[i-1].dataSize, i-1); err != nil {
				return err
			}
		}
		parts[i] = part
		totalAudioSize += uint64(part.dataSize)
	}

	silence := cfg.silence(parts[0].format)
	totalAudioSize += uint64(len(silence)) * uint64(len(sources)-1)

	formatHeader, err := readCarriedHeader(sources[0], parts[0].headerSize)
	if err != nil {
		return err
	}
	header, err := buildCombinedHeader(formatHeader, parts[0].format, totalAudioSize)
	if err != nil {
		return err
	}
	if _, err := w.Write(header); err != nil {
		return fmt.Errorf("write WAV header: %w", err)
	}

	for i, source := range sources {
		if i > 0 && len(silence) > 0 {
			if _, err := w.Write(silence); err != nil {
				return fmt.Errorf("write silence: %w", err)
			}
		}
		if _, err := source.Seek(parts[i].dataOffset, io.SeekStart); err != nil {
			return streamIOError(i, err)
		}
		if _, err := io.CopyN(w, source, parts[i].dataSize); err != nil {
			return fmt.Errorf("copy audio data of WAV file #%d: %w", i, err)
		}
	}
	return nil
}

// readCarriedHeader reads from the first file the header that is carried into the output
// as is.
func readCarriedHeader(source io.ReadSeeker, headerSize int64) ([]byte, error) {
	if headerSize > maxCarriedHeaderSize {
		return nil, &ErrInvalidWAVHeader{
			Index:   0,
			Details: fmt.Sprintf("header before the data chunk is too large (%d bytes, limit %d bytes); use CombineWavData instead", headerSize, maxCarriedHeaderSize),
		}
	}
	header := make([]byte, headerSize)
	if err := readFull(source, 0, header, 0); err != nil {
		return nil, err
	}
	return header, nil
}
