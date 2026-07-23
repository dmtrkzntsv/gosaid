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

// askCombo picks a key combo for a NEW hotkey: the curated list minus
// already-bound combos, or free text validated by the hotkey parser. The edit
// path never calls this — an existing hotkey's combo is fixed.
func askCombo(s *Session) (string, error) {
	var opts []huh.Option[string]
	for _, c := range SuggestedCombos {
		if _, bound := s.Cfg.Hotkeys[c]; !bound {
			opts = append(opts, huh.NewOption(c, c))
		}
	}
	opts = append(opts,
		huh.NewOption("Type your own…", pickTypeOwn),
		huh.NewOption("← Back", pickBack),
	)
	choice := ""
	if err := huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().Title("Shortcut").Options(opts...).
			Height(listHeight(len(opts))).Value(&choice),
	)).Run(); err != nil {
		return "", cancelable(err)
	}
	if choice == pickBack {
		return "", errCancelStep
	}
	if choice != pickTypeOwn {
		return choice, nil
	}
	combo := ""
	var lockingSeen *hotkey.LockingKeyError
	if err := huh.NewForm(huh.NewGroup(
		huh.NewInput().Title("Shortcut").
			Description("Esc goes back.").
			Placeholder("ctrl+alt+space").
			Validate(func(v string) error {
				v = strings.ToLower(strings.TrimSpace(v))
				if _, _, err := hotkey.Parse(v); err != nil {
					var locking *hotkey.LockingKeyError
					if errors.As(err, &locking) {
						lockingSeen = locking
						return errors.New(locking.Short())
					}
					return err
				}
				if _, bound := s.Cfg.Hotkeys[v]; bound {
					return fmt.Errorf("%q is already bound", v)
				}
				return nil
			}).
			Value(&combo),
	)).Run(); err != nil {
		if lockingSeen != nil {
			fmt.Println(lockingSeen.Error())
		}
		return "", cancelable(err)
	}
	return strings.ToLower(strings.TrimSpace(combo)), nil
}

// askMode picks hold vs toggle.
func askMode(current string) (string, error) {
	if current == "" {
		current = string(config.ModeHold)
	}
	choice := current
	if err := huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().Title("Mode").
			Description("Hold: record while pressed. Toggle: press to start, press to stop.").
			Options(
				huh.NewOption("Hold (push-to-talk)", string(config.ModeHold)),
				huh.NewOption("Toggle", string(config.ModeToggle)),
				huh.NewOption("← Back", pickBack),
			).
			Height(listHeight(3)).
			Value(&choice),
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
	sel := huh.NewSelect[string]().Title(title).Options(opts...).
		Height(listHeight(len(opts))).Value(&choice)
	if desc != "" {
		sel = sel.Description(desc)
	}
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
		huh.NewSelect[string]().Title("Translate to").Options(opts...).
			Height(listHeight(len(opts))).Value(&choice),
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
