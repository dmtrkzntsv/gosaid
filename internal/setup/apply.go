package setup

import (
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
