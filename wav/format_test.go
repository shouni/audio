package wav

import (
	"encoding/binary"
	"errors"
	"testing"
)

// KSDATAFORMAT_SUBTYPE_* are the WAVE_FORMAT_EXTENSIBLE sub-format GUIDs.
// The first 2 bytes are the actual encoding; the rest is a fixed value shared by every
// standard subtype.
var (
	subFormatPCM   = subFormatGUID(pcmFormatTag)
	subFormatFloat = subFormatGUID(floatFormatTag)
)

func subFormatGUID(tag uint16) [16]byte {
	var guid [16]byte
	binary.LittleEndian.PutUint16(guid[0:], tag)
	copy(guid[4:], []byte{0x00, 0x00, 0x10, 0x00, 0x80, 0x00, 0x00, 0xAA, 0x00, 0x38, 0x9B, 0x71})
	return guid
}

// extensibleSpec holds the values for assembling a WAVE_FORMAT_EXTENSIBLE test WAV.
type extensibleSpec struct {
	numChannels   uint16
	sampleRate    uint32
	bitsPerSample uint16
	validBits     uint16 // bitsPerSample when 0
	channelMask   uint32
	subFormat     [16]byte
	fmtChunkSize  uint32 // extensibleFormatChunkSize when 0
	audio         []byte
}

func defaultExtensibleSpec(audio []byte) extensibleSpec {
	return extensibleSpec{
		numChannels:   2,
		sampleRate:    48000,
		bitsPerSample: 16,
		channelMask:   0x3, // FL | FR
		subFormat:     subFormatPCM,
		audio:         audio,
	}
}

// buildExtensibleWAV assembles an extensible-format WAV with a 40-byte fmt chunk.
func buildExtensibleWAV(spec extensibleSpec) []byte {
	blockAlign := spec.numChannels * spec.bitsPerSample / 8

	payload := make([]byte, extensibleFormatChunkSize)
	binary.LittleEndian.PutUint16(payload[0:], extensibleFormatTag)
	binary.LittleEndian.PutUint16(payload[2:], spec.numChannels)
	binary.LittleEndian.PutUint32(payload[4:], spec.sampleRate)
	binary.LittleEndian.PutUint32(payload[8:], spec.sampleRate*uint32(blockAlign))
	binary.LittleEndian.PutUint16(payload[12:], blockAlign)
	binary.LittleEndian.PutUint16(payload[14:], spec.bitsPerSample)
	binary.LittleEndian.PutUint16(payload[16:], 22) // cbSize
	validBits := spec.validBits
	if validBits == 0 {
		validBits = spec.bitsPerSample
	}
	binary.LittleEndian.PutUint16(payload[18:], validBits)
	binary.LittleEndian.PutUint32(payload[20:], spec.channelMask)
	copy(payload[24:], spec.subFormat[:])

	declaredSize := spec.fmtChunkSize
	if declaredSize == 0 {
		declaredSize = extensibleFormatChunkSize
	}

	out := make([]byte, 0, wavRiffHeaderSize+chunkHeaderSize+len(payload)+chunkHeaderSize+len(spec.audio))
	out = append(out, "RIFF"...)
	out = append(out, 0, 0, 0, 0)
	out = append(out, "WAVE"...)
	out = append(out, "fmt "...)
	out = binary.LittleEndian.AppendUint32(out, declaredSize)
	out = append(out, payload...)
	out = append(out, "data"...)
	out = binary.LittleEndian.AppendUint32(out, uint32(len(spec.audio)))
	out = append(out, spec.audio...)
	binary.LittleEndian.PutUint32(out[riffChunkSizeOffset:], uint32(len(out)-chunkHeaderSize))
	return out
}

