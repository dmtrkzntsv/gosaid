package setup

import (
	"testing"

	"github.com/dmtrkzntsv/gosaid/internal/audio"
)

func TestMicrophoneOptionsLabelSystemDefault(t *testing.T) {
	opts := microphoneOptions([]audio.CaptureDevice{
		{Name: "USB Mic"},
		{Name: "MacBook Microphone", Default: true},
	})
	if len(opts) != 3 {
		t.Fatalf("option count = %d, want 3", len(opts))
	}
	if opts[0].Key != "System default" || opts[0].Value != "" {
		t.Fatalf("first option = %#v, want System default", opts[0])
	}
	if got := opts[2].Key; got != "MacBook Microphone (system default)" {
		t.Fatalf("default label = %q", got)
	}
}

func TestSelectedMicrophone(t *testing.T) {
	devices := []audio.CaptureDevice{
		{Name: "MacBook Microphone", Default: true},
		{Name: "USB PnP Sound Device"},
	}
	tests := []struct {
		name      string
		current   string
		want      string
		available bool
	}{
		{name: "system default", current: "", want: "", available: true},
		{name: "exact device", current: "USB PnP Sound Device", want: "USB PnP Sound Device", available: true},
		{name: "configured substring", current: "usb pnp", want: "USB PnP Sound Device", available: true},
		{name: "unavailable device", current: "Studio Display", want: "Studio Display", available: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, available := selectedMicrophone(tt.current, devices)
			if got != tt.want || available != tt.available {
				t.Fatalf("selectedMicrophone(%q) = %q, %v; want %q, %v",
					tt.current, got, available, tt.want, tt.available)
			}
		})
	}
}
