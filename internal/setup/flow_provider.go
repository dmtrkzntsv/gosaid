package setup

import (
	"fmt"
	"strings"

	"charm.land/huh/v2"

	"github.com/dmtrkzntsv/gosaid/internal/config"
)

// Picker sentinels — \x00 cannot collide with endpoint ids or combos.
const (
	pickAdd  = "\x00add"
	pickBack = "\x00back"
)

// runProviderFlow is the provider manager: list endpoints, edit/delete one,
// or add a new provider. Loops until Back.
func runProviderFlow(s *Session) error {
	for {
		var opts []huh.Option[string]
		for _, d := range s.Cfg.Drivers {
			for _, e := range d.Endpoints {
				opts = append(opts, huh.NewOption(EndpointSummary(d.Driver, e), e.ID))
			}
		}
		opts = append(opts,
			huh.NewOption("+ Add new provider", pickAdd),
			huh.NewOption("← Back", pickBack),
		)
		var choice string
		if err := huh.NewForm(huh.NewGroup(
			huh.NewSelect[string]().Title("Providers").Options(opts...).Value(&choice),
		)).Run(); err != nil {
			return err
		}
		switch choice {
		case pickBack:
			return nil
		case pickAdd:
			if err := runAddProvider(s); err != nil {
				return err
			}
		default:
			if err := runProviderActions(s, choice); err != nil {
				return err
			}
		}
	}
}

// endpointDriver returns the driver type owning an endpoint id ("" if none).
func endpointDriver(cfg *config.Config, id string) string {
	for _, d := range cfg.Drivers {
		for _, e := range d.Endpoints {
			if e.ID == id {
				return d.Driver
			}
		}
	}
	return ""
}

// runProviderActions shows Edit / Delete / Back for one endpoint.
func runProviderActions(s *Session, id string) error {
	var action string
	if err := huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().Title(id).Options(
			huh.NewOption("Edit", "edit"),
			huh.NewOption("Delete", "delete"),
			huh.NewOption("← Back", "back"),
		).Value(&action),
	)).Run(); err != nil {
		return err
	}
	switch action {
	case "edit":
		if endpointDriver(s.Cfg, id) == config.DriverWhisperCPP {
			return runModelFlow(s)
		}
		return runEditCloudProvider(s, id)
	case "delete":
		return runDeleteProvider(s, id)
	}
	return nil
}

// runEditCloudProvider updates api_base/api_key of an openai_compatible
// endpoint.
func runEditCloudProvider(s *Session, id string) error {
	var apiBase, apiKey string
	for _, d := range s.Cfg.Drivers {
		for _, e := range d.Endpoints {
			if e.ID == id {
				apiBase, apiKey = e.Config.APIBase, e.Config.APIKey
			}
		}
	}
	if err := huh.NewForm(huh.NewGroup(
		huh.NewInput().Title("API base URL").
			Validate(requireNonEmpty("api base")).Value(&apiBase),
		huh.NewInput().Title("API key").EchoMode(huh.EchoModePassword).
			Validate(requireNonEmpty("api key")).Value(&apiKey),
	)).Run(); err != nil {
		return err
	}
	if err := UpdateOpenAIEndpoint(s.Cfg, id, apiBase, apiKey); err != nil {
		return err
	}
	s.Dirty = true
	return nil
}

// runDeleteProvider deletes an endpoint, first resolving hotkeys that
// reference it: reassign to another openai_compatible endpoint when one
// exists, otherwise offer to delete the referencing hotkeys. No mutation
// happens until the final confirm.
func runDeleteProvider(s *Session, id string) error {
	if reason := DeleteEndpointBlocked(s.Cfg, id); reason != "" {
		fmt.Println("Cannot delete: " + reason)
		return nil
	}

	refs := HotkeysUsingEndpoint(s.Cfg, id)
	reassignTo := ""
	cascade := false
	if len(refs) > 0 {
		var others []string
		for _, other := range OpenAIEndpointIDs(s.Cfg) {
			if other != id {
				others = append(others, other)
			}
		}
		if len(others) > 0 {
			var opts []huh.Option[string]
			for _, o := range others {
				opts = append(opts, huh.NewOption("Reassign to "+o, o))
			}
			if err := huh.NewForm(huh.NewGroup(
				huh.NewSelect[string]().
					Title(fmt.Sprintf("Hotkeys using %q: %s", id, strings.Join(refs, ", "))).
					Description("Pick a replacement provider for them.").
					Options(opts...).Value(&reassignTo),
			)).Run(); err != nil {
				return err
			}
		} else {
			if len(refs) >= len(s.Cfg.Hotkeys) {
				fmt.Println("Cannot delete: every hotkey uses this provider — add another provider first")
				return nil
			}
			if err := huh.NewForm(huh.NewGroup(
				huh.NewConfirm().
					Title(fmt.Sprintf("Also delete the hotkeys using it (%s)?", strings.Join(refs, ", "))).
					Affirmative("Delete them").Negative("Cancel").Value(&cascade),
			)).Run(); err != nil {
				return err
			}
			if !cascade {
				return nil
			}
		}
	}

	confirmed := false
	if err := huh.NewForm(huh.NewGroup(
		huh.NewConfirm().Title(fmt.Sprintf("Delete provider %q?", id)).
			Affirmative("Delete").Negative("Cancel").Value(&confirmed),
	)).Run(); err != nil {
		return err
	}
	if !confirmed {
		return nil
	}
	if reassignTo != "" {
		ReassignEndpoint(s.Cfg, id, reassignTo)
	} else if cascade {
		for _, combo := range refs {
			DeleteHotkey(s.Cfg, combo)
		}
	}
	if err := DeleteEndpoint(s.Cfg, id); err != nil {
		return err
	}
	s.Dirty = true
	return nil
}

// runAddProvider is the preset-driven add flow. Also used by the first-run
// guided chain.
func runAddProvider(s *Session) error {
	presetKey := ""
	var presetOpts []huh.Option[string]
	for _, p := range ProviderPresets {
		presetOpts = append(presetOpts, huh.NewOption(p.Label, p.Key))
	}
	if err := huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().Title("Add a provider").Options(presetOpts...).Value(&presetKey),
	)).Run(); err != nil {
		return err
	}
	var preset ProviderPreset
	for _, p := range ProviderPresets {
		if p.Key == presetKey {
			preset = p
		}
	}
	if preset.Local {
		return runModelFlow(s)
	}

	id := preset.Key
	if preset.Custom {
		id = ""
	}
	apiBase := preset.APIBase
	var apiKey string
	fields := []huh.Field{
		huh.NewInput().Title("Endpoint id").
			Description("Short name used in model references (e.g. \"openai:whisper-1\").").
			Validate(func(v string) error {
				return validateEndpointID(s.Cfg, strings.TrimSpace(v))
			}).
			Value(&id),
	}
	if preset.Custom {
		fields = append(fields, huh.NewInput().Title("API base URL").
			Placeholder("https://api.example.com/v1").
			Validate(requireNonEmpty("api base")).Value(&apiBase))
	}
	fields = append(fields, huh.NewInput().Title("API key").EchoMode(huh.EchoModePassword).
		Validate(requireNonEmpty("api key")).Value(&apiKey))
	if err := huh.NewForm(huh.NewGroup(fields...)).Run(); err != nil {
		return err
	}
	if err := AddOpenAIEndpoint(s.Cfg, strings.TrimSpace(id), apiBase, apiKey); err != nil {
		return err
	}
	s.Dirty = true
	return nil
}
