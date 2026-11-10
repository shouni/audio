// Package wav joins WAV data at the binary level, without decoding.
//
// There are two ways to combine. CombineWavData takes byte slices and returns a byte
// slice; CombineTo passes the audio data straight from io.ReadSeeker inputs to an
// io.Writer. Their validation and output are identical; only their memory use differs.
// Inspect runs the same validation as combining, without combining, and returns the
// format and duration.
package wav

import (
	"fmt"
	"time"
)

// CombineOption is a function that adjusts how combining behaves.
type CombineOption func(*combineConfig)

// combineConfig is the combining configuration.
type combineConfig struct {
	gap time.Duration
}

// WithGap returns an Option that sets the length of the silence inserted between one WAV
// and the next.
//
// Audio synthesized sentence by sentence sounds crammed when joined back to back, with no
// break between them; use this to leave a pause. The silence is truncated to a sample
// boundary (the block align), so the channel order never shifts.
//
// No silence is inserted when gap is 0 or less, or shorter than one sample.
func WithGap(gap time.Duration) CombineOption {
	return func(c *combineConfig) {
		c.gap = gap
	}
}

// newCombineConfig returns the configuration with the options applied in order.
func newCombineConfig(opts []CombineOption) combineConfig {
	var cfg combineConfig
	for _, opt := range opts {
		opt(&cfg)
	}
	return cfg
}

// silence returns the bytes of silence inserted between one WAV and the next, or nil when
// no silence is needed.
func (c combineConfig) silence(format Format) []byte {
	if c.gap <= 0 {
		return nil
	}
	byteRate := format.byteRate()
	blockAlign := uint64(format.blockAlign())
	if byteRate == 0 || blockAlign == 0 {
		return nil
	}

	size := mulDiv(uint64(c.gap), byteRate, uint64(time.Second))
	size -= size % blockAlign
	if size == 0 {
		return nil
	}

	buf := make([]byte, size)
	if fill := format.silenceByte(); fill != 0 {
		for i := range buf {
			buf[i] = fill
		}
	}
	return buf
}

// CombineWavData combines several WAVs into a single WAV.
// To keep memory use down, it holds the audio data to combine as slices and copies them
// only once, when building the final buffer.
//
// The output header reuses the first file's fmt chunk as is, so files whose formats do
// not match return *ErrMismatchedWAVFormat. Compressed formats such as ADPCM are not
// guaranteed to form a valid WAV when joined, so they return *ErrUnsupportedWAVFormat.
// When the data length of any input but the last is not a multiple of the block size (one
// sample across all channels), joining it as is would shift every following sample, so
// that returns *ErrInvalidWAVHeader as well.
//
// Every input is held in memory, so CombineTo suits long combinations better.
func CombineWavData(wavDataList [][]byte, opts ...CombineOption) ([]byte, error) {
	if len(wavDataList) == 0 {
		return nil, &ErrNoAudioData{}
	}
	cfg := newCombineConfig(opts)

	// extractAudioData's errors already carry the file position (#N), so return them as is.
	first, err := extractAudioData(wavDataList[0], 0)
	if err != nil {
		return nil, err
	}

	// The silence is the same in every gap, so allocate one and reuse it.
	silence := cfg.silence(first.format)

	extractedAudio := make([][]byte, 0, len(wavDataList)*2-1)
	extractedAudio = append(extractedAudio, first.audioData)
	totalAudioSize := uint64(len(first.audioData))

	previous := first
	for i := 1; i < len(wavDataList); i++ {
		current, err := extractAudioData(wavDataList[i], i)
		if err != nil {
			return nil, err
		}
		// The output header is the first file's, so a different format would make the
		// second and later files play back as something they are not.
		if err := verifySameFormat(first.format, current.format, i); err != nil {
			return nil, err
		}
		if err := verifyBlockAligned(previous.format, int64(len(previous.audioData)), i-1); err != nil {
			return nil, err
		}
		if len(silence) > 0 {
			extractedAudio = append(extractedAudio, silence)
			totalAudioSize += uint64(len(silence))
		}
		extractedAudio = append(extractedAudio, current.audioData)
		totalAudioSize += uint64(len(current.audioData))
		previous = current
	}

	return buildCombinedWav(first, extractedAudio, totalAudioSize)
}

// verifyBlockAligned checks that the data length of a WAV followed by another input is a
// multiple of the block size (one sample across all channels).
//
// Combining concatenates data without decoding, so if an intermediate input ends partway
// through a sample, every following sample is read shifted by that much, and the result
// is a roar rather than silence. The last input is exempt because nothing follows it (a
// decoder discards a truncated sample at the end). Formats whose blockAlign is 0 (under
// 8 bits) cannot be checked and are let through.
func verifyBlockAligned(format Format, dataSize int64, index int) error {
	blockAlign := int64(format.blockAlign())
	if blockAlign == 0 || dataSize%blockAlign == 0 {
		return nil
	}
	return &ErrInvalidWAVHeader{
		Index: index,
		Details: fmt.Sprintf("data chunk length (%d bytes) is not a multiple of the block size (%d bytes); following samples would shift, so it cannot be combined",
			dataSize, blockAlign),
	}
}

// verifySameFormat checks that a format to combine matches the first file's.
func verifySameFormat(first, current Format, index int) error {
	switch {
	case first.AudioFormat != current.AudioFormat:
		return mismatchedFormat(index, "format tag", uint32(first.AudioFormat), uint32(current.AudioFormat))
	case first.NumChannels != current.NumChannels:
		return mismatchedFormat(index, "channel count", uint32(first.NumChannels), uint32(current.NumChannels))
	case first.SampleRate != current.SampleRate:
		return mismatchedFormat(index, "sample rate", first.SampleRate, current.SampleRate)
	case first.BitsPerSample != current.BitsPerSample:
		return mismatchedFormat(index, "bits per sample", uint32(first.BitsPerSample), uint32(current.BitsPerSample))
	case first.ValidBitsPerSample != current.ValidBitsPerSample:
		// With the same container size the bytes do not shift, but the combined header
		// claims the first file's value, so some software reads the second and later
		// files with their low bits discarded.
		return mismatchedFormat(index, "valid bits per sample", uint32(first.ValidBitsPerSample), uint32(current.ValidBitsPerSample))
	case first.ChannelMask != current.ChannelMask:
		return mismatchedFormat(index, "channel mask", first.ChannelMask, current.ChannelMask)
	case first.SubFormat != current.SubFormat:
		return &ErrMismatchedSubFormat{Index: index, First: first.SubFormat, Got: current.SubFormat}
	}
	return nil
}

func mismatchedFormat(index int, field string, first, got uint32) error {
	return &ErrMismatchedWAVFormat{Index: index, Field: field, First: first, Got: got}
}
