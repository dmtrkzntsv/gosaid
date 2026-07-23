package setup

import (
	"testing"

	"github.com/dmtrkzntsv/gosaid/internal/config"
)

func TestHotkeySummary(t *testing.T) {
	hk := config.Hotkey{
		Mode:       config.ModeHold,
		Transcribe: config.TranscribeStage{Model: "openai:whisper-1"},
		Enhance:    &config.EnhanceStage{Model: "openai:gpt-5.4-nano"},
		Translate:  &config.TranslateStage{OutputLanguage: "en", Model: "openai:gpt-5.4-nano"},
	}
	want := "option+right · hold · transcribe → enhance → translate(en)"
	if got := HotkeySummary("option+right", hk); got != want {
		t.Errorf("got %q\nwant %q", got, want)
	}

	plain := config.Hotkey{Transcribe: config.TranscribeStage{Model: "local:base"}}
	want = "option+left · hold · transcribe"
	if got := HotkeySummary("option+left", plain); got != want {
		t.Errorf("empty mode defaults to hold: got %q, want %q", got, want)
	}

	comp := config.Hotkey{
		Mode:       config.ModeToggle,
		Transcribe: config.TranscribeStage{Model: "openai:whisper-1"},
		Compose:    &config.ComposeStage{Model: "openai:gpt-5.4-nano"},
	}
	want = "option+up · toggle · transcribe → compose"
	if got := HotkeySummary("option+up", comp); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
