package setup

import (
	"fmt"

	"charm.land/huh/v2"

	"github.com/dmtrkzntsv/gosaid/internal/config"
)

// runHub is the `gosaid setup` menu. On a first run (no usable provider) it
// chains the guided flow instead: provider → first hotkey → microphone.
func runHub(s *Session) error {
	if FirstRun(s.Cfg) {
		return runFirstRun(s)
	}
	for {
		var choice string
		if err := huh.NewForm(huh.NewGroup(
			huh.NewSelect[string]().Title("GoSaid setup").Options(
				huh.NewOption("Hotkeys", "hotkey"),
				huh.NewOption("Providers", "provider"),
				huh.NewOption("Local models", "model"),
				huh.NewOption("Default microphone", "mic"),
				huh.NewOption("Done", "done"),
			).Value(&choice),
		)).Run(); err != nil {
			return err
		}
		var err error
		switch choice {
		case "hotkey":
			err = runHotkeyFlow(s)
		case "provider":
			err = runProviderFlow(s)
		case "model":
			err = runModelFlow(s)
		case "mic":
			err = runMicFlow(s)
		case "done":
			return nil
		}
		if err != nil {
			return err
		}
	}
}

// runFirstRun is the guided chain for a fresh install. It rebuilds drivers
// and hotkeys from scratch (dropping the shipped placeholder config) and
// walks: provider → first hotkey → default microphone.
func runFirstRun(s *Session) error {
	fmt.Println("Welcome to GoSaid! Let's set up a provider, a hotkey, and your microphone.")
	ResetForFirstRun(s.Cfg)
	s.Cfg.Version = config.CurrentVersion
	if s.Cfg.ToggleMaxSeconds <= 0 {
		s.Cfg.ToggleMaxSeconds = config.DefaultToggleSeconds
	}
	if s.Cfg.InjectionMode == "" {
		s.Cfg.InjectionMode = config.InjectionModePaste
	}
	s.Dirty = true
	if err := runAddProvider(s); err != nil {
		return err
	}
	if err := runHotkeyWizard(s, nil); err != nil {
		return err
	}
	return runMicFlow(s)
}
