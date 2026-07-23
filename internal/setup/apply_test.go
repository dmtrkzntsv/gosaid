package setup

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dmtrkzntsv/gosaid/internal/config"
)

func TestUpsertHotkey(t *testing.T) {
	cfg := &config.Config{}
	UpsertHotkey(cfg, "option+space", config.Hotkey{Mode: config.ModeHold})
	if _, ok := cfg.Hotkeys["option+space"]; !ok {
		t.Fatal("hotkey not added")
	}
	// Upsert replaces.
	UpsertHotkey(cfg, "option+space", config.Hotkey{Mode: config.ModeToggle})
	if cfg.Hotkeys["option+space"].Mode != config.ModeToggle {
		t.Error("upsert did not replace")
	}
}

func TestResetConfig(t *testing.T) {
	cfg := config.Default()
	cfg.SoundFeedback = true
	cfg.Microphone = "MacBook"
	ResetConfig(cfg)
	if len(cfg.Drivers) != 0 || len(cfg.Hotkeys) != 0 {
		t.Fatalf("drivers/hotkeys must be cleared: %+v", cfg)
	}
	// Global settings survive — reset is "start the pipeline over", not
	// "forget the user's preferences".
	if !cfg.SoundFeedback || cfg.Microphone != "MacBook" {
		t.Error("global settings must survive reset")
	}
}

func TestIsUnconfigured(t *testing.T) {
	missingModelPath := filepath.Join(t.TempDir(), "does-not-exist.gguf")

	existingModelPath := filepath.Join(t.TempDir(), "model.bin")
	if err := os.WriteFile(existingModelPath, []byte("x"), 0o600); err != nil {
		t.Fatalf("write temp model file: %v", err)
	}

	tests := []struct {
		name string
		cfg  *config.Config
		want bool
	}{
		{
			name: "empty config",
			cfg:  &config.Config{},
			want: true,
		},
		{
			name: "placeholder openai + missing local model, mimics example config",
			cfg: &config.Config{
				Drivers: []config.Driver{
					{
						Driver: config.DriverOpenAICompatible,
						Endpoints: []config.Endpoint{
							{ID: "openai", Config: config.EndpointConfig{APIBase: "https://api.openai.com/v1", APIKey: "REPLACE_ME"}},
						},
					},
					{
						Driver: config.DriverLlamaCPP,
						Endpoints: []config.Endpoint{
							{ID: "local-llm", Config: config.EndpointConfig{Models: map[string]string{"gemma": missingModelPath}}},
						},
					},
				},
			},
			want: true,
		},
		{
			name: "openai endpoint with a real api key",
			cfg: &config.Config{
				Drivers: []config.Driver{
					{
						Driver: config.DriverOpenAICompatible,
						Endpoints: []config.Endpoint{
							{ID: "openai", Config: config.EndpointConfig{APIBase: "https://api.openai.com/v1", APIKey: "sk-x"}},
						},
					},
				},
			},
			want: false,
		},
		{
			name: "whisper_cpp endpoint whose model file exists on disk",
			cfg: &config.Config{
				Drivers: []config.Driver{
					{
						Driver: config.DriverWhisperCPP,
						Endpoints: []config.Endpoint{
							{ID: "local", Config: config.EndpointConfig{Models: map[string]string{"base": existingModelPath}}},
						},
					},
				},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsUnconfigured(tt.cfg); got != tt.want {
				t.Errorf("IsUnconfigured() = %v, want %v", got, tt.want)
			}
		})
	}
}
