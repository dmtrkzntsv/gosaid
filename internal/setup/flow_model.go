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

// runModelFlow is the local Whisper model manager: pick a model to install,
// or paste a Hugging Face link. Installed models stay in the list so they can
// be removed. Loops until the user goes back.
func runModelFlow(s *Session) error {
	for {
		// Every outcome except backing out re-renders, so the list always
		// reflects what is actually registered. Backing out returns
		// errCancelStep, which ends the loop and the enclosing manager
		// absorbs.
		if err := modelPicker(s); err != nil {
			// Leaving with nothing installed is reachable by choosing Local
			// Whisper as a provider and then backing out: the endpoint can't
			// transcribe and the config won't validate. Say so here rather
			// than letting the save fail with a bare validator message.
			if len(models.RegisteredModels(s.Cfg, localEndpointID)) == 0 {
				fmt.Println("No local model installed — local transcription needs one.")
			}
			return err
		}
	}
}

// modelPicker runs one pass of the picker: install a catalog model, remove an
// installed one, or add one from a link.
func modelPicker(s *Session) error {
	registered := models.RegisteredModels(s.Cfg, localEndpointID)

	// Catalog entries first, then anything registered from a link.
	opts := make([]huh.Option[string], 0, len(models.Catalog)+3)
	inCatalog := map[string]bool{}
	for _, e := range models.Catalog {
		inCatalog[e.Name] = true
		label := fmt.Sprintf("%s · %s · %s", e.Name, e.Size, e.Note)
		if registered[e.Name] != "" {
			label = "✓ " + label
		}
		opts = append(opts, huh.NewOption(label, e.Name))
	}
	var customNames []string
	for name := range registered {
		if !inCatalog[name] {
			customNames = append(customNames, name)
		}
	}
	sort.Strings(customNames)
	for _, name := range customNames {
		opts = append(opts, huh.NewOption("✓ "+name+" · from a link", name))
	}
	opts = append(opts,
		huh.NewOption("Add a model from a Hugging Face link…", pickAdd),
		huh.NewOption("← Back", pickBack),
	)

	choice := ""
	if err := huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().
			Title("Local Whisper models").
			Description("✓ marks a model you already have. Pick one to install or remove it.").
			Options(opts...).
			Height(listHeight(len(opts))).
			Value(&choice),
	)).Run(); err != nil {
		return cancelable(err)
	}
	switch choice {
	case pickBack:
		// Backing out of the picker backs out of the manager: the caller
		// (hub or provider flow) absorbs this and re-shows its own list.
		return errCancelStep
	case pickAdd:
		modelsDir, err := platform.ModelsDir()
		if err != nil {
			return err
		}
		// Re-render afterwards so the new model shows up in the list.
		return absorbCancel(runCustomModelPrompt(s, modelsDir))
	}

	if registered[choice] != "" {
		return removeModel(s, choice)
	}
	return installModel(s, choice)
}

// installModel downloads a catalog model and registers it.
func installModel(s *Session, name string) error {
	var file string
	for _, e := range models.Catalog {
		if e.Name == name {
			file = e.File
		}
	}
	if file == "" {
		// Only reachable for a link-installed model that was since removed
		// from disk; nothing to download from the catalog.
		return fmt.Errorf("no catalog entry for %q", name)
	}
	modelsDir, err := platform.ModelsDir()
	if err != nil {
		return err
	}
	fmt.Printf("Downloading %s…\n", name)
	dest, _, err := models.FetchModelFile(models.HuggingFaceBase, models.CatalogRepo, file, modelsDir, false)
	if err != nil {
		fmt.Printf("Download failed for %s: %v\n", name, err)
		return nil
	}
	models.Register(s.Cfg, localEndpointID, name, dest)
	s.Dirty = true
	fmt.Printf("Installed %s.\n", name)
	return nil
}

// removeModel unregisters an installed model, after resolving the hotkeys
// that use it and asking whether to delete the file too.
func removeModel(s *Session, name string) error {
	if reason := RemoveModelBlocked(s.Cfg, localEndpointID, name); reason != "" {
		fmt.Printf("Keeping %q: %s\n", name, reason)
		return nil
	}
	refs := HotkeysUsingModel(s.Cfg, localEndpointID, name)
	if len(refs) >= len(s.Cfg.Hotkeys) && len(refs) > 0 {
		fmt.Printf("Keeping %q: every hotkey uses it — add another hotkey first\n", name)
		return nil
	}

	title := fmt.Sprintf("Remove %q?", name)
	if len(refs) > 0 {
		title = fmt.Sprintf("Remove %q and delete the hotkeys using it (%s)?",
			name, strings.Join(refs, ", "))
	}
	proceed := false
	deleteFile := true
	if err := huh.NewForm(huh.NewGroup(
		huh.NewConfirm().
			Title(title).
			Affirmative("Remove").Negative("Keep it").
			Value(&proceed),
		huh.NewConfirm().
			Title("Also delete the model file from disk?").
			Description("Model files are large; keeping it lets you re-add without re-downloading.").
			Value(&deleteFile),
	)).Run(); err != nil {
		return cancelable(err)
	}
	if !proceed {
		return nil
	}
	for _, combo := range refs {
		DeleteHotkey(s.Cfg, combo)
	}
	// ok=false is unreachable: name came from the same registered map.
	if path, ok := models.Unregister(s.Cfg, localEndpointID, name); ok {
		s.Dirty = true
		if deleteFile {
			// Deleted only after a successful save, so a discarded session
			// never leaves the config pointing at a file that's gone.
			s.PendingDeletes = append(s.PendingDeletes, path)
		}
	}
	fmt.Printf("Removed %s.\n", name)
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
