package setup

import (
	"errors"
	"fmt"
	"strings"

	"charm.land/huh/v2"

	"github.com/dmtrkzntsv/gosaid/internal/config"
	"github.com/dmtrkzntsv/gosaid/internal/hotkey"
)

const pickTypeOwn = "\x00type-own"

const customShortcutHelp = `Use at least one modifier and one key.
Modifiers: ctrl · shift · alt/option · cmd/win
Keys:
  a–z · 0–9 · f1–f12
  left · right · up · down
  space · tab · enter · esc`

// askCombo picks a key combo for a NEW hotkey. Configured suggestions remain
// visible and marked, but validation prevents selecting them. The edit path
// never calls this — an existing hotkey's combo is fixed.
func askCombo(s *Session) (string, error) {
	for {
		opts := shortcutOptions(s.Cfg.Hotkeys)
		choice := ""
		sel := optionSelect("Shortcut", "", opts, &choice).
			Validate(func(combo string) error {
				if _, configured := s.Cfg.Hotkeys[combo]; configured {
					return fmt.Errorf("%s is already configured", combo)
				}
				return nil
			})
		if err := huh.NewForm(huh.NewGroup(sel)).Run(); err != nil {
			return "", cancelable(err)
		}
		switch choice {
		case pickTypeOwn:
			combo, canceled, err := askCustomShortcut(s.Cfg.Hotkeys)
			if err != nil {
				return "", err
			}
			if canceled {
				continue
			}
			return combo, nil
		default:
			return choice, nil
		}
	}
}

// askCustomShortcut collects a custom key combination. The explicit Cancel
// button returns canceled=true so askCombo can re-open its suggested list.
func askCustomShortcut(configured map[string]config.Hotkey) (string, bool, error) {
	combo := ""
	validationMessage := ""
	for {
		description := customShortcutHelp + "\n"
		if validationMessage != "" {
			description += "\n" + validationMessage
		}
		field := newShortcutInput(&combo, description)
		if err := huh.NewForm(huh.NewGroup(field)).Run(); err != nil {
			return "", false, cancelable(err)
		}
		if field.canceled {
			return "", true, nil
		}
		normalized, err := validateCustomShortcut(configured, combo)
		if err == nil {
			return normalized, false, nil
		}
		validationMessage = err.Error()
	}
}

func validateCustomShortcut(configured map[string]config.Hotkey, raw string) (string, error) {
	combo := strings.ToLower(strings.TrimSpace(raw))
	if _, _, err := hotkey.Parse(combo); err != nil {
		var locking *hotkey.LockingKeyError
		if errors.As(err, &locking) {
			return "", errors.New(locking.Short())
		}
		return "", err
	}
	if _, bound := configured[combo]; bound {
		return "", fmt.Errorf("%q is already bound", combo)
	}
	return combo, nil
}

// askMode picks hold vs toggle.
func askMode(current string) (string, error) {
	if current == "" {
		current = string(config.ModeHold)
	}
	choice := current
	opts := []huh.Option[string]{
		huh.NewOption("Hold (push-to-talk)", string(config.ModeHold)),
		huh.NewOption("Toggle", string(config.ModeToggle)),
		huh.NewOption("← Back", pickBack),
	}
	if err := huh.NewForm(huh.NewGroup(
		optionSelect(
			"Mode",
			"Hold: record while pressed. Toggle: press to start, press to stop.",
			opts,
			&choice,
		),
	)).Run(); err != nil {
		return "", cancelable(err)
	}
	if choice == pickBack {
		return "", errCancelStep
	}
	return choice, nil
}

// askYesNo asks a stage enable question as a vertical Yes/No list (rather than
// a confirm's side-by-side buttons), matching the other wizard steps. Esc or
// "← Back" backs out.
func askYesNo(title, desc string, current bool) (bool, error) {
	choice := "no"
	if current {
		choice = "yes"
	}
	opts := []huh.Option[string]{
		huh.NewOption("Yes", "yes"),
		huh.NewOption("No", "no"),
		huh.NewOption("← Back", pickBack),
	}
	sel := optionSelect(title, desc, opts, &choice)
	if err := huh.NewForm(huh.NewGroup(sel)).Run(); err != nil {
		return false, cancelable(err)
	}
	if choice == pickBack {
		return false, errCancelStep
	}
	return choice == "yes", nil
}

// askTargetLanguage picks the translate output language.
func askTargetLanguage(current string) (string, error) {
	var opts []huh.Option[string]
	for _, code := range config.Languages() {
		opts = append(opts, huh.NewOption(
			fmt.Sprintf("%s (%s)", config.LanguageName(code), code), code))
	}
	opts = append(opts, huh.NewOption("← Back", pickBack))
	choice := current
	if err := huh.NewForm(huh.NewGroup(
		optionSelect("Translate to", "", opts, &choice),
	)).Run(); err != nil {
		return "", cancelable(err)
	}
	if choice == pickBack {
		return "", errCancelStep
	}
	return choice, nil
}

// askInstructions collects the compose instructions.
func askInstructions(current string) (string, error) {
	v := current
	if err := huh.NewForm(huh.NewGroup(
		huh.NewText().Title("Compose instructions").
			Description("e.g. \"Write in a formal, business-email register.\"").
			Value(&v),
	)).Run(); err != nil {
		return "", cancelable(err)
	}
	return v, nil
}
