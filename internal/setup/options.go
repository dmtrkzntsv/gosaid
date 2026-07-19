package setup

import (
	"fmt"
	"sort"

	"github.com/dmtrkzntsv/gosaid/internal/config"
)

// SuggestedCombos is the curated key-combo list for the hotkey wizard. All
// entries parse with the hotkey package on every platform ("option" and
// "alt" are aliases everywhere).
var SuggestedCombos = []string{
	"option+left", "option+right", "option+up", "option+down",
	"ctrl+alt+space", "ctrl+alt+enter",
	"option+f8", "option+f9", "option+f10",
}

// ModelOption is one selectable model reference.
type ModelOption struct {
	Label string
	Ref   string // "endpoint:model"
}

// TranscribeModelOptions lists every model the transcribe stage can use:
// each whisper_cpp model, plus the preset-suggested transcription model of
// each recognized openai_compatible endpoint.
func TranscribeModelOptions(cfg *config.Config) []ModelOption {
	var out []ModelOption
	for _, d := range cfg.Drivers {
		for _, e := range d.Endpoints {
			switch d.Driver {
			case config.DriverWhisperCPP:
				names := make([]string, 0, len(e.Config.Models))
				for name := range e.Config.Models {
					names = append(names, name)
				}
				sort.Strings(names)
				for _, name := range names {
					out = append(out, ModelOption{
						Label: fmt.Sprintf("%s (local)", name),
						Ref:   e.ID + ":" + name,
					})
				}
			case config.DriverOpenAICompatible:
				if p := PresetForAPIBase(e.Config.APIBase); p != nil && p.TranscribeModel != "" {
					out = append(out, ModelOption{
						Label: fmt.Sprintf("%s (%s)", p.TranscribeModel, e.ID),
						Ref:   e.ID + ":" + p.TranscribeModel,
					})
				}
			}
		}
	}
	return out
}

// ChatModelOptions lists the preset-suggested chat model of each recognized
// openai_compatible endpoint. whisper_cpp endpoints never appear (they are
// transcription-only).
func ChatModelOptions(cfg *config.Config) []ModelOption {
	var out []ModelOption
	for _, d := range cfg.Drivers {
		if d.Driver != config.DriverOpenAICompatible {
			continue
		}
		for _, e := range d.Endpoints {
			if p := PresetForAPIBase(e.Config.APIBase); p != nil && p.ChatModel != "" {
				out = append(out, ModelOption{
					Label: fmt.Sprintf("%s (%s)", p.ChatModel, e.ID),
					Ref:   e.ID + ":" + p.ChatModel,
				})
			}
		}
	}
	return out
}

// OpenAIEndpointIDs returns the ids of all openai_compatible endpoints,
// sorted — the free-text model path lets the user pick one and type a model
// id when no preset suggestion exists.
func OpenAIEndpointIDs(cfg *config.Config) []string {
	var out []string
	for _, d := range cfg.Drivers {
		if d.Driver != config.DriverOpenAICompatible {
			continue
		}
		for _, e := range d.Endpoints {
			out = append(out, e.ID)
		}
	}
	sort.Strings(out)
	return out
}
