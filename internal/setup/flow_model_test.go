package setup

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dmtrkzntsv/gosaid/internal/config"
)

func writeModelFixture(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte("model"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestAvailableModelSourcesIncludesLocalAndHosted(t *testing.T) {
	speechPath := writeModelFixture(t, "speech.bin")
	chatPath := writeModelFixture(t, "chat.gguf")
	cfg := &config.Config{Drivers: []config.Driver{
		{
			Driver: config.DriverWhisperCPP,
			Endpoints: []config.Endpoint{{
				ID: "speech",
				Config: config.EndpointConfig{
					Models: map[string]string{"turbo": speechPath},
				},
			}},
		},
		{
			Driver: config.DriverLlamaCPP,
			Endpoints: []config.Endpoint{{
				ID: "text",
				Config: config.EndpointConfig{
					Models: map[string]string{"gemma": chatPath},
				},
			}},
		},
		{
			Driver: config.DriverOpenAICompatible,
			Endpoints: []config.Endpoint{
				{
					ID: "openai",
					Config: config.EndpointConfig{
						APIBase: config.OpenAIAPIBase,
						APIKey:  "sk-openai",
					},
				},
				{
					ID: "openrouter",
					Config: config.EndpointConfig{
						APIBase: config.OpenRouterAPIBase,
						APIKey:  "sk-router",
					},
				},
				{
					ID: "custom",
					Config: config.EndpointConfig{
						APIBase:         "http://localhost:11434/v1",
						APIKey:          "local",
						TranscribeModel: "whisper-custom",
						ChatModel:       "chat-custom",
					},
				},
			},
		},
	}}

	stt := sourcesByEndpoint(availableModelSources(cfg, "whisper"))
	for endpoint, ref := range map[string]string{
		"speech": "speech:turbo",
		"openai": "openai:whisper-1",
		"custom": "custom:whisper-custom",
	} {
		if got := firstRef(stt[endpoint]); got != ref {
			t.Errorf("STT source %q = %q, want %q", endpoint, got, ref)
		}
	}
	if _, exists := stt["openrouter"]; exists {
		t.Fatal("OpenRouter must not be offered for transcription")
	}

	chat := sourcesByEndpoint(availableModelSources(cfg, "chat"))
	for endpoint, ref := range map[string]string{
		"text":       "text:gemma",
		"openai":     "openai:gpt-5.4-nano",
		"openrouter": "openrouter:openai/gpt-5.4-nano",
		"custom":     "custom:chat-custom",
	} {
		if got := firstRef(chat[endpoint]); got != ref {
			t.Errorf("chat source %q = %q, want %q", endpoint, got, ref)
		}
	}
}

func TestAvailableModelSourcesSkipsUnusableEndpoints(t *testing.T) {
	cfg := &config.Config{Drivers: []config.Driver{
		{
			Driver: config.DriverWhisperCPP,
			Endpoints: []config.Endpoint{{
				ID: "missing",
				Config: config.EndpointConfig{
					Models: map[string]string{"turbo": filepath.Join(t.TempDir(), "missing.bin")},
				},
			}},
		},
		{
			Driver: config.DriverOpenAICompatible,
			Endpoints: []config.Endpoint{
				{
					ID: "placeholder",
					Config: config.EndpointConfig{
						APIBase: config.OpenAIAPIBase,
						APIKey:  "REPLACE_ME",
					},
				},
				{
					ID: "unknown-model",
					Config: config.EndpointConfig{
						APIBase: "https://example.com/v1",
						APIKey:  "key",
					},
				},
			},
		},
	}}
	if got := availableModelSources(cfg, "whisper"); len(got) != 0 {
		t.Fatalf("unusable STT sources = %+v, want none", got)
	}
	if got := availableModelSources(cfg, "chat"); len(got) != 0 {
		t.Fatalf("unusable chat sources = %+v, want none", got)
	}
}

func TestAutomaticModelRefOnlyForSingleSource(t *testing.T) {
	single := []modelSource{{
		EndpointID: "speech",
		Refs:       []string{"speech:small", "speech:turbo"},
	}}
	if got, ok := automaticModelRef(single, "speech:turbo"); !ok || got != "speech:turbo" {
		t.Fatalf("current model = %q, %v", got, ok)
	}
	if got, ok := automaticModelRef(single, ""); !ok || got != "speech:small" {
		t.Fatalf("default model = %q, %v", got, ok)
	}

	multiple := append(single, modelSource{
		EndpointID: "openai",
		Refs:       []string{"openai:whisper-1"},
	})
	if got, ok := automaticModelRef(multiple, ""); ok || got != "" {
		t.Fatalf("multiple sources resolved automatically to %q", got)
	}
}

func sourcesByEndpoint(sources []modelSource) map[string]modelSource {
	result := make(map[string]modelSource, len(sources))
	for _, source := range sources {
		result[source.EndpointID] = source
	}
	return result
}

func firstRef(source modelSource) string {
	if len(source.Refs) == 0 {
		return ""
	}
	return source.Refs[0]
}
