package setup

import (
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