func TestInspectReadsExtensibleFormat(t *testing.T) {
	info, err := Inspect(buildExtensibleWAV(defaultExtensibleSpec([]byte{1, 2, 3, 4})))
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}

	if info.Format.AudioFormat != extensibleFormatTag {
		t.Errorf("AudioFormat = %#x, want %#x", info.Format.AudioFormat, extensibleFormatTag)
	}
	if info.Format.ChannelMask != 0x3 {
		t.Errorf("ChannelMask = %#x, want 0x3", info.Format.ChannelMask)
	}
	if info.Format.SubFormat != subFormatPCM {
		t.Errorf("SubFormat = %s, want %s", formatGUID(info.Format.SubFormat), formatGUID(subFormatPCM))
	}
	if got := info.Format.AudioFormatTag(); got != pcmFormatTag {
		t.Errorf("AudioFormatTag() = %d, want %d", got, pcmFormatTag)
	}
}

// TestCombineWavDataRejectsMismatchedSubFormat checks that extensible-format WAVs with
// different sub-formats are not combined.
//
// In the extensible format AudioFormat is always 0xFFFE, so without looking at the GUID,
// linear PCM and IEEE float would match as "both 0xFFFE" and come out as silence or
// noise.
func TestCombineWavDataRejectsMismatchedSubFormat(t *testing.T) {
	// Only float of 32 bits and above is supported, so both use 32 bits.
	first := defaultExtensibleSpec([]byte{1, 2, 3, 4, 5, 6, 7, 8})
	first.bitsPerSample = 32
	second := first
	second.subFormat = subFormatFloat

	_, err := CombineWavData([][]byte{
		buildExtensibleWAV(first),
		buildExtensibleWAV(second),
	})
	if err == nil {
		t.Fatal("CombineWavData() error = nil, want ErrMismatchedSubFormat")
	}
	mismatch, ok := errors.AsType[*ErrMismatchedSubFormat](err)
	if !ok {
		t.Fatalf("error type = %T, want *ErrMismatchedSubFormat", err)
	}
	if mismatch.Index != 1 {
		t.Errorf("Index = %d, want 1", mismatch.Index)
	}
	if mismatch.First != subFormatPCM || mismatch.Got != subFormatFloat {
		t.Errorf("First/Got = %s/%s, want %s/%s",
			formatGUID(mismatch.First), formatGUID(mismatch.Got),
			formatGUID(subFormatPCM), formatGUID(subFormatFloat))
	}
}

// TestCombineWavDataRejectsMismatchedChannelMask checks that WAVs with the same channel
// count but different speaker positions are not combined. Joining FL/FR with FC/LFE as
// 2ch would assign the second and later files to different speakers.
func TestCombineWavDataRejectsMismatchedChannelMask(t *testing.T) {
	second := defaultExtensibleSpec([]byte{5, 6, 7, 8})
	second.channelMask = 0xC // FC | LFE

	_, err := CombineWavData([][]byte{
		buildExtensibleWAV(defaultExtensibleSpec([]byte{1, 2, 3, 4})),
		buildExtensibleWAV(second),
	})
	mismatch, ok := errors.AsType[*ErrMismatchedWAVFormat](err)
	if !ok {
		t.Fatalf("error type = %T, want *ErrMismatchedWAVFormat", err)
	}
	if mismatch.Field != "channel mask" {
		t.Errorf("Field = %q, want channel mask", mismatch.Field)
	}
}

func TestCombineWavDataAcceptsIdenticalExtensibleFormat(t *testing.T) {
	combined, err := CombineWavData([][]byte{
		buildExtensibleWAV(defaultExtensibleSpec([]byte{1, 2, 3, 4})),
		buildExtensibleWAV(defaultExtensibleSpec([]byte{5, 6, 7, 8})),
	})
	if err != nil {
		t.Fatalf("CombineWavData() error = %v", err)
	}
	info, err := Inspect(combined)
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	if info.DataSize != 8 {
		t.Errorf("DataSize = %d, want 8", info.DataSize)
	}
	if info.Format.SubFormat != subFormatPCM {
		t.Errorf("SubFormat = %s, want %s", formatGUID(info.Format.SubFormat), formatGUID(subFormatPCM))
	}
}

