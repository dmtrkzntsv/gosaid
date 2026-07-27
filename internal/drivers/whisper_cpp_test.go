package drivers

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dmtrkzntsv/gosaid/internal/config"
	"github.com/dmtrkzntsv/gosaid/internal/whisper"
)

func TestWhisperCPPChatUnsupported(t *testing.T) {
	d := NewWhisperCPP(map[string]string{"base": "/tmp/x.bin"}, 0)
	_, err := d.Chat(context.Background(), "base", "sys", "user")
	if err == nil || !strings.Contains(err.Error(), "do not support chat") {
		t.Fatalf("expected chat-unsupported error, got: %v", err)
	}
}

func TestWhisperCPPUnknownModel(t *testing.T) {
	d := NewWhisperCPP(map[string]string{"base": "/tmp/x.bin"}, 0)
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
	}, 0)

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
	d.cache.mu.Lock()
	defer d.cache.mu.Unlock()
	if len(d.cache.loaded) != 0 {
		t.Fatalf("expected no models cached after failed loads, got %d", len(d.cache.loaded))
	}
}

// fakeWhisperModel stands in for a loaded whisper.Model in unload tests.
type fakeWhisperModel struct {
	mu     sync.Mutex
	closed bool
	delay  time.Duration // simulated inference time
}

func (f *fakeWhisperModel) Transcribe(samples []float32, opts whisper.Options) (whisper.Result, error) {
	time.Sleep(f.delay)
	return whisper.Result{Text: "ok"}, nil
}

func (f *fakeWhisperModel) Close() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
}

func (f *fakeWhisperModel) isClosed() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.closed
}

// fakeLoader counts loads and hands out fresh fake models.
type fakeLoader struct {
	mu     sync.Mutex
	delay  time.Duration
	models []*fakeWhisperModel
}

func (l *fakeLoader) load(path string) (whisperModel, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	m := &fakeWhisperModel{delay: l.delay}
	l.models = append(l.models, m)
	return m, nil
}

func (l *fakeLoader) loadCount() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.models)
}

func (l *fakeLoader) model(i int) *fakeWhisperModel {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.models[i]
}

// waitFor polls cond until it holds or the deadline passes.
func waitFor(t *testing.T, d time.Duration, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal(msg)
}

func transcribeOK(t *testing.T, d *WhisperCPP) {
	t.Helper()
	if _, err := d.Transcribe(context.Background(), []float32{0}, 16000, "base", TranscribeOptions{}); err != nil {
		t.Fatal(err)
	}
}

func TestWhisperCPPUnloadAfterIdle(t *testing.T) {
	l := &fakeLoader{}
	d := NewWhisperCPP(map[string]string{"base": "/tmp/x.bin"}, 20*time.Millisecond)
	d.cache.load = l.load

	transcribeOK(t, d)
	if l.loadCount() != 1 {
		t.Fatalf("expected 1 load, got %d", l.loadCount())
	}
	waitFor(t, 2*time.Second, func() bool { return l.model(0).isClosed() },
		"model was not unloaded after idle timeout")

	// Next use reloads lazily.
	transcribeOK(t, d)
	if l.loadCount() != 2 {
		t.Fatalf("expected reload after unload, got %d loads", l.loadCount())
	}
}

func TestWhisperCPPUnloadDisabled(t *testing.T) {
	l := &fakeLoader{}
	d := NewWhisperCPP(map[string]string{"base": "/tmp/x.bin"}, 0)
	d.cache.load = l.load

	transcribeOK(t, d)
	time.Sleep(80 * time.Millisecond)
	transcribeOK(t, d)
	if l.model(0).isClosed() {
		t.Fatal("model was unloaded with unload disabled")
	}
	if l.loadCount() != 1 {
		t.Fatalf("expected model to stay resident, got %d loads", l.loadCount())
	}
}

// TestWhisperCPPUnloadSkipsInflight arms the idle timer with a short timeout,
// then starts a slow transcription so the timer fires mid-inference. The
// in-flight run must not have its model freed under it.
func TestWhisperCPPUnloadSkipsInflight(t *testing.T) {
	l := &fakeLoader{delay: 100 * time.Millisecond}
	d := NewWhisperCPP(map[string]string{"base": "/tmp/x.bin"}, 30*time.Millisecond)
	d.cache.load = l.load

	transcribeOK(t, d) // arms the idle timer on release
	transcribeOK(t, d) // slow run; timer fires while in flight
	if l.loadCount() != 1 {
		t.Fatalf("model was unloaded under an in-flight transcription (%d loads)", l.loadCount())
	}
	// After going idle it still unloads.
	waitFor(t, 2*time.Second, func() bool { return l.model(0).isClosed() },
		"model was not unloaded after in-flight run completed")
}

func TestBuildRegistryWhisperCPP(t *testing.T) {
	cfg := &config.Config{
		UnloadAfterSeconds: 42,
		Drivers: []config.Driver{{
			Driver: config.DriverWhisperCPP,
			Endpoints: []config.Endpoint{{
				ID:     "local",
				Config: config.EndpointConfig{Models: map[string]string{"base": "/tmp/x.bin"}},
			}},
		}},
	}
	r, err := BuildRegistry(cfg)
	if err != nil {
		t.Fatal(err)
	}
	d, err := r.Endpoint("local")
	if err != nil {
		t.Fatal(err)
	}
	local, ok := d.(*WhisperCPP)
	if !ok {
		t.Fatalf("expected *WhisperCPP, got %T", d)
	}
	if local.cache.unloadAfter != 42*time.Second {
		t.Fatalf("unloadAfter = %s, want 42s", local.cache.unloadAfter)
	}
}
