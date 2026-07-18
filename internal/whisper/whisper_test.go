package whisper

import (
	"encoding/binary"
	"os"
	"strings"
	"testing"
)

// readWAV16 reads a 16-bit PCM mono WAV file and returns float32 samples.
// Minimal parser sufficient for testdata/jfk.wav (16 kHz mono PCM16).
func readWAV16(t *testing.T, path string) []float32 {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// Find the "data" chunk after the 12-byte RIFF header.
	i := 12
	for i+8 <= len(data) {
		id := string(data[i : i+4])
		size := int(binary.LittleEndian.Uint32(data[i+4 : i+8]))
		if id == "data" {
			pcm := data[i+8 : i+8+size]
			out := make([]float32, len(pcm)/2)
			for j := range out {
				out[j] = float32(int16(binary.LittleEndian.Uint16(pcm[j*2:]))) / 32768
			}
			return out
		}
		i += 8 + size
	}
	t.Fatal("no data chunk in WAV")
	return nil
}

// TestTranscribeJFK exercises the real model end-to-end. Gated: set
// GOSAID_WHISPER_MODEL to a GGML model path (e.g. ggml-tiny.en.bin) to run.
func TestTranscribeJFK(t *testing.T) {
	modelPath := os.Getenv("GOSAID_WHISPER_MODEL")
	if modelPath == "" {
		t.Skip("GOSAID_WHISPER_MODEL not set")
	}
	m, err := Load(modelPath)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()

	samples := readWAV16(t, "testdata/jfk.wav")
	res, err := m.Transcribe(samples, Options{Language: "en"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.ToLower(res.Text), "country") {
		t.Fatalf("unexpected transcript: %q", res.Text)
	}
}

func TestLoadMissingFile(t *testing.T) {
	if _, err := Load("/nonexistent/model.bin"); err == nil {
		t.Fatal("expected error for missing model file")
	}
}

func TestTranscribeEmptySamples(t *testing.T) {
	m := &Model{} // no ctx needed: empty-samples check fires first
	if _, err := m.Transcribe(nil, Options{}); err == nil {
		t.Fatal("expected error for empty samples")
	}
}