// TestParseFormatChunkRejectsTruncatedExtensible checks that a fmt chunk claiming 0xFFFE
// without the extension is rejected. Skipping over it would leave SubFormat at its zero
// value and make genuinely different files match.
func TestParseFormatChunkRejectsTruncatedExtensible(t *testing.T) {
	spec := defaultExtensibleSpec([]byte{1, 2})
	spec.fmtChunkSize = minFormatChunkSize

	_, err := Inspect(buildExtensibleWAV(spec))
	if _, ok := errors.AsType[*ErrInvalidWAVHeader](err); !ok {
		t.Fatalf("error type = %T, want *ErrInvalidWAVHeader", err)
	}
}

// TestCombineWavDataRejectsMismatchedValidBits checks that WAVs with the same container
// size but different valid bits are not combined. The bytes do not shift, but the combined
// header claims the first file's value, so some software reads the second and later files
// with their low bits discarded.
func TestCombineWavDataRejectsMismatchedValidBits(t *testing.T) {
	first := defaultExtensibleSpec([]byte{1, 2, 3, 4, 5, 6, 7, 8})
	first.bitsPerSample = 32
	first.validBits = 24
	second := first
	second.validBits = 32

	_, err := CombineWavData([][]byte{buildExtensibleWAV(first), buildExtensibleWAV(second)})
	mismatch, ok := errors.AsType[*ErrMismatchedWAVFormat](err)
	if !ok {
		t.Fatalf("error type = %T, want *ErrMismatchedWAVFormat", err)
	}
	if mismatch.Field != "valid bits per sample" || mismatch.First != 24 || mismatch.Got != 32 {
		t.Errorf("Field/First/Got = %q/%d/%d, want valid bits per sample/24/32", mismatch.Field, mismatch.First, mismatch.Got)
	}
}

func TestInspectReadsValidBits(t *testing.T) {
	spec := defaultExtensibleSpec([]byte{1, 2, 3, 4, 5, 6, 7, 8})
	spec.bitsPerSample = 32
	spec.validBits = 24

	info, err := Inspect(buildExtensibleWAV(spec))
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	if info.Format.ValidBitsPerSample != 24 {
		t.Errorf("ValidBitsPerSample = %d, want 24", info.Format.ValidBitsPerSample)
	}
}

func TestInspectRejectsValidBitsLargerThanContainer(t *testing.T) {
	spec := defaultExtensibleSpec([]byte{1, 2, 3, 4})
	spec.validBits = 24

	_, err := Inspect(buildExtensibleWAV(spec))
	if _, ok := errors.AsType[*ErrInvalidWAVHeader](err); !ok {
		t.Fatalf("error type = %T, want *ErrInvalidWAVHeader", err)
	}
}

