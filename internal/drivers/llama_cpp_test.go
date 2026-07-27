package drivers

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dmtrkzntsv/gosaid/internal/config"
	"github.com/dmtrkzntsv/gosaid/internal/llama"
)

func TestLlamaCPPTranscribeUnsupported(t *testing.T) {
	d := NewLlamaCPP(map[string]string{"gemma": "/tmp/x.gguf"}, 0)
	_, err := d.Transcribe(context.Background(), []float32{0}, 16000, "gemma", TranscribeOptions{})
	if err == nil || !strings.Contains(err.Error(), "do not support transcription") {
		t.Fatalf("expected transcription-unsupported error, got: %v", err)
	}
	_, err = d.TranslateSpeech(context.Background(), []float32{0}, 16000, "gemma", TranslateSpeechOptions{})
	if err == nil || !strings.Contains(err.Error(), "do not support speech translation") {
		t.Fatalf("expected speech-translation-unsupported error, got: %v", err)
	}
}

func TestLlamaCPPChatUnknownModel(t *testing.T) {
	d := NewLlamaCPP(map[string]string{"gemma": "/tmp/x.gguf"}, 0)
	_, err := d.Chat(context.Background(), "other", "sys", "user")
	if err == nil || !strings.Contains(err.Error(), "unknown model") {
		t.Fatalf("expected unknown-model error, got: %v", err)
	}
}

// fakeLlamaModel stands in for a loaded llama.Model.
type fakeLlamaModel struct {
	mu     sync.Mutex
	closed bool
	calls  int
}

func (f *fakeLlamaModel) Chat(ctx context.Context, system, user string, opts llama.Options) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return "cleaned: " + user, nil
}

func (f *fakeLlamaModel) Close() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
}

func (f *fakeLlamaModel) isClosed() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.closed
}

func TestLlamaCPPChatUsesLoadedModel(t *testing.T) {
	fake := &fakeLlamaModel{}
	loads := 0
	d := NewLlamaCPP(map[string]string{"gemma": "/tmp/x.gguf"}, 0)
	d.cache.load = func(path string) (llamaModel, error) { loads++; return fake, nil }

	out, err := d.Chat(context.Background(), "gemma", "sys", "hello")
	if err != nil {
		t.Fatal(err)
	}
	if out != "cleaned: hello" {
		t.Fatalf("unexpected chat output %q", out)
	}
	if _, err := d.Chat(context.Background(), "gemma", "sys", "again"); err != nil {
		t.Fatal(err)
	}
	if loads != 1 {
		t.Fatalf("expected model to load once and stay resident, got %d loads", loads)
	}
}

func chatOK(t *testing.T, d *LlamaCPP) {
	t.Helper()
	if _, err := d.Chat(context.Background(), "gemma", "sys", "hello"); err != nil {
		t.Fatal(err)
	}
}

func TestLlamaCPPUnloadAfterIdle(t *testing.T) {
	var mu sync.Mutex
	var models []*fakeLlamaModel
	d := NewLlamaCPP(map[string]string{"gemma": "/tmp/x.gguf"}, 20*time.Millisecond)
	d.cache.load = func(path string) (llamaModel, error) {
		mu.Lock()
		defer mu.Unlock()
		m := &fakeLlamaModel{}
		models = append(models, m)
		return m, nil
	}

	chatOK(t, d)
	waitFor(t, 2*time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(models) == 1 && models[0].isClosed()
	}, "LLM was not unloaded after idle timeout")

	chatOK(t, d)
	mu.Lock()
	loads := len(models)
	mu.Unlock()
	if loads != 2 {
		t.Fatalf("expected LLM reload after unload, got %d loads", loads)
	}
}

func TestLlamaCPPUnloadDisabledByDefault(t *testing.T) {
	fake := &fakeLlamaModel{}
	loads := 0
	d := NewLlamaCPP(map[string]string{"gemma": "/tmp/x.gguf"}, 0)
	d.cache.load = func(path string) (llamaModel, error) {
		loads++
		return fake, nil
	}

	chatOK(t, d)
	time.Sleep(80 * time.Millisecond)
	chatOK(t, d)
	if fake.isClosed() {
		t.Fatal("LLM was unloaded with unload_after_seconds = 0")
	}
	if loads != 1 {
		t.Fatalf("expected LLM to stay resident, got %d loads", loads)
	}
}

func TestBuildRegistryLlamaCPP(t *testing.T) {
	cfg := &config.Config{
		UnloadAfterSeconds: 42,
		Drivers: []config.Driver{{
			Driver: config.DriverLlamaCPP,
			Endpoints: []config.Endpoint{{
				ID:     "local-llm",
				Config: config.EndpointConfig{Models: map[string]string{"gemma": "/tmp/x.gguf"}},
			}},
		}},
	}
	r, err := BuildRegistry(cfg)
	if err != nil {
		t.Fatal(err)
	}
	d, err := r.Endpoint("local-llm")
	if err != nil {
		t.Fatal(err)
	}
	local, ok := d.(*LlamaCPP)
	if !ok {
		t.Fatalf("expected *LlamaCPP, got %T", d)
	}
	if local.cache.unloadAfter != 42*time.Second {
		t.Fatalf("unloadAfter = %s, want 42s", local.cache.unloadAfter)
	}
}
