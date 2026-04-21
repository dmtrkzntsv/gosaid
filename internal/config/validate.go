package config

import (
	"fmt"
	"slices"
	"strings"

	"github.com/dmtrkzntsv/gosaid/internal/routing"
)

// Validate checks every rule. Combo parsing is deferred to the hotkey package
// (wired in Step 7) — at this stage we only verify the combo string is
// non-empty and uses allowed characters.
func Validate(cfg *Config) error {
	if cfg == nil {
		return fmt.Errorf("config is nil")
	}
	if len(cfg.Drivers) == 0 {
		return fmt.Errorf("at least one driver must be configured")
	}

	endpointIDs := map[string]struct{}{}
	for di, d := range cfg.Drivers {
		if d.Driver != DriverOpenAICompatible {
			return fmt.Errorf("drivers[%d]: unknown driver type %q (only %q is supported)", di, d.Driver, DriverOpenAICompatible)
		}
		if len(d.Endpoints) == 0 {
			return fmt.Errorf("drivers[%d]: at least one endpoint is required", di)
		}
		for ei, e := range d.Endpoints {
			if e.ID == "" {
				return fmt.Errorf("drivers[%d].endpoints[%d]: id is required", di, ei)
			}
			if _, dup := endpointIDs[e.ID]; dup {
				return fmt.Errorf("duplicate endpoint id %q", e.ID)
			}
			endpointIDs[e.ID] = struct{}{}
			if e.Config.APIBase == "" {
				return fmt.Errorf("endpoint %q: api_base is required", e.ID)
			}
			if e.Config.APIKey == "" {
				return fmt.Errorf("endpoint %q: api_key is required", e.ID)
			}
		}
	}

	if cfg.InjectionMode != "" && cfg.InjectionMode != InjectionModePaste {
		return fmt.Errorf("injection_mode %q is not supported (only %q)", cfg.InjectionMode, InjectionModePaste)
	}
	if cfg.ToggleMaxSeconds <= 0 {
		return fmt.Errorf("toggle_max_seconds must be > 0")
	}

	if len(cfg.Hotkeys) == 0 {
		return fmt.Errorf("at least one hotkey must be configured")
	}
	for combo, hk := range cfg.Hotkeys {
		if err := validateCombo(combo); err != nil {
			return fmt.Errorf("hotkey %q: %w", combo, err)
		}
		if err := validateHotkey(hk, endpointIDs); err != nil {
			return fmt.Errorf("hotkey %q: %w", combo, err)
		}
	}
	return nil
}

func validateHotkey(hk Hotkey, endpoints map[string]struct{}) error {
	switch hk.Mode {
	case "", ModeHold, ModeToggle:
	default:
		return fmt.Errorf("invalid mode %q (expected 'hold' or 'toggle')", hk.Mode)
	}
	if hk.Transcribe.Model == "" {
		return fmt.Errorf("transcribe.model is required")
	}
	if err := checkModelRef("transcribe.model", hk.Transcribe.Model, endpoints); err != nil {
		return err
	}
	if hk.Transcribe.OutputLanguage != "" && hk.Transcribe.OutputLanguage != "en" {
		return fmt.Errorf("transcribe.output_language must be \"en\" or unset (Whisper's native translate is English-only)")
	}
	if hk.Transcribe.InputLanguage != "" && !IsValidLanguage(hk.Transcribe.InputLanguage) {
		return fmt.Errorf("transcribe.input_language: unknown language %q", hk.Transcribe.InputLanguage)
	}
	if hk.Translate != nil {
		if hk.Translate.OutputLanguage == "" {
			return fmt.Errorf("translate.output_language is required")
		}
		if !IsValidLanguage(hk.Translate.OutputLanguage) {
			return fmt.Errorf("translate.output_language: unknown language %q", hk.Translate.OutputLanguage)
		}
		if hk.Translate.Model == "" {
			return fmt.Errorf("translate.model is required")
		}
		if err := checkModelRef("translate.model", hk.Translate.Model, endpoints); err != nil {
			return err
		}
	}
	if hk.Enhance != nil {
		if hk.Enhance.Model == "" {
			return fmt.Errorf("enhance.model is required")
		}
		if err := checkModelRef("enhance.model", hk.Enhance.Model, endpoints); err != nil {
			return err
		}
	}
	if hk.Compose != nil {
		if hk.Compose.Model == "" {
			return fmt.Errorf("compose.model is required")
		}
		if err := checkModelRef("compose.model", hk.Compose.Model, endpoints); err != nil {
			return err
		}
	}
	return nil
}

func checkModelRef(field, ref string, endpoints map[string]struct{}) error {
	m, err := routing.ParseModelRef(ref)
	if err != nil {
		return fmt.Errorf("%s: %w", field, err)
	}
	if _, ok := endpoints[m.Endpoint]; !ok {
		return fmt.Errorf("%s: unknown endpoint %q", field, m.Endpoint)
	}
	return nil
}

// validateCombo does a stub parse sufficient for Step 2. The hotkey package
// replaces this with a real parser in Step 7.
func validateCombo(combo string) error {
	if combo == "" {
		return fmt.Errorf("hotkey combo cannot be empty")
	}
	parts := strings.Split(combo, "+")
	if len(parts) < 2 {
		return fmt.Errorf("hotkey %q must include at least one modifier and a key (e.g. 'ctrl+alt+space')", combo)
	}
	if slices.Contains(parts, "") {
		return fmt.Errorf("hotkey %q has an empty segment", combo)
	}
	return nil
}
