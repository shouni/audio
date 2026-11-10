package wav

import (
	"fmt"
	"math"
)

// ErrNoAudioData is returned when there is no audio data to combine.
type ErrNoAudioData struct{}

func (e *ErrNoAudioData) Error() string {
	return "no audio data to combine"
}

// ErrInvalidWAVHeader is returned when a WAV header fails validation.
type ErrInvalidWAVHeader struct {
	// Index is the position of the WAV file the error occurred in.
	// It is -1 for a check that does not belong to a list, such as Inspect.
	Index int
	// Details describes the error.
	Details string
}

func (e *ErrInvalidWAVHeader) Error() string {
	if e.Index >= 0 {
		return fmt.Sprintf("invalid header in WAV file #%d: %s", e.Index, e.Details)
	}
	return fmt.Sprintf("invalid WAV header: %s", e.Details)
}

// ErrMismatchedWAVFormat is returned when a WAV to combine does not match the format of
// the first file.
//
// Combining concatenates bytes without decoding, so joining a different format plays the
// second and later files back as the first file's format, at the wrong speed and pitch.
//
// When only the WAVE_FORMAT_EXTENSIBLE sub-format GUID differs, the value cannot be
// expressed as a number, so the error is ErrMismatchedSubFormat instead.
type ErrMismatchedWAVFormat struct {
	// Index is the position of the WAV file whose format differs.
	Index int
	// Field is the name of the field that differs (for example "sample rate").
	Field string
	// First is the first file's value, Got is the value in the file at Index.
	First uint32
	Got   uint32
}

func (e *ErrMismatchedWAVFormat) Error() string {
	return fmt.Sprintf("WAV file #%d differs from the first file in %s (%d ≠ %d); combining without decoding requires matching formats",
		e.Index, e.Field, e.Got, e.First)
}

// ErrMismatchedSubFormat is returned when a WAVE_FORMAT_EXTENSIBLE sub-format GUID does
// not match the first file.
//
// In the extensible format the fmt chunk's AudioFormat is always 0xFFFE and the actual
// encoding lives in the GUID. Without looking at the GUID, linear PCM and IEEE float
// would be joined as "both 0xFFFE".
type ErrMismatchedSubFormat struct {
	// Index is the position of the WAV file whose sub-format differs.
	Index int
	// First is the first file's GUID, Got is the GUID of the file at Index.
	First [16]byte
	Got   [16]byte
}

func (e *ErrMismatchedSubFormat) Error() string {
	return fmt.Sprintf("sub-format of WAV file #%d does not match the first file (%s ≠ %s); combining without decoding requires matching formats",
		e.Index, formatGUID(e.Got), formatGUID(e.First))
}

// ErrUnsupportedWAVFormat is returned when a WAV is in a format that cannot be joined
// without decoding.
//
// Supported are linear PCM (bit depths that are multiples of 8), IEEE float (32/64 bit),
// G.711 A-law / μ-law (8 bit), and the WAVE_FORMAT_EXTENSIBLE form of each.
// Convert compressed formats, such as ADPCM or a WAV carrying MP3, to PCM first.
type ErrUnsupportedWAVFormat struct {
	// Index is the position of the unsupported WAV file.
	// It is -1 for a check that does not belong to a list, such as Inspect.
	Index int
	// Format is the format that was read.
	Format Format
}

func (e *ErrUnsupportedWAVFormat) Error() string {
	encoding := fmt.Sprintf("encoding %#04x", e.Format.AudioFormat)
	if e.Format.AudioFormat == extensibleFormatTag {
		encoding = "sub-format " + formatGUID(e.Format.SubFormat)
	}
	target := "WAV file"
	if e.Index >= 0 {
		target = fmt.Sprintf("WAV file #%d", e.Index)
	}
	return fmt.Sprintf("%s is in an unsupported format (%s, %d bits); combining without decoding supports only linear PCM, IEEE float, A-law and μ-law, so convert it to PCM first",
		target, encoding, e.Format.BitsPerSample)
}

// ErrWAVTooLarge is returned when the combined result exceeds what RIFF can express (4GB).
//
// The RIFF chunk size and the data chunk size are both 32-bit, so a larger file cannot
// record its sizes correctly. Split it and combine the parts separately.
type ErrWAVTooLarge struct {
	// Size is the RIFF chunk size that would have been written (whole file minus 8 bytes).
	Size uint64
}

func (e *ErrWAVTooLarge) Error() string {
	return fmt.Sprintf("combined WAV file exceeds 4GB (%d bytes, limit %d bytes)", e.Size, uint64(math.MaxUint32))
}
