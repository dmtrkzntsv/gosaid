package setup

import (
	"errors"
	"fmt"
	"os"

	"charm.land/huh/v2"
	"golang.org/x/term"
)

const setupUsage = "usage: gosaid setup [hotkey|provider|model|mic]"

// Run is the `gosaid setup` entry point. An empty args runs the hub; a topic
// arg jumps straight to that manager and then to save.
func Run(args []string) int {
	topic := ""
	if len(args) > 0 {
		topic = args[0]
	}
	var flow func(*Session) error
	switch topic {
	case "":
		flow = runHub
	case "hotkey":
		flow = runHotkeyFlow
	case "provider":
		flow = runProviderFlow
	case "model":
		flow = runModelFlow
	case "mic":
		flow = runMicFlow
	default:
		fmt.Fprintf(os.Stderr, "unknown setup topic: %s\n%s\n", topic, setupUsage)
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
	for {
		// A top-level flow that ends on a backed-out step is the user leaving
		// setup, not an error worth printing.
		if err := uncancel(flow(s)); err != nil {
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
				// fall through to finish() below to save what we have
			}
		}
		if err := finish(s); err != nil {
			fmt.Fprintf(os.Stderr, "config not saved: %v\n", err)
			if topic == "" {
				continue // hub: back to the menu to fix it
			}
			return 1
		}
		return 0
	}
}

// errCancelStep means "the user backed out of this step" (Esc/Ctrl+C inside a
// wizard or sub-prompt). Manager loops absorb it and re-show their list;
// it must never reach Run, which would end the whole session.
var errCancelStep = errors.New("step cancelled")

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
