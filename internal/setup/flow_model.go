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
			fmt.Sprintf("%s · %s · %s", e.Name, e.Size, e.Note), e.Name,
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

	// Offer the custom-model prompt as a row in the list rather than a
	// separate question, so the whole screen is one checklist.
	opts = append(opts, huh.NewOption("+ Add a model from a Hugging Face link", pickAdd))

	var selected []string
	for name := range registered {
		selected = append(selected, name)
	}
	// Each field gets its own group: sharing one group squeezes the
	// multi-select's viewport until its rows scroll out of view entirely.
	apply := true
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Local Whisper models").
				Description("Space toggles. Checked models are kept; unchecking removes them. New checks download from Hugging Face.").
				Options(opts...).
				Value(&selected),
		),
		// A multi-select can't hold a "← Back" entry without it reading as a
		// checkbox, so backing out is an explicit step of its own.
		huh.NewGroup(
			huh.NewConfirm().
				Title("Apply these model changes?").
				Affirmative("Apply").Negative("← Back").
				Value(&apply),
		),
	)
	if err := form.Run(); err != nil {
		return cancelable(err)
	}
	if !apply {
		return errCancelStep
	}

	// The custom-model row is an action, not a model to register.
	addCustom := false
	kept := selected[:0]
	for _, name := range selected {
		if name == pickAdd {
			addCustom = true
			continue
		}
		kept = append(kept, name)
	}
	selected = kept

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
				return cancelable(err)
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
			return cancelable(err)
		}
		if deleteFiles {
			s.PendingDeletes = append(s.PendingDeletes, removedPaths...)
		}
	}

	if addCustom {
		// A cancelled custom-model prompt just skips that step.
		if err := absorbCancel(runCustomModelPrompt(s, modelsDir)); err != nil {
			return err
		}
	}
	// Reached by picking Local Whisper as a provider and then checking
	// nothing: the endpoint has no models, so it isn't a usable provider and
	// the config wouldn't validate. Say so rather than letting the save fail
	// with a bare validator message later.
	if len(models.RegisteredModels(s.Cfg, localEndpointID)) == 0 {
		fmt.Println("No local models selected — local transcription needs at least one.")
	}
	return nil
}

// runCustomModelPrompt downloads and registers one model outside the catalog,
// taking the Hugging Face link the user can copy straight from the browser.
func runCustomModelPrompt(s *Session, modelsDir string) error {
	var link string
	form := huh.NewForm(huh.NewGroup(
		huh.NewInput().
			Title("Hugging Face model link").
			Description("Paste the file's page or download URL. Esc goes back.").
			Placeholder("https://huggingface.co/ggerganov/whisper.cpp/blob/main/ggml-large-v3-q5_0.bin").
			Validate(func(v string) error {
				_, _, err := models.ParseHuggingFaceURL(v)
				return err
			}).
			Value(&link),
	))
	if err := form.Run(); err != nil {
		return cancelable(err)
	}
	repo, file, err := models.ParseHuggingFaceURL(link)
	if err != nil {
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
