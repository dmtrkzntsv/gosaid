package setup

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"charm.land/huh/v2"

	"github.com/dmtrkzntsv/gosaid/internal/models"
	"github.com/dmtrkzntsv/gosaid/internal/platform"
)

// osRemove is a seam for tests; production uses os.Remove.
var osRemove = os.Remove

// localEndpointID is the whisper_cpp endpoint managed by setup; the same
// default `gosaid model download` uses.
const localEndpointID = "local"

// runModelFlow is the local Whisper model manager: a multi-select over the
// curated catalog (plus any custom-registered models), a custom-model
// prompt, and immediate download / deferred-save semantics per the spec.
func runModelFlow(s *Session) error {
	registered := models.RegisteredModels(s.Cfg, localEndpointID)

	// Catalog entries plus registered models outside the catalog.
	opts := make([]huh.Option[string], 0, len(models.Catalog)+2)
	inCatalog := map[string]bool{}
	for _, e := range models.Catalog {
		inCatalog[e.Name] = true
		opts = append(opts, huh.NewOption(
			fmt.Sprintf("%s · %s", e.Name, e.Size), e.Name,
		).Selected(registered[e.Name] != ""))
	}
	var customNames []string
	for name := range registered {
		if !inCatalog[name] {
			customNames = append(customNames, name)
		}
	}
	sort.Strings(customNames)
	for _, name := range customNames {
		opts = append(opts, huh.NewOption(name+" · custom", name).Selected(true))
	}

	var selected []string
	for name := range registered {
		selected = append(selected, name)
	}
	addCustom := false
	form := huh.NewForm(huh.NewGroup(
		huh.NewMultiSelect[string]().
			Title("Local Whisper models").
			Description("Checked models are kept; unchecking removes them. New checks download from Hugging Face.").
			Options(opts...).
			Value(&selected),
		huh.NewConfirm().
			Title("Add a custom model from Hugging Face?").
			Value(&addCustom),
	))
	if err := form.Run(); err != nil {
		return err
	}

	diff := DiffModelSelection(registered, selected)
	modelsDir, err := platform.ModelsDir()
	if err != nil {
		return err
	}

	for _, name := range diff.Add {
		var file string
		for _, e := range models.Catalog {
			if e.Name == name {
				file = e.File
			}
		}
		if file == "" {
			continue // custom models can only be unchecked, never re-added here
		}
		fmt.Printf("Downloading %s…\n", name)
		dest, _, err := models.FetchModelFile(models.HuggingFaceBase, models.CatalogRepo, file, modelsDir, false)
		if err != nil {
			fmt.Printf("Download failed for %s: %v — skipped.\n", name, err)
			continue
		}
		models.Register(s.Cfg, localEndpointID, name, dest)
		s.Dirty = true
	}

	var removedPaths []string
	for _, name := range diff.Remove {
		if reason := RemoveModelBlocked(s.Cfg, localEndpointID, name); reason != "" {
			fmt.Printf("Keeping %q: %s\n", name, reason)
			continue
		}
		if refs := HotkeysUsingModel(s.Cfg, localEndpointID, name); len(refs) > 0 {
			if len(refs) >= len(s.Cfg.Hotkeys) {
				fmt.Printf("Keeping %q: every hotkey uses it — add another hotkey first\n", name)
				continue
			}
			proceed := false
			err := huh.NewForm(huh.NewGroup(
				huh.NewConfirm().
					Title(fmt.Sprintf("Remove %q and delete the hotkeys using it (%s)?",
						name, strings.Join(refs, ", "))).
					Affirmative("Remove them").Negative("Keep it").
					Value(&proceed),
			)).Run()
			if err != nil {
				return err
			}
			if !proceed {
				continue
			}
			for _, combo := range refs {
				DeleteHotkey(s.Cfg, combo)
			}
		}
		// ok=false is unreachable: diff.Remove names come from the same registered map Unregister reads.
		if path, ok := models.Unregister(s.Cfg, localEndpointID, name); ok {
			removedPaths = append(removedPaths, path)
			s.Dirty = true
		}
	}
	if len(removedPaths) > 0 {
		deleteFiles := true
		err := huh.NewForm(huh.NewGroup(
			huh.NewConfirm().
				Title(fmt.Sprintf("Also delete %d model file(s) from disk?", len(removedPaths))).
				Description("Model files are large; keeping them lets you re-add without re-downloading.").
				Value(&deleteFiles),
		)).Run()
		if err != nil {
			return err
		}
		if deleteFiles {
			s.PendingDeletes = append(s.PendingDeletes, removedPaths...)
		}
	}

	if addCustom {
		if err := runCustomModelPrompt(s, modelsDir); err != nil {
			return err
		}
	}
	return nil
}

// runCustomModelPrompt downloads and registers one model outside the catalog.
func runCustomModelPrompt(s *Session, modelsDir string) error {
	var repo, file string
	form := huh.NewForm(huh.NewGroup(
		huh.NewInput().
			Title("Hugging Face repository").
			Placeholder("ggerganov/whisper.cpp").
			Validate(requireNonEmpty("repository")).
			Value(&repo),
		huh.NewInput().
			Title("Model file").
			Placeholder("ggml-large-v3-q5_0.bin").
			Validate(requireNonEmpty("file")).
			Value(&file),
	))
	if err := form.Run(); err != nil {
		return err
	}
	name := models.DeriveName(file)
	fmt.Printf("Downloading %s…\n", name)
	dest, _, err := models.FetchModelFile(models.HuggingFaceBase, repo, file, modelsDir, false)
	if err != nil {
		fmt.Printf("Download failed: %v\n", err)
		return nil
	}
	models.Register(s.Cfg, localEndpointID, name, dest)
	s.Dirty = true
	return nil
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
