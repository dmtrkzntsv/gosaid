package drivers

import (
	"context"
	"strings"
	"testing"

	"github.com/dmtrkzntsv/gosaid/internal/config"
)

func TestWhisperCPPChatUnsupported(t *testing.T) {
	d := NewWhisperCPP(map[string]string{"base": "/tmp/x.bin"})
	_, err := d.Chat(context.Background(), "base", "sys", "user")
	if err == nil || !strings.Contains(err.Error(), "do not support chat") {
		t.Fatalf("expected chat-unsupported error, got: %v", err)
	}
}

func TestWhisperCPPUnknownModel(t *testing.T) {
	d := NewWhisperCPP(map[string]string{"base": "/tmp/x.bin"})
	_, err := d.Transcribe(context.Background(), []float32{0}, 16000, "huge", TranscribeOptions{})
	if err == nil || !strings.Contains(err.Error(), "unknown model") {
		t.Fatalf("expected unknown-model error, got: %v", err)
	}
}

func TestBuildRegistryWhisperCPP(t *testing.T) {
	cfg := &config.Config{Drivers: []config.Driver{{
		Driver: config.DriverWhisperCPP,
		Endpoints: []config.Endpoint{{
			ID:     "local",
			Config: config.EndpointConfig{Models: map[string]string{"base": "/tmp/x.bin"}},
		}},
	}}}
	r, err := BuildRegistry(cfg)
	if err != nil {
		t.Fatal(err)
	}
	d, err := r.Endpoint("local")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := d.(*WhisperCPP); !ok {
		t.Fatalf("expected *WhisperCPP, got %T", d)
	}
}
