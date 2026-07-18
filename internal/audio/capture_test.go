package audio

import (
	"math"
	"slices"
	"testing"
)

func TestMatchesDevice(t *testing.T) {
	cases := []struct {
		name, preferred string
		want            bool
	}{
		{"USB PnP Sound Device", "usb", true},
		{"USB PnP Sound Device", "USB PnP Sound Device", true},
		{"MacBook Pro Microphone", "macbook pro", true},
		{"MacBook Pro Microphone", "usb", false},
		{"MacBook Pro Microphone", "", false},
	}
	for _, c := range cases {
		if got := MatchesDevice(c.name, c.preferred); got != c.want {
			t.Errorf("MatchesDevice(%q, %q): got %v, want %v", c.name, c.preferred, got, c.want)
		}
	}
}

func TestOrderPreference(t *testing.T) {
	names := []string{"Webcam Mic", "MacBook Pro Microphone", "USB PnP Sound Device"}
	defaults := []bool{false, true, false}

	cases := []struct {
		desc, preferred string
		want            []int
	}{
		{"no preference: default first, then enumeration order", "", []int{1, 0, 2}},
		{"preferred first, then default, then rest", "usb", []int{2, 1, 0}},
		{"preferred is the default: no duplicates", "macbook", []int{1, 0, 2}},
		{"unplugged preference: falls back to default ordering", "airpods", []int{1, 0, 2}},
	}
	for _, c := range cases {
		if got := orderPreference(names, defaults, c.preferred); !slices.Equal(got, c.want) {
			t.Errorf("%s: got %v, want %v", c.desc, got, c.want)
		}
	}
}

func TestOrderPreference_MultipleMatches(t *testing.T) {
	names := []string{"USB Mic A", "Built-in", "USB Mic B"}
	defaults := []bool{false, true, false}
	// Both USB devices match; they keep enumeration order ahead of the default.
	if got := orderPreference(names, defaults, "usb"); !slices.Equal(got, []int{0, 2, 1}) {
		t.Errorf("got %v, want [0 2 1]", got)
	}
}

func TestResampleLinear_NoOpWhenRatesMatch(t *testing.T) {
	in := []float32{0.1, 0.2, 0.3, 0.4}
	out := ResampleLinear(in, 16000, 16000)
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
	out := ResampleLinear(in, 48000, 16000)
	if len(out) != 100 {
		t.Fatalf("length: got %d, want 100", len(out))
	}
}

func TestResampleLinear_UpsampleLength(t *testing.T) {
	// 8 kHz → 16 kHz: doubles.
	in := make([]float32, 50)
	out := ResampleLinear(in, 8000, 16000)
	if len(out) != 100 {
		t.Fatalf("length: got %d, want 100", len(out))
	}
}

func TestResampleLinear_Empty(t *testing.T) {
	if got := ResampleLinear(nil, 48000, 16000); got != nil {
		t.Errorf("nil input: got %v, want nil", got)
	}
	if got := ResampleLinear([]float32{}, 48000, 16000); len(got) != 0 {
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
	out := ResampleLinear(in, inRate, outRate)

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
	if got := ResampleLinear(in, 0, 16000); len(got) != len(in) {
		t.Errorf("zero inRate: should pass through, got len %d", len(got))
	}
	if got := ResampleLinear(in, 48000, 0); len(got) != len(in) {
		t.Errorf("zero outRate: should pass through, got len %d", len(got))
	}
}
