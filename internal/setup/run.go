package setup

import (
	"errors"
	"fmt"
	"os"
	"sort"

	"charm.land/huh/v2"
	"golang.org/x/term"

	"github.com/dmtrkzntsv/gosaid/internal/config"
)

// osRemove is a seam for tests; production uses os.Remove.
var osRemove = os.Remove

const setupUsage = "usage: gosaid setup   (local setup wizard; edit config.json for cloud providers)"

// Run is the `gosaid setup` entry point: a single local-only wizard. Any
// argument is rejected — there are no sub-topics.
func Run(args []string) int {
	if len(args) > 0 {
		fmt.Fprintf(os.Stderr, "gosaid setup takes no arguments\n%s\n", setupUsage)
		return 2
	}
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		fmt.Fprintln(os.Stderr, "error: gosaid setup requires an interactive terminal")
		return 1
	}
	s, err := LoadSession()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}

	prefill, err := chooseEntry(s)
	if err != nil {
		if errors.Is(err, errCancelStep) || errors.Is(err, huh.ErrUserAborted) {
			return 0
		}
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}

	if err := uncancel(runWizard(s, prefill)); err != nil {
		if abort, ferr := confirmDiscardOnAbort(s, err); abort {
			if ferr != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", ferr)
				return 1
			}
			fmt.Println("Changes discarded.")
			return 0
		} else if !errors.Is(err, huh.ErrUserAborted) {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			if !s.Dirty || !confirmSaveAfterError() {
				return 1
			}
		}
	}
	if err := finish(s); err != nil {
		fmt.Fprintf(os.Stderr, "config not saved: %v\n", err)
		return 1
	}
	return 0
}

// chooseEntry runs the fresh-vs-existing branch, returning the answers to
// pre-fill the wizard with (nil = fresh). It may reset the config in place.
func chooseEntry(s *Session) (*HotkeyAnswers, error) {
	if len(s.Cfg.Hotkeys) == 0 {
		return nil, nil // fresh
	}
	scratch := false
	if err := huh.NewForm(huh.NewGroup(
		huh.NewConfirm().
			Title("A config already exists. Start from scratch?").
			Description("Yes clears it (including cloud providers). No lets you edit or add a hotkey.").
			Value(&scratch),
	)).Run(); err != nil {
		return nil, cancelable(err)
	}
	if scratch {
		ResetConfig(s.Cfg)
		s.Dirty = true
		return nil, nil
	}

	// "No" — pick a hotkey to edit, or add a new one.
	combos := make([]string, 0, len(s.Cfg.Hotkeys))
	for combo := range s.Cfg.Hotkeys {
		combos = append(combos, combo)
	}
	sort.Strings(combos)
	var opts []huh.Option[string]
	for _, combo := range combos {
		opts = append(opts, huh.NewOption(HotkeySummary(combo, s.Cfg.Hotkeys[combo]), combo))
	}
	const pickAddNew = "\x00add-new"
	opts = append(opts, huh.NewOption("+ Add a new hotkey", pickAddNew))
	choice := ""
	if err := huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().Title("Which hotkey?").Options(opts...).
			Height(listHeight(len(opts))).Value(&choice),
	)).Run(); err != nil {
		return nil, cancelable(err)
	}
	if choice == pickAddNew {
		return nil, nil
	}
	a := AnswersFrom(choice, s.Cfg.Hotkeys[choice])
	return &a, nil
}

// runWizard runs the eight steps. prefill != nil seeds them from an existing
// hotkey (edit path); nil starts blank (mic from the current global setting).
func runWizard(s *Session, prefill *HotkeyAnswers) error {
	var a HotkeyAnswers
	if prefill != nil {
		a = *prefill
	} else {
		a.Mode = string(config.ModeHold)
	}

	// 1. Microphone (global).
	mic, err := askMicrophone(s.Cfg.Microphone)
	if err != nil {
		return err
	}
	if mic != s.Cfg.Microphone {
		s.Cfg.Microphone = mic
		s.Dirty = true
	}

	// 2. Transcription model → a.TranscribeRef. installWhisperModel registers
	// on s.Cfg in place, so the just-installed model is visible below.
	ref, err := installWhisperModel(s)
	if err != nil {
		return err
	}
	a.TranscribeRef = ref

	// 3. Shortcut.
	combo, err := askCombo(s, a.Combo)
	if err != nil {
		return err
	}
	a.Combo = combo

	// 4. Mode.
	mode, err := askMode(a.Mode)
	if err != nil {
		return err
	}
	a.Mode = mode

	// 5-7. Stage toggles.
	if a.Enhance, err = askYesNo("Enable enhance (clean up speech)?", "", a.Enhance); err != nil {
		return err
	}
	if a.Translate, err = askYesNo("Enable translate?", "", a.Translate); err != nil {
		return err
	}
	if a.Translate {
		if a.TargetLang, err = askTargetLanguage(a.TargetLang); err != nil {
			return err
		}
	}
	if a.Compose, err = askYesNo("Enable compose (rewrite to order)?", "", a.Compose); err != nil {
		return err
	}
	if a.Compose {
		if a.Instructions, err = askInstructions(a.Instructions); err != nil {
			return err
		}
	}

	// 8. Chat model — only when a stage needs it.
	if a.NeedsChatModel() {
		chatRef, err := installChatModel(s)
		if err != nil {
			return err
		}
		a.ChatRef = chatRef
	}

	UpsertHotkey(s.Cfg, a.Combo, BuildHotkey(a))
	s.Dirty = true
	fmt.Printf("Hotkey ready: %s\n", HotkeySummary(a.Combo, s.Cfg.Hotkeys[a.Combo]))
	return nil
}

