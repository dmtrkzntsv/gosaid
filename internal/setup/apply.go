package setup

import (
	"fmt"
	"sort"
	"strings"

	"github.com/dmtrkzntsv/gosaid/internal/config"
	"github.com/dmtrkzntsv/gosaid/internal/routing"
)

// UpsertHotkey adds or replaces a hotkey binding.
func UpsertHotkey(cfg *config.Config, combo string, hk config.Hotkey) {
	if cfg.Hotkeys == nil {
		cfg.Hotkeys = map[string]config.Hotkey{}
	}
	cfg.Hotkeys[combo] = hk
}

// DeleteHotkey removes a hotkey binding.
func DeleteHotkey(cfg *config.Config, combo string) {
	delete(cfg.Hotkeys, combo)
}

// EndpointIDInUse reports whether any driver owns an endpoint with this id.
func EndpointIDInUse(cfg *config.Config, id string) bool {
	for _, d := range cfg.Drivers {
		for _, e := range d.Endpoints {
			if e.ID == id {
				return true
			}
		}
	}
	return false
}

func validateEndpointID(cfg *config.Config, id string) error {
	if id == "" {
		return fmt.Errorf("endpoint id is required")
	}
	if strings.Contains(id, ":") {
		return fmt.Errorf("endpoint id must not contain ':' (model references are 'endpoint:model')")
	}
	if EndpointIDInUse(cfg, id) {
		return fmt.Errorf("endpoint id %q already exists", id)
	}
	return nil
}

// AddOpenAIEndpoint appends an openai_compatible endpoint, creating the
// driver block if absent.
func AddOpenAIEndpoint(cfg *config.Config, id, apiBase, apiKey string) error {
	if err := validateEndpointID(cfg, id); err != nil {
		return err
	}
	ep := config.Endpoint{ID: id, Config: config.EndpointConfig{APIBase: apiBase, APIKey: apiKey}}
	for di := range cfg.Drivers {
		if cfg.Drivers[di].Driver == config.DriverOpenAICompatible {
			cfg.Drivers[di].Endpoints = append(cfg.Drivers[di].Endpoints, ep)
			return nil
		}
	}
	cfg.Drivers = append(cfg.Drivers, config.Driver{
		Driver:    config.DriverOpenAICompatible,
		Endpoints: []config.Endpoint{ep},
	})
	return nil
}

// UpdateOpenAIEndpoint replaces api_base and api_key of an existing endpoint.
func UpdateOpenAIEndpoint(cfg *config.Config, id, apiBase, apiKey string) error {
	for di := range cfg.Drivers {
		if cfg.Drivers[di].Driver != config.DriverOpenAICompatible {
			continue
		}
		for ei := range cfg.Drivers[di].Endpoints {
			e := &cfg.Drivers[di].Endpoints[ei]
			if e.ID == id {
				e.Config.APIBase = apiBase
				e.Config.APIKey = apiKey
				return nil
			}
		}
	}
	return fmt.Errorf("no openai_compatible endpoint %q", id)
}

// DeleteEndpoint removes an endpoint from whichever driver owns it, pruning
// driver blocks left without endpoints.
func DeleteEndpoint(cfg *config.Config, id string) error {
	for di := range cfg.Drivers {
		d := &cfg.Drivers[di]
		for ei := range d.Endpoints {
			if d.Endpoints[ei].ID != id {
				continue
			}
			d.Endpoints = append(d.Endpoints[:ei], d.Endpoints[ei+1:]...)
			if len(d.Endpoints) == 0 {
				cfg.Drivers = append(cfg.Drivers[:di], cfg.Drivers[di+1:]...)
			}
			return nil
		}
	}
	return fmt.Errorf("no endpoint %q", id)
}

// stageRefs returns pointers to every model-ref field of a hotkey, keyed by
// stage kind ("transcribe" or "chat"). Used by the reassignment rewriter.
func stageRefs(hk *config.Hotkey) map[*string]string {
	refs := map[*string]string{&hk.Transcribe.Model: "transcribe"}
	if hk.Enhance != nil {
		refs[&hk.Enhance.Model] = "chat"
	}
	if hk.Translate != nil {
		refs[&hk.Translate.Model] = "chat"
	}
	if hk.Compose != nil {
		refs[&hk.Compose.Model] = "chat"
	}
	return refs
}

