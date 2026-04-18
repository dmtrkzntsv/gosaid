package audio

import (
	"encoding/binary"
	"errors"
	"io"
	"math"
)

// EncodeWAV writes float32 samples in [-1, 1] as a 16-bit PCM mono WAV.
// Used to build the payload uploaded to Whisper endpoints.
func EncodeWAV(samples []float32, sampleRate int) []byte {
	const bitsPerSample = 16
	const numChannels = 1
	byteRate := sampleRate * numChannels * bitsPerSample / 8
	blockAlign := numChannels * bitsPerSample / 8
	dataSize := len(samples) * 2
	buf := make([]byte, 44+dataSize)

	copy(buf[0:4], []byte("RIFF"))
	binary.LittleEndian.PutUint32(buf[4:8], uint32(36+dataSize))
	copy(buf[8:12], []byte("WAVE"))
	copy(buf[12:16], []byte("fmt "))
	binary.LittleEndian.PutUint32(buf[16:20], 16) // PCM chunk size
	binary.LittleEndian.PutUint16(buf[20:22], 1)  // PCM
	binary.LittleEndian.PutUint16(buf[22:24], numChannels)
	binary.LittleEndian.PutUint32(buf[24:28], uint32(sampleRate))
	binary.LittleEndian.PutUint32(buf[28:32], uint32(byteRate))
	binary.LittleEndian.PutUint16(buf[32:34], uint16(blockAlign))
	binary.LittleEndian.PutUint16(buf[34:36], bitsPerSample)
	copy(buf[36:40], []byte("data"))
	binary.LittleEndian.PutUint32(buf[40:44], uint32(dataSize))

	for i, s := range samples {
		v := int16(math.Max(-1, math.Min(1, float64(s))) * 32767)
		binary.LittleEndian.PutUint16(buf[44+i*2:], uint16(v))
	}
	return buf
}

// PCM is raw 16-bit signed little-endian PCM plus its sample rate and channel count.
type PCM struct {
	Data       []byte
	SampleRate int
	Channels   int
}

// ParseWAV extracts the "data" chunk from a PCM WAV. It tolerates extra chunks
// between "fmt " and "data" and rejects anything that is not 16-bit PCM.
func ParseWAV(data []byte) (PCM, error) {
	if len(data) < 44 || string(data[0:4]) != "RIFF" || string(data[8:12]) != "WAVE" {
		return PCM{}, errors.New("not a WAV file")
	}
	// Locate the fmt chunk.
	if string(data[12:16]) != "fmt " {
		return PCM{}, errors.New("missing fmt chunk")
	}
	fmtSize := binary.LittleEndian.Uint32(data[16:20])
	audioFormat := binary.LittleEndian.Uint16(data[20:22])
	if audioFormat != 1 {
		return PCM{}, errors.New("unsupported WAV format (need PCM)")
	}
	channels := int(binary.LittleEndian.Uint16(data[22:24]))
	sampleRate := int(binary.LittleEndian.Uint32(data[24:28]))
	bitsPerSample := binary.LittleEndian.Uint16(data[34:36])
	if bitsPerSample != 16 {
		return PCM{}, errors.New("unsupported bit depth (need 16-bit PCM)")
	}

	// Scan for data chunk starting after fmt.
	pos := 20 + int(fmtSize)
	for pos+8 <= len(data) {
		id := string(data[pos : pos+4])
		size := binary.LittleEndian.Uint32(data[pos+4 : pos+8])
		if id == "data" {
			start := pos + 8
			end := min(start+int(size), len(data))
			return PCM{Data: data[start:end], SampleRate: sampleRate, Channels: channels}, nil
		}
		pos += 8 + int(size)
	}
	return PCM{}, io.ErrUnexpectedEOF
}
