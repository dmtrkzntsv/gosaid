package drivers

import (
	"fmt"
	"sort"
	"time"

	"github.com/dmtrkzntsv/gosaid/internal/config"
	"github.com/dmtrkzntsv/gosaid/internal/routing"
)

// Registry holds the concrete Driver for each endpoint id referenced in config.
type Registry struct {
	endpoints map[string]Driver
}

type modelPreloader interface {
	Preload(model string) error
}

// BuildRegistry constructs a Registry from validated config. Returns an error
// if any endpoint uses an unsupported driver or a duplicate id slips through.
func BuildRegistry(cfg *config.Config) (*Registry, error) {
	r := &Registry{endpoints: map[string]Driver{}}
	for _, d := range cfg.Drivers {
		for _, e := range d.Endpoints {
			if _, dup := r.endpoints[e.ID]; dup {
				return nil, fmt.Errorf("duplicate endpoint id %q", e.ID)
			}
			switch d.Driver {
			case config.DriverOpenAICompatible:
				r.endpoints[e.ID] = NewOpenAICompatible(e.Config.APIBase, e.Config.APIKey)
			case config.DriverWhisperCPP:
				r.endpoints[e.ID] = NewWhisperCPP(e.Config.Models,
					time.Duration(e.Config.UnloadAfterSeconds)*time.Second)
			case config.DriverLlamaCPP:
				r.endpoints[e.ID] = NewLlamaCPP(e.Config.Models,
					time.Duration(e.Config.UnloadAfterSeconds)*time.Second)
			default:
				return nil, fmt.Errorf("unsupported driver type %q", d.Driver)
			}
		}
	}
	return r, nil
}

// Endpoint returns the driver for an endpoint id, or an error if unknown.
func (r *Registry) Endpoint(id string) (Driver, error) {
	d, ok := r.endpoints[id]
	if !ok {
		return nil, fmt.Errorf("unknown endpoint %q", id)
	}
	return d, nil
}

// PreloadHotkeyModels loads the local models referenced by configured hotkeys.
// Transcription models are loaded first, followed by models from enabled text
// stages. Hosted endpoints are skipped, and shared references are loaded once.
// The returned refs are the local models that were loaded, in load order.
func (r *Registry) PreloadHotkeyModels(hotkeys map[string]config.Hotkey) ([]string, error) {
	var speechRefs, chatRefs []string
	for _, hk := range hotkeys {
		speechRefs = append(speechRefs, hk.Transcribe.Model)
		if hk.Translate.IsEnabled() {
			chatRefs = append(chatRefs, hk.Translate.Model)
		}
		if hk.Enhance.IsEnabled() {
			chatRefs = append(chatRefs, hk.Enhance.Model)
		}
		if hk.Compose.IsEnabled() {
			chatRefs = append(chatRefs, hk.Compose.Model)
		}
	}

	speechRefs = uniqueSorted(speechRefs)
	chatRefs = uniqueSorted(chatRefs)
	seen := make(map[string]struct{}, len(speechRefs)+len(chatRefs))
	loaded := make([]string, 0, len(speechRefs)+len(chatRefs))
	for _, ref := range append(speechRefs, chatRefs...) {
		if _, ok := seen[ref]; ok {
			continue
		}
		seen[ref] = struct{}{}

		modelRef, err := routing.ParseModelRef(ref)
		if err != nil {
			return loaded, fmt.Errorf("preload model %q: %w", ref, err)
		}
		endpoint, err := r.Endpoint(modelRef.Endpoint)
		if err != nil {
			return loaded, fmt.Errorf("preload model %q: %w", ref, err)
		}
		preloader, ok := endpoint.(modelPreloader)
		if !ok {
			continue
		}
		if err := preloader.Preload(modelRef.Model); err != nil {
			return loaded, fmt.Errorf("preload model %q: %w", ref, err)
		}
		loaded = append(loaded, ref)
	}
	return loaded, nil
}

func uniqueSorted(refs []string) []string {
	seen := make(map[string]struct{}, len(refs))
	out := make([]string, 0, len(refs))
	for _, ref := range refs {
		if _, ok := seen[ref]; ok {
			continue
		}
		seen[ref] = struct{}{}
		out = append(out, ref)
	}
	sort.Strings(out)
	return out
}
