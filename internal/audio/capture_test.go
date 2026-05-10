package audio

import (
	"math"
	"testing"
)

func TestResampleLinear_NoOpWhenRatesMatch(t *testing.T) {
	in := []float32{0.1, 0.2, 0.3, 0.4}
	out := resampleLinear(in, 16000, 16000)
	if len(out) != len(in) {
		t.Fatalf("length: got %d, want %d", len(out), len(in))
	}
	for i := range in {
		if out[i] != in[i] {
			t.Errorf("sample %d: got %v, want %v", i, out[i], in[i])
		}
	}
}

func TestResampleLinear_DownsampleLength(t *testing.T) {
	// 48 kHz → 16 kHz: 3:1 ratio. 300 samples in → 100 samples out.
	in := make([]float32, 300)
	out := resampleLinear(in, 48000, 16000)
	if len(out) != 100 {
		t.Fatalf("length: got %d, want 100", len(out))
	}
}

func TestResampleLinear_UpsampleLength(t *testing.T) {
	// 8 kHz → 16 kHz: doubles.
	in := make([]float32, 50)
	out := resampleLinear(in, 8000, 16000)
	if len(out) != 100 {
		t.Fatalf("length: got %d, want 100", len(out))
	}
}

func TestResampleLinear_Empty(t *testing.T) {
	if got := resampleLinear(nil, 48000, 16000); got != nil {
		t.Errorf("nil input: got %v, want nil", got)
	}
	if got := resampleLinear([]float32{}, 48000, 16000); len(got) != 0 {
		t.Errorf("empty input: got len %d, want 0", len(got))
	}
}

func TestResampleLinear_PreservesSineRoughly(t *testing.T) {
	// A 1 kHz sine sampled at 48 kHz should still look like a 1 kHz sine
	// after downsampling to 16 kHz. Energy should be preserved within ~5%.
	const inRate = 48000
	const outRate = 16000
	const freq = 1000.0
	const dur = 0.1 // 100 ms

	inLen := int(inRate * dur)
	in := make([]float32, inLen)
	for i := range in {
		in[i] = float32(math.Sin(2 * math.Pi * freq * float64(i) / inRate))
	}
	out := resampleLinear(in, inRate, outRate)

	expectedLen := int(outRate * dur)
	if got := len(out); got < expectedLen-1 || got > expectedLen+1 {
		t.Errorf("length: got %d, want ~%d", got, expectedLen)
	}

	var inEnergy, outEnergy float64
	for _, s := range in {
		inEnergy += float64(s) * float64(s)
	}
	for _, s := range out {
		outEnergy += float64(s) * float64(s)
	}
	inMean := inEnergy / float64(len(in))
	outMean := outEnergy / float64(len(out))
	if math.Abs(inMean-outMean)/inMean > 0.1 {
		t.Errorf("mean-square energy diverged: in=%.4f out=%.4f", inMean, outMean)
	}
}

func TestResampleLinear_InvalidRates(t *testing.T) {
	in := []float32{0.1, 0.2, 0.3}
	if got := resampleLinear(in, 0, 16000); len(got) != len(in) {
		t.Errorf("zero inRate: should pass through, got len %d", len(got))
	}
	if got := resampleLinear(in, 48000, 0); len(got) != len(in) {
		t.Errorf("zero outRate: should pass through, got len %d", len(got))
	}
}
