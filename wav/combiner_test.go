package wav

import (
	"bytes"
	"encoding/binary"
	"errors"
	"strings"
	"testing"
)

func TestCombineWavDataConcatenatesAudioPayloads(t *testing.T) {
	first := testWAV([]byte{1, 2, 3})
	second := testWAV([]byte{4, 5})

	combined, err := CombineWavData([][]byte{first, second})
	if err != nil {
		t.Fatalf("CombineWavData() error = %v", err)
	}

	if string(combined[:4]) != "RIFF" {
		t.Fatalf("header chunk = %q, want RIFF", string(combined[:4]))
	}
	if !strings.Contains(string(combined[:16]), "WAVE") {
		t.Fatal("combined wav does not contain WAVE header")
	}
	if len(combined) < WavTotalHeaderSize {
		t.Fatalf("combined wav is too short: %d bytes", len(combined))
	}
	gotAudio := combined[len(combined)-5:]
	wantAudio := []byte{1, 2, 3, 4, 5}
	dataSize := binary.LittleEndian.Uint32(combined[len(combined)-5-4 : len(combined)-5])
	expectedSize := uint32(len(wantAudio))
	if dataSize != expectedSize {
		t.Fatalf("data size = %d, want %d", dataSize, expectedSize)
	}
	if !bytes.Equal(gotAudio, wantAudio) {
		t.Fatalf("audio payload = %v, want %v", gotAudio, wantAudio)
	}
}

func TestCombineWavDataReturnsErrorOnEmptyInput(t *testing.T) {
	_, err := CombineWavData(nil)
	if err == nil {
		t.Fatal("CombineWavData() error = nil, want ErrNoAudioData")
	}
	if _, ok := errors.AsType[*ErrNoAudioData](err); !ok {
		t.Fatalf("error type = %T, want *ErrNoAudioData", err)
	}
}

func testWAV(audio []byte) []byte {
	header := make([]byte, 44)
	copy(header[0:], []byte("RIFF"))
	binary.LittleEndian.PutUint32(header[4:], uint32(36+len(audio)))
	copy(header[8:], []byte("WAVE"))
	copy(header[12:], []byte("fmt "))
	binary.LittleEndian.PutUint32(header[16:], 16)
	binary.LittleEndian.PutUint16(header[20:], 1)
	binary.LittleEndian.PutUint16(header[22:], 1)
	binary.LittleEndian.PutUint32(header[24:], 24000)
	binary.LittleEndian.PutUint32(header[28:], 48000)
	binary.LittleEndian.PutUint16(header[32:], 2)
	binary.LittleEndian.PutUint16(header[34:], 16)
	copy(header[36:], []byte("data"))
	binary.LittleEndian.PutUint32(header[40:], uint32(len(audio)))
	return append(header, audio...)
}
