package audio

import (
	"bytes"
	"testing"
)

func TestEncodeDecodeRoundTrip(t *testing.T) {
	in := []float32{0, 0.5, -0.5, 1, -1, 0.25}
	wav := EncodeWAV(in, 16000)

	pcm, err := ParseWAV(wav)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if pcm.SampleRate != 16000 {
		t.Fatalf("sample rate = %d", pcm.SampleRate)
	}
	if pcm.Channels != 1 {
		t.Fatalf("channels = %d", pcm.Channels)
	}
	if len(pcm.Data) != len(in)*2 {
		t.Fatalf("data size = %d, want %d", len(pcm.Data), len(in)*2)
	}
}

func TestParseWAV_Rejects(t *testing.T) {
	if _, err := ParseWAV([]byte("nope")); err == nil {
		t.Error("expected error for garbage input")
	}
	// Valid RIFF/WAVE but non-PCM format code.
	bad := bytes.Clone(EncodeWAV([]float32{0}, 16000))
	bad[20] = 3 // IEEE float
	if _, err := ParseWAV(bad); err == nil {
		t.Error("expected error for non-PCM format")
	}
}

func TestEncodeClips(t *testing.T) {
	// Values outside [-1, 1] must clip, not wrap.
	wav := EncodeWAV([]float32{2.0, -2.0}, 8000)
	pcm, err := ParseWAV(wav)
	if err != nil {
		t.Fatal(err)
	}
	// First sample: 32767 = 0x7FFF (little-endian: FF 7F).
	if pcm.Data[0] != 0xFF || pcm.Data[1] != 0x7F {
		t.Errorf("positive clip wrong: %x %x", pcm.Data[0], pcm.Data[1])
	}
	// Second sample: -32767 = 0x8001.
	if pcm.Data[2] != 0x01 || pcm.Data[3] != 0x80 {
		t.Errorf("negative clip wrong: %x %x", pcm.Data[2], pcm.Data[3])
	}
}