// errCancelStep means "the user backed out of this step" (Esc/Ctrl+C inside a
// wizard or sub-prompt). Manager loops absorb it and re-show their list;
// it must never reach Run, which would end the whole session.
var errCancelStep = errors.New("step cancelled")

// listHeight sizes a select/multi-select so every option is visible.
//
// huh subtracts a field's rendered title and description from the height it
// is given, and defaults that height to the options' own height — so a list
// with a description collapses, in the worst case to a single row. Budget for
// the chrome, and cap the result so a long list (the language picker) still
// scrolls instead of running off the screen.
//
// With this set per field, several fields can share one group and render on
// a single screen; huh only overrides a field's height when it exceeds the
// group's.
func listHeight(options int) int {
	const chrome, max = 4, 16
	h := options + chrome
	if h > max {
		return max
	}
	return h
}

// cancelable converts a form's user-abort into errCancelStep. Wrap every
// prompt that sits inside a manager loop so Esc backs out one level instead
// of quitting setup.
func cancelable(err error) error {
	if errors.Is(err, huh.ErrUserAborted) {
		return errCancelStep
	}
	return err
}

// absorbCancel maps errCancelStep to nil, for manager loops that want to
// continue rather than propagate a backed-out step.
func absorbCancel(err error) error {
	if errors.Is(err, errCancelStep) {
		return nil
	}
	return err
}

// uncancel turns errCancelStep back into huh.ErrUserAborted, for callers with
// no list to return to (the first-run chain). Run() then applies its normal
// abort handling — discard confirm and exit — instead of seeing an internal
// sentinel it has no rule for.
func uncancel(err error) error {
	if errors.Is(err, errCancelStep) {
		return huh.ErrUserAborted
	}
	return err
}

// confirmSaveAfterError asks whether to save a dirty session after a
// non-abort flow error. A second error or abort while asking counts as a
// decline (returns false) so the caller discards.
func confirmSaveAfterError() bool {
	save := true
	if err := huh.NewForm(huh.NewGroup(
		huh.NewConfirm().
			Title("Save the changes made so far?").
			Value(&save),
	)).Run(); err != nil {
		return false
	}
	return save
}

// confirmDiscardOnAbort handles Ctrl+C/Esc out of a form. With no unsaved
// changes it's a plain exit. With changes, ask: discarding returns
// abort=true; declining returns abort=false so the caller proceeds to save.
func confirmDiscardOnAbort(s *Session, err error) (bool, error) {
	if !errors.Is(err, huh.ErrUserAborted) {
		return false, nil
	}
	if !s.Dirty {
		return true, nil
	}
	discard := false
	cerr := huh.NewForm(huh.NewGroup(
		huh.NewConfirm().
			Title("Discard unsaved changes?").
			Affirmative("Discard").Negative("Save them").
			Value(&discard),
	)).Run()
	if cerr != nil {
		return true, nil // second abort: discard
	}
	return discard, nil
}

// finish writes the config when anything changed and tells the user how to
// apply it. Task 6 replaces the hint with an interactive restart offer.
func finish(s *Session) error {
	if !s.Dirty {
		fmt.Println("No changes.")
		return nil
	}
	if err := s.Save(); err != nil {
		return err
	}
	fmt.Printf("Saved %s\n", s.Path)
	for _, p := range s.PendingDeletes {
		if err := osRemove(p); err != nil {
			fmt.Printf("Could not delete %s: %v\n", p, err)
		}
	}
	s.PendingDeletes = nil
	offerRestart()
	return nil
}
