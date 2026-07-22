package setup

import (
	"fmt"

	"charm.land/huh/v2"

	"github.com/dmtrkzntsv/gosaid/internal/config"
	"github.com/dmtrkzntsv/gosaid/internal/models"
	"github.com/dmtrkzntsv/gosaid/internal/platform"
)

// pickBack is the sentinel value for a "← Back" option in a select. \x00
// cannot collide with a real model name or combo.
const pickBack = "\x00back"

// installWhisperModel ensures a local transcription model exists and returns
// its "speech:<name>" ref. If one is already installed it is reused without
// prompting; otherwise the user picks from the whisper catalog and it downloads.
func installWhisperModel(s *Session) (string, error) {
	return installModel(s, "whisper", config.DriverWhisperCPP, models.DefaultWhisperEndpoint,
		"Transcription model", "Runs on-device — no API key, no network once downloaded.")
}

// installChatModel ensures a local chat model exists and returns its
// "text:<name>" ref. Reused if already installed, else picked and downloaded.
func installChatModel(s *Session) (string, error) {
	return installModel(s, "chat", config.DriverLlamaCPP, models.DefaultLlamaEndpoint,
		"Chat model", "Backs enhance, translate and compose. Runs on-device.")
}

// installModel is the shared install path for one model kind.
func installModel(s *Session, kind, driver, endpointID, title, desc string) (string, error) {
	// Reuse an already-installed model of this kind — the wizard installs one
	// per endpoint, so the first registered name is the one to use.
	for name := range models.RegisteredModelsFor(s.Cfg, driver, endpointID) {
		return endpointID + ":" + name, nil
	}

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
		huh.NewSelect[string]().
			Title(title).
			Description(desc).
			Options(opts...).
			Height(listHeight(len(opts))).
			Value(&choice),
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
