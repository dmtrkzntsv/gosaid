package drivers

import (
	"context"
	"strings"
	"sync"
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

// TestWhisperCPPConcurrentModelAccess exercises the cold-load path in
// model() from many goroutines at once, for both the same model name (the
// duplicate-load race) and different model names (lock-scoping regression:
// w.mu must not be held across whisper.Load, or a concurrent request for one
// model would block on an unrelated model's load). All configured paths are
// nonexistent, so whisper.Load fails fast without needing a real model file;
// run with -race to catch any data race on the loaded map.
func TestWhisperCPPConcurrentModelAccess(t *testing.T) {
	d := NewWhisperCPP(map[string]string{
		"missing-a": "/nonexistent/path/a.bin",
		"missing-b": "/nonexistent/path/b.bin",
	})

	var wg sync.WaitGroup
	const n = 20
	for i := 0; i < n; i++ {
		name := "missing-a"
		if i%2 == 0 {
			name = "missing-b"
		}
		wg.Add(1)
		go func(model string) {
			defer wg.Done()
			_, err := d.Transcribe(context.Background(), []float32{0}, 16000, model, TranscribeOptions{})
			if err == nil {
				t.Errorf("expected load error for nonexistent model file %q", model)
			}
		}(name)
	}
	wg.Wait()

	// A failed load must not be cached: the model stays absent from the map.
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.loaded) != 0 {
		t.Fatalf("expected no models cached after failed loads, got %d", len(d.loaded))
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
