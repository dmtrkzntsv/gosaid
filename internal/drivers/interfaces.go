package drivers

import "context"

// Driver is the minimum surface every transcription/chat backend must expose.
// Only `openai_compatible` is implemented; the interface exists to keep the
// pipeline agnostic.
type Driver interface {
	// Transcribe performs general speech-to-text. On auto-detect, the result's
	// DetectedLanguage is non-empty.
	Transcribe(ctx context.Context, samples []float32, sampleRate int, model string,
		opts TranscribeOptions) (TranscribeResult, error)

	// TranslateSpeech uses Whisper's native translate task, which outputs
	// English regardless of source language. Kept as a fast path.
	TranslateSpeech(ctx context.Context, samples []float32, sampleRate int, model string,
		opts TranslateSpeechOptions) (string, error)

	// Chat runs a single-turn system+user completion and returns the assistant's text.
	Chat(ctx context.Context, model, system, user string) (string, error)
}

type TranscribeOptions struct {
	Language      string // "" for auto-detect
	InitialPrompt string // vocabulary hint
}

type TranscribeResult struct {
	Text             string
	DetectedLanguage string
}

type TranslateSpeechOptions struct {
	SourceLanguage string
	InitialPrompt  string
}
