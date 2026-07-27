package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// llamaTestConfig returns a valid config with an openai endpoint, a
// llama_cpp endpoint holding one real temp model file, and one hotkey
// using the llama model for enhance.
func llamaTestConfig(t *testing.T) *Config {
	t.Helper()
	dir := t.TempDir()
	model := filepath.Join(dir, "gemma.gguf")
	if err := os.WriteFile(model, []byte("gguf"), 0o644); err != nil {
		t.Fatal(err)
	}
	return &Config{
		Version: CurrentVersion,
		Drivers: []Driver{
			{Driver: DriverOpenAICompatible, Endpoints: []Endpoint{{
				ID:     "openai",
				Config: EndpointConfig{APIBase: "https://api.openai.com/v1", APIKey: "sk-x"},
			}}},
			{Driver: DriverLlamaCPP, Endpoints: []Endpoint{{
				ID:     "local-llm",
				Config: EndpointConfig{Models: map[string]string{"gemma": model}},
			}}},
		},
		Hotkeys: map[string]Hotkey{"ctrl+alt+space": {
			Transcribe: TranscribeStage{Model: "openai:whisper-1"},
			Enhance:    &EnhanceStage{Model: "local-llm:gemma"},
		}},
		ToggleMaxSeconds:   60,
		UnloadAfterSeconds: 300,
	}
}

func TestValidateLlamaCPPHappyPath(t *testing.T) {
	if err := Validate(llamaTestConfig(t)); err != nil {
		t.Fatalf("expected valid config, got: %v", err)
	}
}

func TestValidateLlamaCPPRequiresModels(t *testing.T) {
	cfg := llamaTestConfig(t)
	cfg.Drivers[1].Endpoints[0].Config.Models = nil
	err := Validate(cfg)
	if err == nil || !strings.Contains(err.Error(), "models") {
		t.Fatalf("expected missing-models error, got: %v", err)
	}
}

func TestValidateLlamaCPPModelFileMustExist(t *testing.T) {
	cfg := llamaTestConfig(t)
	cfg.Drivers[1].Endpoints[0].Config.Models["gemma"] = "/nonexistent/gemma.gguf"
	err := Validate(cfg)
	if err == nil || !strings.Contains(err.Error(), "file not found") {
		t.Fatalf("expected file-not-found error, got: %v", err)
	}
}

func TestValidateLlamaCPPRejectsTranscribeRef(t *testing.T) {
	cfg := llamaTestConfig(t)
	hk := cfg.Hotkeys["ctrl+alt+space"]
	hk.Transcribe.Model = "local-llm:gemma"
	cfg.Hotkeys["ctrl+alt+space"] = hk
	err := Validate(cfg)
	if err == nil || !strings.Contains(err.Error(), "chat stages only") {
		t.Fatalf("expected chat-stages-only error, got: %v", err)
	}
}

func TestValidateLlamaCPPUnknownModelName(t *testing.T) {
	cfg := llamaTestConfig(t)
	hk := cfg.Hotkeys["ctrl+alt+space"]
	hk.Enhance = &EnhanceStage{Model: "local-llm:nope"}
	cfg.Hotkeys["ctrl+alt+space"] = hk
	err := Validate(cfg)
	if err == nil || !strings.Contains(err.Error(), "no model named") {
		t.Fatalf("expected no-model-named error, got: %v", err)
	}
}
