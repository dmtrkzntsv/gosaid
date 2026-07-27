package drivers

import (
	"reflect"
	"testing"

	"github.com/dmtrkzntsv/gosaid/internal/config"
)

func TestPreloadHotkeyModelsLoadsOnlyActiveLocalModels(t *testing.T) {
	cfg := &config.Config{
		Drivers: []config.Driver{
			{
				Driver: config.DriverWhisperCPP,
				Endpoints: []config.Endpoint{{
					ID: "speech",
					Config: config.EndpointConfig{Models: map[string]string{
						"stt": "/tmp/stt.bin",
					}},
				}},
			},
			{
				Driver: config.DriverLlamaCPP,
				Endpoints: []config.Endpoint{{
					ID: "text",
					Config: config.EndpointConfig{Models: map[string]string{
						"compose": "/tmp/compose.gguf",
						"shared":  "/tmp/shared.gguf",
						"unused":  "/tmp/unused.gguf",
					}},
				}},
			},
			{
				Driver: config.DriverOpenAICompatible,
				Endpoints: []config.Endpoint{{
					ID: "hosted",
					Config: config.EndpointConfig{
						APIBase: "https://example.com/v1",
						APIKey:  "test",
					},
				}},
			},
		},
		Hotkeys: map[string]config.Hotkey{
			"ctrl+alt+a": {
				Transcribe: config.TranscribeStage{Model: "speech:stt"},
				Translate: &config.TranslateStage{
					Enable: boolPointer(false),
					Model:  "text:unused",
				},
				Enhance: &config.EnhanceStage{Model: "text:shared"},
				Compose: &config.ComposeStage{Model: "text:shared"},
			},
			"ctrl+alt+b": {
				Transcribe: config.TranscribeStage{Model: "speech:stt"},
				Compose:    &config.ComposeStage{Model: "text:compose"},
			},
			"ctrl+alt+c": {
				Transcribe: config.TranscribeStage{Model: "hosted:whisper"},
				Enhance:    &config.EnhanceStage{Model: "hosted:chat"},
			},
		},
	}
	registry, err := BuildRegistry(cfg)
	if err != nil {
		t.Fatal(err)
	}

	speech := registry.endpoints["speech"].(*WhisperCPP)
	var speechLoads []string
	speech.cache.load = func(path string) (whisperModel, error) {
		speechLoads = append(speechLoads, path)
		return &fakeWhisperModel{}, nil
	}
	text := registry.endpoints["text"].(*LlamaCPP)
	var textLoads []string
	text.cache.load = func(path string) (llamaModel, error) {
		textLoads = append(textLoads, path)
		return &fakeLlamaModel{}, nil
	}

	loaded, err := registry.PreloadHotkeyModels(cfg.Hotkeys)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"speech:stt", "text:compose", "text:shared"}; !reflect.DeepEqual(loaded, want) {
		t.Fatalf("loaded refs = %v, want %v", loaded, want)
	}
	if want := []string{"/tmp/stt.bin"}; !reflect.DeepEqual(speechLoads, want) {
		t.Fatalf("speech loads = %v, want %v", speechLoads, want)
	}
	if want := []string{"/tmp/compose.gguf", "/tmp/shared.gguf"}; !reflect.DeepEqual(textLoads, want) {
		t.Fatalf("text loads = %v, want %v", textLoads, want)
	}
}

func TestPreloadHotkeyModelsReturnsLoadError(t *testing.T) {
	cfg := &config.Config{
		Drivers: []config.Driver{{
			Driver: config.DriverWhisperCPP,
			Endpoints: []config.Endpoint{{
				ID: "speech",
				Config: config.EndpointConfig{Models: map[string]string{
					"missing": "/nonexistent/missing.bin",
				}},
			}},
		}},
		Hotkeys: map[string]config.Hotkey{
			"ctrl+alt+a": {
				Transcribe: config.TranscribeStage{Model: "speech:missing"},
			},
		},
	}
	registry, err := BuildRegistry(cfg)
	if err != nil {
		t.Fatal(err)
	}

	loaded, err := registry.PreloadHotkeyModels(cfg.Hotkeys)
	if err == nil {
		t.Fatal("expected preload error")
	}
	if len(loaded) != 0 {
		t.Fatalf("loaded refs = %v, want none", loaded)
	}
}

func boolPointer(value bool) *bool {
	return &value
}
