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

func TestEndpointSummary(t *testing.T) {
	cloud := config.Endpoint{ID: "openai", Config: config.EndpointConfig{APIBase: "https://api.openai.com/v1", APIKey: "sk-x"}}
	if got := EndpointSummary(config.DriverOpenAICompatible, cloud); got != "openai · api.openai.com" {
		t.Errorf("got %q", got)
	}
	local := config.Endpoint{ID: "local", Config: config.EndpointConfig{Models: map[string]string{"base": "/m/b.bin", "tiny": "/m/t.bin"}}}
	if got := EndpointSummary(config.DriverWhisperCPP, local); got != "local · whisper_cpp (2 models)" {
		t.Errorf("got %q", got)
	}
	one := config.Endpoint{ID: "local", Config: config.EndpointConfig{Models: map[string]string{"base": "/m/b.bin"}}}
	if got := EndpointSummary(config.DriverWhisperCPP, one); got != "local · whisper_cpp (1 model)" {
		t.Errorf("got %q", got)
	}
}
