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
			// Local models are reached through Providers → Local Whisper,
			// not as a top-level entry: they are one provider's detail.
			huh.NewSelect[string]().Title("GoSaid setup").Options(
				huh.NewOption("Providers", "provider"),
				huh.NewOption("Hotkeys", "hotkey"),
				huh.NewOption("Microphone", "mic"),
				huh.NewOption("Exit", "exit"),
			).Value(&choice),
		)).Run(); err != nil {
			return err
		}
		var err error
		switch choice {
		case "provider":
			err = runProviderFlow(s)
		case "hotkey":
			err = runHotkeyFlow(s)
		case "mic":
			err = runMicFlow(s)
		case "exit":
			return nil
		}
		// Backing out of a manager returns to this menu, not out of setup.
		if err := absorbCancel(err); err != nil {
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
	// Each step here is required, so a cancelled step ends the guided chain
	// rather than skipping ahead to a half-configured save. Run() treats the
	// resulting abort as the user leaving setup.
	if err := uncancel(runAddProvider(s)); err != nil {
		return err
	}
	if err := uncancel(runHotkeyWizard(s, nil)); err != nil {
		return err
	}
	return uncancel(runMicFlow(s))
}