// TestRejectsUnsupportedFormat checks that formats that cannot be joined without decoding
// are rejected before combining. For compressed formats the bytes per sample do not
// follow from the bit depth, so neither block boundaries nor silence can be guaranteed.
func TestRejectsUnsupportedFormat(t *testing.T) {
	plain := func(tag, bits uint16) []byte {
		return buildWAV(wavSpec{audioFormat: tag, numChannels: 1, sampleRate: 24000, bitsPerSample: bits, audio: []byte{1, 2, 3, 4}})
	}
	extensible := func(subFormat [16]byte, bits uint16) []byte {
		spec := defaultExtensibleSpec([]byte{1, 2, 3, 4, 5, 6, 7, 8})
		spec.subFormat = subFormat
		spec.bitsPerSample = bits
		return buildExtensibleWAV(spec)
	}
	nonStandardGUID := subFormatPCM
	nonStandardGUID[15] ^= 0xFF

	tests := []struct {
		name string
		wav  []byte
	}{
		{"IMA ADPCM", plain(0x11, 4)},
		{"MS ADPCM", plain(2, 4)},
		{"MP3", plain(0x55, 0)},
		{"4-bit PCM", plain(pcmFormatTag, 4)},
		{"12-bit PCM", plain(pcmFormatTag, 12)},
		{"16-bit float", plain(floatFormatTag, 16)},
		{"16-bit μ-law", plain(muLawFormatTag, 16)},
		{"extensible ADPCM", extensible(subFormatGUID(0x11), 16)},
		{"extensible non-standard GUID", extensible(nonStandardGUID, 16)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Inspect(tt.wav)
			unsupported, ok := errors.AsType[*ErrUnsupportedWAVFormat](err)
			if !ok {
				t.Fatalf("Inspect() error type = %T, want *ErrUnsupportedWAVFormat", err)
			}
			if unsupported.Index != -1 {
				t.Errorf("Index = %d, want -1", unsupported.Index)
			}

			_, err = CombineWavData([][]byte{buildWAV(defaultSpec([]byte{1, 2})), tt.wav})
			unsupported, ok = errors.AsType[*ErrUnsupportedWAVFormat](err)
			if !ok {
				t.Fatalf("CombineWavData() error type = %T, want *ErrUnsupportedWAVFormat", err)
			}
			if unsupported.Index != 1 {
				t.Errorf("Index = %d, want 1", unsupported.Index)
			}
		})
	}
}

func TestAcceptsSupportedFormat(t *testing.T) {
	tests := []struct {
		name string
		spec wavSpec
	}{
		{"8-bit PCM", wavSpec{audioFormat: pcmFormatTag, bitsPerSample: 8}},
		{"24-bit PCM", wavSpec{audioFormat: pcmFormatTag, bitsPerSample: 24}},
		{"32-bit float", wavSpec{audioFormat: floatFormatTag, bitsPerSample: 32}},
		{"64-bit float", wavSpec{audioFormat: floatFormatTag, bitsPerSample: 64}},
		{"A-law", wavSpec{audioFormat: aLawFormatTag, bitsPerSample: 8}},
		{"μ-law", wavSpec{audioFormat: muLawFormatTag, bitsPerSample: 8}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.spec.numChannels = 1
			tt.spec.sampleRate = 8000
			tt.spec.audio = make([]byte, 24)
			if _, err := Inspect(buildWAV(tt.spec)); err != nil {
				t.Errorf("Inspect() error = %v", err)
			}
		})
	}
}

func TestFormatGUID(t *testing.T) {
	if got, want := formatGUID(subFormatPCM), "{00000001-0000-0010-8000-00AA00389B71}"; got != want {
		t.Errorf("formatGUID() = %s, want %s", got, want)
	}
}

func TestFormatSilenceByte(t *testing.T) {
	tests := []struct {
		name   string
		format Format
		want   byte
	}{
		// Only 8-bit linear PCM is unsigned; the midpoint 0x80 is silence.
		{"8bit PCM", Format{AudioFormat: pcmFormatTag, BitsPerSample: 8}, 0x80},
		{"16bit PCM", Format{AudioFormat: pcmFormatTag, BitsPerSample: 16}, 0x00},
		{"32bit float", Format{AudioFormat: 3, BitsPerSample: 32}, 0x00},
		{"extensible 8bit PCM", Format{AudioFormat: extensibleFormatTag, BitsPerSample: 8, SubFormat: subFormatPCM}, 0x80},
		// G.711 has different encoding rules altogether, so writing 0x00 gives noise, not silence.
		{"μ-law", Format{AudioFormat: muLawFormatTag, BitsPerSample: 8}, 0xFF},
		{"A-law", Format{AudioFormat: aLawFormatTag, BitsPerSample: 8}, 0xD5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.format.silenceByte(); got != tt.want {
				t.Errorf("silenceByte() = %#x, want %#x", got, tt.want)
			}
		})
	}
}
