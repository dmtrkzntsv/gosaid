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
		if err := flow(s); err != nil {
			if abort, ferr := confirmDiscardOnAbort(s, err); abort {
				if ferr != nil {
					fmt.Fprintf(os.Stderr, "error: %v\n", ferr)
					return 1
				}
				fmt.Println("Changes discarded.")
				return 0
			} else if !errors.Is(err, huh.ErrUserAborted) {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				return 1
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
	offerRestart()
	return nil
}
