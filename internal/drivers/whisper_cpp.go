package drivers

import (
	"context"
	"fmt"
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

// WhisperCPP implements Driver over locally-loaded whisper.cpp models.
type WhisperCPP struct {
	cache *modelCache[whisperModel]
}

func NewWhisperCPP(models map[string]string, unloadAfter time.Duration) *WhisperCPP {
	return &WhisperCPP{cache: newModelCache(models, unloadAfter,
		func(path string) (whisperModel, error) { return whisper.Load(path) },
		func(m whisperModel) { m.Close() },
		config.DriverWhisperCPP)}
}

func (w *WhisperCPP) run(ctx context.Context, samples []float32, sampleRate int,
	model string, opts whisper.Options) (whisper.Result, error) {
	if err := ctx.Err(); err != nil {
		return whisper.Result{}, err
	}
	e, err := w.cache.acquire(model)
	if err != nil {
		return whisper.Result{}, err
	}
	defer w.cache.release(model, e)
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
