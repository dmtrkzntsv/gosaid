package setup

import (
	"os"

	"github.com/dmtrkzntsv/gosaid/internal/config"
)

// UpsertHotkey adds or replaces a hotkey binding.
func UpsertHotkey(cfg *config.Config, combo string, hk config.Hotkey) {
	if cfg.Hotkeys == nil {
		cfg.Hotkeys = map[string]config.Hotkey{}
	}
	cfg.Hotkeys[combo] = hk
}

// ResetConfig clears drivers and hotkeys for a "start from scratch" run,
// keeping global settings (microphone, toggle window, sounds, injection mode,
// log level, user context). Downloaded model files on disk are untouched, so
// re-installing a model reuses its file instead of downloading again.
func ResetConfig(cfg *config.Config) {
	cfg.Drivers = nil
	cfg.Hotkeys = map[string]config.Hotkey{}
}

// IsUnconfigured reports whether the config is effectively a fresh start: no
// drivers/hotkeys, or only the shipped placeholder (an openai_compatible
// endpoint whose api_key is empty or "REPLACE_ME", with no other usable
// provider). config.Load writes the example config on a missing file, so a
// genuine first run arrives here looking "configured" — this recognizes it.
func IsUnconfigured(cfg *config.Config) bool {
	for _, d := range cfg.Drivers {
		for _, e := range d.Endpoints {
			switch d.Driver {
			case config.DriverWhisperCPP, config.DriverLlamaCPP:
				for _, p := range e.Config.Models {
					abs, err := config.ExpandPath(p)
					if err == nil {
						if _, err := os.Stat(abs); err == nil {
							return false // a real, on-disk local model exists
						}
					}
				}
			case config.DriverOpenAICompatible:
				if e.Config.APIKey != "" && e.Config.APIKey != "REPLACE_ME" {
					return false
				}
			}
		}
	}
	return true
}
