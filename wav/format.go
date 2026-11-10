package wav

import (
	"encoding/binary"
	"fmt"
)

// Format is the playback format read from a WAV fmt chunk.
type Format struct {
	// AudioFormat is the encoding of the audio data (1 = linear PCM, 3 = IEEE float,
	// 0xFFFE = WAVE_FORMAT_EXTENSIBLE).
	AudioFormat uint16
	// NumChannels is the number of channels (1 = mono, 2 = stereo).
	NumChannels uint16
	// SampleRate is the number of samples per second (Hz).
	SampleRate uint32
	// BitsPerSample is the number of bits per sample.
	// In the extensible format it is the size of the container holding one sample.
	BitsPerSample uint16
	// ValidBitsPerSample is how many of BitsPerSample actually carry signal
	// (for example 24 bits of audio in a 32-bit container).
	// It is set only for WAVE_FORMAT_EXTENSIBLE and is 0 for every other format.
	ValidBitsPerSample uint16
	// ChannelMask is the bit mask assigning each channel to a speaker position.
	// It is set only for WAVE_FORMAT_EXTENSIBLE and is 0 for every other format.
	ChannelMask uint32
	// SubFormat is the GUID naming the actual encoding under WAVE_FORMAT_EXTENSIBLE.
	// It is the zero value for every other format.
	SubFormat [16]byte
}

// AudioFormatTag returns the actual encoding.
//
// Under WAVE_FORMAT_EXTENSIBLE, AudioFormat is always 0xFFFE and the encoding sits in
// the first 2 bytes of the SubFormat GUID. AudioFormatTag hides that difference so the
// extensible format compares with the same values as the plain one (1 = linear PCM,
// 3 = IEEE float).
func (f Format) AudioFormatTag() uint16 {
	if f.AudioFormat != extensibleFormatTag {
		return f.AudioFormat
	}
	return binary.LittleEndian.Uint16(f.SubFormat[0:2])
}

// mulDiv computes value×numerator÷denominator by splitting value into its quotient and
// remainder by denominator, because multiplying first overflows uint64 for long durations
// and high rates. The quotient is an integer, so dividing only the remainder part still
// gives the same result as truncating division.
func mulDiv(value, numerator, denominator uint64) uint64 {
	return value/denominator*numerator + value%denominator*numerator/denominator
}

// byteRate returns the number of bytes per second.
func (f Format) byteRate() uint64 {
	return uint64(f.SampleRate) * uint64(f.blockAlign())
}

// blockAlign returns the size in bytes of one sample across all channels.
// Silence has to be cut in this unit, or the channel order shifts.
func (f Format) blockAlign() uint32 {
	return uint32(f.NumChannels) * uint32(f.BitsPerSample) / 8
}

// silenceByte returns the byte value that represents silence.
//
// Which byte means "amplitude zero" depends on the encoding. Only signed formats may be
// filled with 0x00; writing 0x00 into any other format produces noise, not silence.
//
//   - 8-bit linear PCM is unsigned (silence is the midpoint of 0-255), so 0x80.
//   - μ-law stores the sign bit and magnitude inverted, so 0xFF.
//   - A-law inverts the even bits (XOR with 0x55), so 0xD5.
//   - Integer PCM of 16 bits and above and IEEE float are signed, so 0x00.
func (f Format) silenceByte() byte {
	switch f.AudioFormatTag() {
	case pcmFormatTag:
		if f.BitsPerSample == 8 {
			return 0x80
		}
	case muLawFormatTag:
		return 0xFF
	case aLawFormatTag:
		return 0xD5
	}
	return 0x00
}

// AudioFormat values naming an encoding (WAVE_FORMAT_*).
const (
	// pcmFormatTag is linear PCM.
	pcmFormatTag = 1
	// floatFormatTag is IEEE float.
	floatFormatTag = 3
	// aLawFormatTag is G.711 A-law.
	aLawFormatTag = 6
	// muLawFormatTag is G.711 μ-law.
	muLawFormatTag = 7
)

// subFormatGUIDSuffix is the trailing 12 bytes shared by every standard sub-format GUID
// (KSDATAFORMAT_SUBTYPE_*). Of the first 4 bytes, the low 2 are the encoding and the high
// 2 are 0.
var subFormatGUIDSuffix = [12]byte{0x00, 0x00, 0x10, 0x00, 0x80, 0x00, 0x00, 0xAA, 0x00, 0x38, 0x9B, 0x71}

// verifySupported checks that the format can be joined without decoding.
//
// Supported are formats where the size of one sample follows from the bit depth and
// channel count alone and the silence byte is known (linear PCM, IEEE float, G.711
// A-law / μ-law). For compressed formats, such as ADPCM or a WAV carrying MP3, neither
// the block boundaries nor silence follow from the bit depth, so there is no guarantee
// that joining them yields a valid WAV.
func (f Format) verifySupported(index int) error {
	if f.AudioFormat == extensibleFormatTag &&
		(binary.LittleEndian.Uint16(f.SubFormat[2:4]) != 0 || [12]byte(f.SubFormat[4:]) != subFormatGUIDSuffix) {
		return &ErrUnsupportedWAVFormat{Index: index, Format: f}
	}

	var supported bool
	switch f.AudioFormatTag() {
	case pcmFormatTag:
		supported = f.BitsPerSample > 0 && f.BitsPerSample%8 == 0
	case floatFormatTag:
		supported = f.BitsPerSample == 32 || f.BitsPerSample == 64
	case aLawFormatTag, muLawFormatTag:
		supported = f.BitsPerSample == 8
	}
	if !supported {
		return &ErrUnsupportedWAVFormat{Index: index, Format: f}
	}
	return nil
}

// formatGUID renders a 16-byte sub-format GUID in its canonical string form.
// The first 3 groups are little-endian, the last 2 keep their byte order.
func formatGUID(guid [16]byte) string {
	return fmt.Sprintf("{%08X-%04X-%04X-%04X-%012X}",
		binary.LittleEndian.Uint32(guid[0:4]),
		binary.LittleEndian.Uint16(guid[4:6]),
		binary.LittleEndian.Uint16(guid[6:8]),
		binary.BigEndian.Uint16(guid[8:10]),
		guid[10:16])
}
