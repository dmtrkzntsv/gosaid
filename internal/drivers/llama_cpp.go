package drivers

import (
	"context"
	"fmt"
	"time"

	"github.com/dmtrkzntsv/gosaid/internal/config"
	"github.com/dmtrkzntsv/gosaid/internal/llama"
)

// llamaModel is the loaded-model surface LlamaCPP needs; *llama.Model
// satisfies it, tests substitute fakes via the cache's load hook.
type llamaModel interface {
	Chat(ctx context.Context, system, user string, opts llama.Options) (string, error)
	Close()
}

// LlamaCPP implements Driver over locally-loaded llama.cpp GGUF models.
// It serves chat stages only; transcription refs are rejected at config
// validation time.
type LlamaCPP struct {
	cache *modelCache[llamaModel]
}

func NewLlamaCPP(models map[string]string, unloadAfter time.Duration) *LlamaCPP {
	return &LlamaCPP{cache: newModelCache(models, unloadAfter,
		func(path string) (llamaModel, error) { return llama.Load(path) },
		func(m llamaModel) { m.Close() },
		config.DriverLlamaCPP)}
}

// Preload loads model into memory without running inference.
func (l *LlamaCPP) Preload(model string) error {
	return l.cache.preload(model)
}

func (l *LlamaCPP) Chat(ctx context.Context, model, system, user string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	e, err := l.cache.acquire(model)
	if err != nil {
		return "", err
	}
	defer l.cache.release(model, e)
	return e.m.Chat(ctx, system, user, llama.Options{})
}

// Transcribe is a backstop: config validation already rejects
// transcribe-stage refs to llama_cpp endpoints.
func (l *LlamaCPP) Transcribe(ctx context.Context, samples []float32, sampleRate int,
	model string, opts TranscribeOptions) (TranscribeResult, error) {
	return TranscribeResult{}, fmt.Errorf("llama_cpp endpoints do not support transcription")
}

// TranslateSpeech is a backstop, as Transcribe.
func (l *LlamaCPP) TranslateSpeech(ctx context.Context, samples []float32, sampleRate int,
	model string, opts TranslateSpeechOptions) (string, error) {
	return "", fmt.Errorf("llama_cpp endpoints do not support speech translation")
}