// HotkeysUsingEndpoint returns the sorted combos of hotkeys with any stage
// referencing the endpoint.
func HotkeysUsingEndpoint(cfg *config.Config, id string) []string {
	var out []string
	for combo, hk := range cfg.Hotkeys {
		hk := hk
		for ref := range stageRefs(&hk) {
			if m, err := routing.ParseModelRef(*ref); err == nil && m.Endpoint == id {
				out = append(out, combo)
				break
			}
		}
	}
	sort.Strings(out)
	return out
}

// HotkeysUsingModel returns the sorted combos of hotkeys whose transcribe
// stage references endpointID:model.
func HotkeysUsingModel(cfg *config.Config, endpointID, model string) []string {
	want := endpointID + ":" + model
	var out []string
	for combo, hk := range cfg.Hotkeys {
		if hk.Transcribe.Model == want {
			out = append(out, combo)
		}
	}
	sort.Strings(out)
	return out
}

// ReassignEndpoint rewrites every stage ref pointing at fromID to point at
// toID. When toID's api_base matches a preset, the stage-appropriate
// suggested model is substituted; otherwise the model name is kept.
func ReassignEndpoint(cfg *config.Config, fromID, toID string) {
	var preset *ProviderPreset
	for _, d := range cfg.Drivers {
		if d.Driver != config.DriverOpenAICompatible {
			continue
		}
		for _, e := range d.Endpoints {
			if e.ID == toID {
				preset = PresetForAPIBase(e.Config.APIBase)
			}
		}
	}
	for combo, hk := range cfg.Hotkeys {
		for ref, kind := range stageRefs(&hk) {
			m, err := routing.ParseModelRef(*ref)
			if err != nil || m.Endpoint != fromID {
				continue
			}
			model := m.Model
			if preset != nil {
				if kind == "transcribe" && preset.TranscribeModel != "" {
					model = preset.TranscribeModel
				} else if kind == "chat" && preset.ChatModel != "" {
					model = preset.ChatModel
				}
			}
			*ref = toID + ":" + model
		}
		cfg.Hotkeys[combo] = hk
	}
}

// SetDefaultMicrophone sets the global default input device ("" = system
// default).
func SetDefaultMicrophone(cfg *config.Config, name string) {
	cfg.Microphone = name
}

// FirstRun reports whether no usable provider is configured yet. The shipped
// example config (api_key "REPLACE_ME") counts as first-run.
func FirstRun(cfg *config.Config) bool {
	for _, d := range cfg.Drivers {
		for _, e := range d.Endpoints {
			switch d.Driver {
			case config.DriverWhisperCPP:
				return false
			case config.DriverOpenAICompatible:
				if e.Config.APIKey != "" && e.Config.APIKey != "REPLACE_ME" {
					return false
				}
			}
		}
	}
	return true
}

// ResetForFirstRun clears drivers and hotkeys so the guided first-run chain
// builds a clean config, keeping global settings (toggle window, sounds,
// injection mode, log level, user context).
func ResetForFirstRun(cfg *config.Config) {
	cfg.Drivers = nil
	cfg.Hotkeys = map[string]config.Hotkey{}
}

// ModelDiff is the outcome of the local-model multi-select: names to
// download+register and names to unregister.
type ModelDiff struct {
	Add    []string
	Remove []string
}

// DiffModelSelection compares the registered model names with the picker
// selection. Both output slices are sorted.
func DiffModelSelection(registered map[string]string, selected []string) ModelDiff {
	sel := map[string]bool{}
	for _, s := range selected {
		sel[s] = true
	}
	var d ModelDiff
	for _, s := range selected {
		if _, ok := registered[s]; !ok {
			d.Add = append(d.Add, s)
		}
	}
	for name := range registered {
		if !sel[name] {
			d.Remove = append(d.Remove, name)
		}
	}
	sort.Strings(d.Add)
	sort.Strings(d.Remove)
	return d
}
