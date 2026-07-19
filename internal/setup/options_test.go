package setup

import (
	"testing"

	"github.com/dmtrkzntsv/gosaid/internal/config"
)

// testConfig returns a config with one openai preset endpoint and one local
// whisper endpoint with two models.
func testConfig() *config.Config {
	return &config.Config{
		Version: config.CurrentVersion,
		Drivers: []config.Driver{
			{Driver: config.DriverOpenAICompatible, Endpoints: []config.Endpoint{
				{ID: "openai", Config: config.EndpointConfig{APIBase: "https://api.openai.com/v1", APIKey: "sk-x"}},
			}},
			{Driver: config.DriverWhisperCPP, Endpoints: []config.Endpoint{
				{ID: "local", Config: config.EndpointConfig{Models: map[string]string{
					"base": "/m/ggml-base.bin", "tiny": "/m/ggml-tiny.bin",
				}}},
			}},
		},
		Hotkeys:          map[string]config.Hotkey{},
		ToggleMaxSeconds: 60,
	}
}

func TestTranscribeModelOptions(t *testing.T) {
	opts := TranscribeModelOptions(testConfig())
	refs := map[string]bool{}
	for _, o := range opts {
		refs[o.Ref] = true
	}
	for _, want := range []string{"local:base", "local:tiny", "openai:whisper-1"} {
		if !refs[want] {
			t.Errorf("missing option %q in %v", want, opts)
		}
	}
}

func TestChatModelOptions(t *testing.T) {
	opts := ChatModelOptions(testConfig())
	if len(opts) != 1 || opts[0].Ref != "openai:gpt-5.4-nano" {
		t.Fatalf("got %v, want single openai chat option", opts)
	}
	// whisper_cpp endpoints must never appear as chat options.
	for _, o := range opts {
		if o.Ref == "local:base" || o.Ref == "local:tiny" {
			t.Errorf("whisper_cpp leaked into chat options: %v", o)
		}
	}
}

func TestOpenAIEndpointIDs(t *testing.T) {
	got := OpenAIEndpointIDs(testConfig())
	if len(got) != 1 || got[0] != "openai" {
		t.Fatalf("got %v", got)
	}
}

func TestSuggestedCombosParse(t *testing.T) {
	if len(SuggestedCombos) < 4 {
		t.Fatalf("want a useful list, got %v", SuggestedCombos)
	}
}
