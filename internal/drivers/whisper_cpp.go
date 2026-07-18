package drivers

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/dmtrkzntsv/gosaid/internal/audio"
	"github.com/dmtrkzntsv/gosaid/internal/config"
	"github.com/dmtrkzntsv/gosaid/internal/whisper"
)

// whisperModel is the loaded-model surface WhisperCPP needs; *whisper.Model
// satisfies it, tests substitute fakes via the load hook.
type whisperModel interface {
	Transcribe(samples []float32, opts whisper.Options) (whisper.Result, error)
	Close()
}

// modelEntry is a cached loaded model with idle-unload bookkeeping. All
// fields are guarded by WhisperCPP.mu; an entry is only removed from the
// cache (and its model closed) when inflight is zero, so a holder returned
// by acquire can never have the model freed under it.
type modelEntry struct {
	m        whisperModel
	inflight int
	lastUse  time.Time
	timer    *time.Timer // armed while idle and unloading is enabled
}

// WhisperCPP implements Driver over locally-loaded whisper.cpp models.
// Models load lazily on first use and stay resident; a failed load is not
// cached, so the next press retries. If unloadAfter is positive, a model
// idle for that long is freed and reloaded lazily on next use.
type WhisperCPP struct {
	mu          sync.Mutex
	paths       map[string]string // model name → GGML file path (from config)
	loaded      map[string]*modelEntry
	unloadAfter time.Duration
	load        func(path string) (whisperModel, error)
}

func NewWhisperCPP(models map[string]string, unloadAfter time.Duration) *WhisperCPP {
	return &WhisperCPP{
		paths:       models,
		loaded:      map[string]*modelEntry{},
		unloadAfter: unloadAfter,
		load: func(path string) (whisperModel, error) {
			return whisper.Load(path)
		},
	}
}

// acquire returns the cached model for name, loading it if needed, with its
// in-flight count incremented. Callers must pair it with release.
func (w *WhisperCPP) acquire(name string) (*modelEntry, error) {
	w.mu.Lock()
	if e, ok := w.loaded[name]; ok {
		e.inflight++
		w.mu.Unlock()
		return e, nil
	}
	p, ok := w.paths[name]
	w.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("whisper_cpp: unknown model %q", name)
	}

	abs, err := config.ExpandPath(p)
	if err != nil {
		return nil, err
	}
	// The load runs a potentially multi-second cgo call; it must not hold
	// w.mu, or a concurrent Transcribe for a different, already-cached model
	// would block on it for no reason.
	m, err := w.load(abs)
	if err != nil {
		return nil, err
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	if existing, ok := w.loaded[name]; ok {
		// Another goroutine won the race and cached its instance first;
		// close our redundant one and use theirs.
		m.Close()
		existing.inflight++
		return existing, nil
	}
	e := &modelEntry{m: m, inflight: 1}
	w.loaded[name] = e
	return e, nil
}

// release ends a use begun by acquire and, once the model is idle, arms the
// unload timer.
func (w *WhisperCPP) release(name string, e *modelEntry) {
	w.mu.Lock()
	defer w.mu.Unlock()
	e.inflight--
	e.lastUse = time.Now()
	if w.unloadAfter <= 0 || e.inflight > 0 {
		return
	}
	if e.timer == nil {
		e.timer = time.AfterFunc(w.unloadAfter, func() { w.maybeUnload(name) })
	} else {
		e.timer.Reset(w.unloadAfter)
	}
}

// maybeUnload frees the named model if it has been idle for the configured
// duration. Fired by the entry's timer; if a transcription is in flight the
// unload is skipped and the next release re-arms the timer.
func (w *WhisperCPP) maybeUnload(name string) {
	w.mu.Lock()
	e, ok := w.loaded[name]
	if !ok || e.inflight > 0 {
		w.mu.Unlock()
		return
	}
	if idle := time.Since(e.lastUse); idle < w.unloadAfter {
		// Used again after this timer was armed; try again later.
		e.timer.Reset(w.unloadAfter - idle)
		w.mu.Unlock()
		return
	}
	delete(w.loaded, name)
	w.mu.Unlock()
	e.m.Close()
	slog.Info("whisper model unloaded after idle timeout", "model", name,
		"unload_after", w.unloadAfter)
}

func (w *WhisperCPP) run(ctx context.Context, samples []float32, sampleRate int,
	model string, opts whisper.Options) (whisper.Result, error) {
	if err := ctx.Err(); err != nil {
		return whisper.Result{}, err
	}
	e, err := w.acquire(model)
	if err != nil {
		return whisper.Result{}, err
	}
	defer w.release(model, e)
	if sampleRate != audio.CaptureSampleRate {
		samples = audio.ResampleLinear(samples, sampleRate, audio.CaptureSampleRate)
	}
	return e.m.Transcribe(samples, opts)
}

func (w *WhisperCPP) Transcribe(ctx context.Context, samples []float32, sampleRate int,
	model string, opts TranscribeOptions) (TranscribeResult, error) {
	res, err := w.run(ctx, samples, sampleRate, model, whisper.Options{
		Language:      opts.Language,
		InitialPrompt: opts.InitialPrompt,
	})
	if err != nil {
		return TranscribeResult{}, err
	}
	return TranscribeResult{Text: res.Text, DetectedLanguage: res.DetectedLanguage}, nil
}

func (w *WhisperCPP) TranslateSpeech(ctx context.Context, samples []float32, sampleRate int,
	model string, opts TranslateSpeechOptions) (string, error) {
	res, err := w.run(ctx, samples, sampleRate, model, whisper.Options{
		Language:      opts.SourceLanguage,
		Translate:     true,
		InitialPrompt: opts.InitialPrompt,
	})
	if err != nil {
		return "", err
	}
	return res.Text, nil
}

// Chat is a backstop: config validation already rejects chat-stage refs to
// whisper_cpp endpoints.
func (w *WhisperCPP) Chat(ctx context.Context, model, system, user string) (string, error) {
	return "", fmt.Errorf("whisper_cpp endpoints do not support chat stages")
}
