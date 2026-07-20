package setup

import (
	"fmt"
	"os"
	"strings"

	"charm.land/huh/v2"

	"github.com/dmtrkzntsv/gosaid/internal/config"
	"github.com/dmtrkzntsv/gosaid/internal/models"
	"github.com/dmtrkzntsv/gosaid/internal/platform"
)

// osRemove is a seam for tests; production uses os.Remove.
var osRemove = os.Remove

// defaultLocalEndpointID is the suggested name for a local provider, and the
// one `gosaid model download` uses by default.
const defaultLocalEndpointID = "local"

// runModelFlow installs one local Whisper model as a new provider: pick a
// curated model, download it, and name the provider it is registered under.
//
// This is an install flow, not a manager: removing a model is done from the
// provider list, which already handles the hotkeys that reference it. Models
// outside the catalog are installed with `gosaid model download`.
func runModelFlow(s *Session) error {
	opts := make([]huh.Option[string], 0, len(models.Catalog)+1)
	for _, e := range models.Catalog {
		opts = append(opts, huh.NewOption(
			fmt.Sprintf("%s · %s · %s", e.Name, e.Size, e.Note), e.Name,
		))
	}
	opts = append(opts, huh.NewOption("← Back", pickBack))

	choice := ""
	if err := huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().
			Title("Local Whisper models").
			Description("Runs on-device — no API key, no network once downloaded.").
			Options(opts...).
			Height(listHeight(len(opts))).
			Value(&choice),
	)).Run(); err != nil {
		return cancelable(err)
	}
	if choice == pickBack {
		return errCancelStep
	}

	modelsDir, err := platform.ModelsDir()
	if err != nil {
		return err
	}

	file := ""
	for _, e := range models.Catalog {
		if e.Name == choice {
			file = e.File
		}
	}
	if file == "" {
		return fmt.Errorf("no catalog entry for %q", choice)
	}

	fmt.Printf("Downloading %s…\n", choice)
	dest, _, err := models.FetchModelFile(models.HuggingFaceBase, models.CatalogRepo, file, modelsDir, false)
	if err != nil {
		fmt.Printf("Download failed: %v\n", err)
		return nil
	}
	modelName := choice

	endpointID, err := askLocalProviderName(s)
	if err != nil {
		// The file is downloaded and kept; only the config entry is skipped,
		// so re-running the flow reuses it instead of downloading again.
		return err
	}
	models.Register(s.Cfg, endpointID, modelName, dest)
	s.Dirty = true
	fmt.Printf("Installed %s as %q — use it in a hotkey as %s:%s\n",
		modelName, endpointID, endpointID, modelName)
	return nil
}

// askLocalProviderName asks what to call the provider this model is
// registered under. Reusing an existing whisper_cpp endpoint id is allowed —
// that adds the model to it — but colliding with a cloud provider is not.
func askLocalProviderName(s *Session) (string, error) {
	id := defaultLocalEndpointID
	for i := 2; EndpointIDInUse(s.Cfg, id) && endpointDriver(s.Cfg, id) != config.DriverWhisperCPP; i++ {
		id = fmt.Sprintf("%s%d", defaultLocalEndpointID, i)
	}
	if err := huh.NewForm(huh.NewGroup(
		huh.NewInput().
			Title("Provider name").
			Description("Used to reference the model in hotkeys, as \"name:model\".").
			Validate(func(v string) error {
				v = strings.TrimSpace(v)
				if endpointDriver(s.Cfg, v) == config.DriverWhisperCPP {
					return nil // adding to an existing local provider
				}
				return validateEndpointID(s.Cfg, v)
			}).
			Value(&id),
	)).Run(); err != nil {
		return "", cancelable(err)
	}
	return strings.TrimSpace(id), nil
}

// requireNonEmpty is a shared huh validator.
func requireNonEmpty(what string) func(string) error {
	return func(v string) error {
		if strings.TrimSpace(v) == "" {
			return fmt.Errorf("%s is required", what)
		}
		return nil
	}
}
