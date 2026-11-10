package wav

import "time"

// Info summarizes the format and audio data read from a WAV binary.
type Info struct {
	// Format is the content of the fmt chunk.
	Format Format
	// DataSize is the number of bytes in the data chunk payload.
	DataSize int
}

// Duration returns the playback duration of the audio data.
// It returns 0 when the format values do not yield a byte rate (when it would be 0).
func (i Info) Duration() time.Duration {
	rate := i.Format.byteRate()
	if rate == 0 || i.DataSize <= 0 {
		return 0
	}
	// Compute in integers. Going through floating point can add a 1ns error even for a
	// length that falls exactly on a sample boundary.
	return time.Duration(mulDiv(uint64(i.DataSize), uint64(time.Second), rate))
}

// Inspect parses a WAV binary and returns a summary of its format and audio data.
// It runs the same validation as CombineWavData (RIFF/WAVE identifiers, presence and size
// of the fmt/data chunks), so it can serve as a pre-check before combining or to estimate
// a duration. Formats that cannot be joined without decoding, such as ADPCM, return
// *ErrUnsupportedWAVFormat.
func Inspect(wavBytes []byte) (Info, error) {
	parts, err := extractAudioData(wavBytes, -1)
	if err != nil {
		return Info{}, err
	}
	return Info{Format: parts.format, DataSize: len(parts.audioData)}, nil
}
