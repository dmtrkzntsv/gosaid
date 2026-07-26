package setup

import (
	"fmt"
	"os"
	"sort"

	"charm.land/huh/v2"

	"github.com/dmtrkzntsv/gosaid/internal/config"
	"github.com/dmtrkzntsv/gosaid/internal/models"
	"github.com/dmtrkzntsv/gosaid/internal/platform"
)

// pickBack is the sentinel value for a "← Back" option in a select. \x00
// cannot collide with a real model name or combo.
const pickBack = "\x00back"

// installWhisperModel resolves a configured local or hosted transcription
// source. One source is automatic; multiple sources produce a model picker.
// With no usable source, setup offers to install a local model.
func installWhisperModel(s *Session, current string) (string, error) {
	if ref, resolved, err := selectAvailableModel(
		"Transcription model",
		"Choose which configured speech-to-text driver this hotkey should use.",
		availableModelSources(s.Cfg, "whisper"),
		current,
	); resolved || err != nil {
		return ref, err
	}
	return installModel(s, "whisper", config.DriverWhisperCPP, models.DefaultWhisperEndpoint,
		"Transcription model", "Runs on-device — no API key, no network once downloaded.")
}

// installChatModel resolves the model shared by enhance, translate and
// compose. One local or hosted source is automatic; multiple sources produce
// a picker. With none, setup offers to install a local chat model.
func installChatModel(s *Session, current string) (string, error) {
	if ref, resolved, err := selectAvailableModel(
		"Model for enhance, translate and compose",
		"Choose which configured LLM driver should run the enabled text stages.",
		availableModelSources(s.Cfg, "chat"),
		current,
	); resolved || err != nil {
		return ref, err
	}
	return installModel(s, "chat", config.DriverLlamaCPP, models.DefaultLlamaEndpoint,
		"Chat model", "Backs enhance, translate and compose. Runs on-device.")
}

type modelSource struct {
	EndpointID string
	Label      string
	Refs       []string
}

// availableModelSources returns one source per usable endpoint. A source may
// contain several local model refs, but the number of sources—not the number
// of model files—controls whether setup asks. Hosted sources use the explicit
// endpoint model names, falling back to known-provider defaults.
func availableModelSources(cfg *config.Config, kind string) []modelSource {
	var sources []modelSource
	for _, driver := range cfg.Drivers {
		for _, endpoint := range driver.Endpoints {
			var source modelSource
			switch driver.Driver {
			case config.DriverWhisperCPP:
				if kind != "whisper" {
					continue
				}
				source = localModelSource(endpoint, "Local Whisper")
			case config.DriverLlamaCPP:
				if kind != "chat" {
					continue
				}
				source = localModelSource(endpoint, "Local Llama")
			case config.DriverOpenAICompatible:
				if endpoint.Config.APIKey == "" || endpoint.Config.APIKey == "REPLACE_ME" {
					continue
				}
				defaults, known := config.DetectHostedProvider(endpoint.Config.APIBase)
				model := endpoint.Config.TranscribeModel
				if kind == "chat" {
					model = endpoint.Config.ChatModel
				}
				if model == "" && known {
					if kind == "chat" {
						model = defaults.ChatModel
					} else {
						model = defaults.TranscribeModel
					}
				}
				if model == "" {
					continue
				}
				label := "OpenAI-compatible"
				if known {
					label = defaults.Label
				}
				source = modelSource{
					EndpointID: endpoint.ID,
					Label:      label,
					Refs:       []string{endpoint.ID + ":" + model},
				}
			default:
				continue
			}
			if len(source.Refs) > 0 {
				sources = append(sources, source)
			}
		}
	}
	return sources
}

func localModelSource(endpoint config.Endpoint, label string) modelSource {
	var names []string
	for name, path := range endpoint.Config.Models {
		expanded, err := config.ExpandPath(path)
		if err != nil {
			continue
		}
		info, err := os.Stat(expanded)
		if err != nil || info.IsDir() {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	refs := make([]string, 0, len(names))
	for _, name := range names {
		refs = append(refs, endpoint.ID+":"+name)
	}
	return modelSource{EndpointID: endpoint.ID, Label: label, Refs: refs}
}

// automaticModelRef implements the "one driver means no question" rule.
// Editing preserves the current model when it belongs to that sole source.
func automaticModelRef(sources []modelSource, current string) (string, bool) {
	if len(sources) != 1 || len(sources[0].Refs) == 0 {
		return "", false
	}
	for _, ref := range sources[0].Refs {
		if ref == current {
			return current, true
		}
	}
	return sources[0].Refs[0], true
}

func selectAvailableModel(title, description string, sources []modelSource, current string) (string, bool, error) {
	if ref, ok := automaticModelRef(sources, current); ok {
		return ref, true, nil
	}
	if len(sources) == 0 {
		return "", false, nil
	}

	var opts []huh.Option[string]
	available := map[string]bool{}
	for _, source := range sources {
		for _, ref := range source.Refs {
			opts = append(opts, huh.NewOption(
				fmt.Sprintf("%s · %s", ref, source.Label),
				ref,
			))
			available[ref] = true
		}
	}
	opts = append(opts, huh.NewOption("← Back", pickBack))
	choice := current
	if !available[choice] {
		choice = sources[0].Refs[0]
	}
	if err := huh.NewForm(huh.NewGroup(
		optionSelect(title, description, opts, &choice),
	)).Run(); err != nil {
		return "", true, cancelable(err)
	}
	if choice == pickBack {
		return "", true, errCancelStep
	}
	return choice, true, nil
}

// installModel is the shared install path for one model kind.
func installModel(s *Session, kind, driver, endpointID, title, desc string) (string, error) {
	var opts []huh.Option[string]
	for _, e := range models.Catalog {
		if e.Kind != kind {
			continue
		}
		opts = append(opts, huh.NewOption(
			fmt.Sprintf("%s · %s · %s", e.Name, e.Size, e.Note), e.Name,
		))
	}
	opts = append(opts, huh.NewOption("← Back", pickBack))

	choice := ""
	if err := huh.NewForm(huh.NewGroup(
		optionSelect(title, desc, opts, &choice),
	)).Run(); err != nil {
		return "", cancelable(err)
	}
	if choice == pickBack {
		return "", errCancelStep
	}

	var entry models.CatalogEntry
	for _, e := range models.Catalog {
		if e.Name == choice {
			entry = e
		}
	}

	modelsDir, err := platform.ModelsDir()
	if err != nil {
		return "", err
	}
	fmt.Printf("Downloading %s…\n", entry.Name)
	// FetchModelFile reuses an on-disk file (no re-download); RegisterFor
	// mutates s.Cfg, which finish() saves once at the end.
	dest, _, err := models.FetchModelFile(models.HuggingFaceBase, models.CatalogRepoFor(entry), entry.File, modelsDir, false)
	if err != nil {
		return "", fmt.Errorf("download %s: %w", entry.Name, err)
	}
	models.RegisterFor(s.Cfg, driver, endpointID, entry.Name, dest)
	s.Dirty = true
	return endpointID + ":" + entry.Name, nil
}
