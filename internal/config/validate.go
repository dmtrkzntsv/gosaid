package config

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/dmtrkzntsv/gosaid/internal/routing"
)

// endpointInfo carries what stage validation needs to know about an endpoint.
type endpointInfo struct {
	driver string
	models map[string]string // whisper_cpp only
}

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

	endpoints := map[string]endpointInfo{}
	for di, d := range cfg.Drivers {
		switch d.Driver {
		case DriverOpenAICompatible, DriverWhisperCPP:
		default:
			return fmt.Errorf("drivers[%d]: unknown driver type %q (expected %q or %q)",
				di, d.Driver, DriverOpenAICompatible, DriverWhisperCPP)
		}
		if len(d.Endpoints) == 0 {
			return fmt.Errorf("drivers[%d]: at least one endpoint is required", di)
		}
		for ei, e := range d.Endpoints {
			if e.ID == "" {
				return fmt.Errorf("drivers[%d].endpoints[%d]: id is required", di, ei)
			}
			if _, dup := endpoints[e.ID]; dup {
				return fmt.Errorf("duplicate endpoint id %q", e.ID)
			}
			switch d.Driver {
			case DriverOpenAICompatible:
				if e.Config.APIBase == "" {
					return fmt.Errorf("endpoint %q: api_base is required", e.ID)
				}
				if e.Config.APIKey == "" {
					return fmt.Errorf("endpoint %q: api_key is required", e.ID)
				}
				if e.Config.UnloadAfterSeconds != 0 {
					return fmt.Errorf("endpoint %q: unload_after_seconds only applies to whisper_cpp endpoints", e.ID)
				}
			case DriverWhisperCPP:
				if len(e.Config.Models) == 0 {
					return fmt.Errorf("endpoint %q: a non-empty models map is required for whisper_cpp", e.ID)
				}
				if e.Config.UnloadAfterSeconds < 0 {
					return fmt.Errorf("endpoint %q: unload_after_seconds must not be negative", e.ID)
				}
				for name, p := range e.Config.Models {
					abs, err := ExpandPath(p)
					if err != nil {
						return fmt.Errorf("endpoint %q: model %q: %w", e.ID, name, err)
					}
					fi, err := os.Stat(abs)
					if err != nil {
						return fmt.Errorf("endpoint %q: model %q: file not found: %s", e.ID, name, abs)
					}
					if fi.IsDir() {
						return fmt.Errorf("endpoint %q: model %q: path is a directory, not a model file: %s", e.ID, name, abs)
					}
				}
			}
			endpoints[e.ID] = endpointInfo{driver: d.Driver, models: e.Config.Models}
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
		if err := validateHotkey(hk, endpoints); err != nil {
			return fmt.Errorf("hotkey %q: %w", combo, err)
		}
	}
	return nil
}

func validateHotkey(hk Hotkey, endpoints map[string]endpointInfo) error {
	switch hk.Mode {
	case "", ModeHold, ModeToggle:
	default:
		return fmt.Errorf("invalid mode %q (expected 'hold' or 'toggle')", hk.Mode)
	}
	if hk.Transcribe.Model == "" {
		return fmt.Errorf("transcribe.model is required")
	}
	if err := checkModelRef("transcribe.model", hk.Transcribe.Model, endpoints, false); err != nil {
		return err
	}
	if hk.Transcribe.OutputLanguage != "" && hk.Transcribe.OutputLanguage != "en" {
		return fmt.Errorf("transcribe.output_language must be \"en\" or unset (Whisper's native translate is English-only)")
	}
	if hk.Transcribe.InputLanguage != "" && !IsValidLanguage(hk.Transcribe.InputLanguage) {
		return fmt.Errorf("transcribe.input_language: unknown language %q", hk.Transcribe.InputLanguage)
	}
	if hk.Translate.IsEnabled() {
		if hk.Translate.OutputLanguage == "" {
			return fmt.Errorf("translate.output_language is required")
		}
		if !IsValidLanguage(hk.Translate.OutputLanguage) {
			return fmt.Errorf("translate.output_language: unknown language %q", hk.Translate.OutputLanguage)
		}
		if hk.Translate.Model == "" {
			return fmt.Errorf("translate.model is required")
		}
		if err := checkModelRef("translate.model", hk.Translate.Model, endpoints, true); err != nil {
			return err
		}
	}
	if hk.Enhance.IsEnabled() {
		if hk.Enhance.Model == "" {
			return fmt.Errorf("enhance.model is required")
		}
		if err := checkModelRef("enhance.model", hk.Enhance.Model, endpoints, true); err != nil {
			return err
		}
	}
	if hk.Compose.IsEnabled() {
		if hk.Compose.Model == "" {
			return fmt.Errorf("compose.model is required")
		}
		if err := checkModelRef("compose.model", hk.Compose.Model, endpoints, true); err != nil {
			return err
		}
	}
	return nil
}

func checkModelRef(field, ref string, endpoints map[string]endpointInfo, chatStage bool) error {
	m, err := routing.ParseModelRef(ref)
	if err != nil {
		return fmt.Errorf("%s: %w", field, err)
	}
	info, ok := endpoints[m.Endpoint]
	if !ok {
		return fmt.Errorf("%s: unknown endpoint %q", field, m.Endpoint)
	}
	if info.driver == DriverWhisperCPP {
		if chatStage {
			return fmt.Errorf("%s: endpoint %q is whisper_cpp, which supports transcription only", field, m.Endpoint)
		}
		if _, ok := info.models[m.Model]; !ok {
			return fmt.Errorf("%s: endpoint %q has no model named %q", field, m.Endpoint, m.Model)
		}
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

// ExpandPath expands a leading "~/" (or bare "~") to the user's home
// directory. Other paths pass through unchanged.
func ExpandPath(p string) (string, error) {
	if p != "~" && !strings.HasPrefix(p, "~/") {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("expand %q: %w", p, err)
	}
	return filepath.Join(home, strings.TrimPrefix(strings.TrimPrefix(p, "~"), "/")), nil
}
