package drivers

import (
	"context"
	"fmt"
	"sync"

	"github.com/dmtrkzntsv/gosaid/internal/audio"
	"github.com/dmtrkzntsv/gosaid/internal/config"
	"github.com/dmtrkzntsv/gosaid/internal/whisper"
)

// WhisperCPP implements Driver over locally-loaded whisper.cpp models.
// Models load lazily on first use and stay resident; a failed load is not
// cached, so the next press retries.
type WhisperCPP struct {
	mu     sync.Mutex
	paths  map[string]string // model name → GGML file path (from config)
	loaded map[string]*whisper.Model
}

func NewWhisperCPP(models map[string]string) *WhisperCPP {
	return &WhisperCPP{paths: models, loaded: map[string]*whisper.Model{}}
}

func (w *WhisperCPP) model(name string) (*whisper.Model, error) {
	w.mu.Lock()
	if m, ok := w.loaded[name]; ok {
		w.mu.Unlock()
		return m, nil
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
	// whisper.Load runs a potentially multi-second cgo call; it must not hold
	// w.mu, or a concurrent Transcribe for a different, already-cached model
	// would block on it for no reason.
	m, err := whisper.Load(abs)
	if err != nil {
		return nil, err
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	if existing, ok := w.loaded[name]; ok {
		// Another goroutine won the race and cached its instance first;
		// close our redundant one and use theirs.
		m.Close()
		return existing, nil
	}
	w.loaded[name] = m
	return m, nil
}

func (w *WhisperCPP) run(ctx context.Context, samples []float32, sampleRate int,
	model string, opts whisper.Options) (whisper.Result, error) {
	if err := ctx.Err(); err != nil {
		return whisper.Result{}, err
	}
	m, err := w.model(model)
	if err != nil {
		return whisper.Result{}, err
	}
	if sampleRate != audio.CaptureSampleRate {
		samples = audio.ResampleLinear(samples, sampleRate, audio.CaptureSampleRate)
	}
	return m.Transcribe(samples, opts)
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
