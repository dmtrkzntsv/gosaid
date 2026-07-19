package setup

import (
	"fmt"
	"sort"
	"strings"

	"charm.land/huh/v2"

	"github.com/dmtrkzntsv/gosaid/internal/config"
	"github.com/dmtrkzntsv/gosaid/internal/hotkey"
)

const pickTypeOwn = "\x00type-own"

// runHotkeyFlow is the hotkey manager: list bindings, edit/delete one, or
// run the add wizard. Loops until Back.
func runHotkeyFlow(s *Session) error {
	if len(s.Cfg.Drivers) == 0 {
		fmt.Println("No providers configured yet — add one first (gosaid setup provider).")
		return nil
	}
	for {
		combos := make([]string, 0, len(s.Cfg.Hotkeys))
		for combo := range s.Cfg.Hotkeys {
			combos = append(combos, combo)
		}
		sort.Strings(combos)
		var opts []huh.Option[string]
		for _, combo := range combos {
			opts = append(opts, huh.NewOption(HotkeySummary(combo, s.Cfg.Hotkeys[combo]), combo))
		}
		opts = append(opts,
			huh.NewOption("+ Add new hotkey", pickAdd),
			huh.NewOption("← Back", pickBack),
		)
		var choice string
		if err := huh.NewForm(huh.NewGroup(
			huh.NewSelect[string]().Title("Hotkeys").Options(opts...).Value(&choice),
		)).Run(); err != nil {
			return cancelable(err)
		}
		switch choice {
		case pickBack:
			return nil
		case pickAdd:
			// A cancelled wizard returns to this list, not out of setup.
			if err := absorbCancel(runHotkeyWizard(s, nil)); err != nil {
				return err
			}
		default:
			if err := absorbCancel(runHotkeyActions(s, choice)); err != nil {
				return err
			}
		}
	}
}

// runHotkeyActions shows Edit / Delete / Back for one binding.
func runHotkeyActions(s *Session, combo string) error {
	var action string
	if err := huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().Title(HotkeySummary(combo, s.Cfg.Hotkeys[combo])).Options(
			huh.NewOption("Edit", "edit"),
			huh.NewOption("Delete", "delete"),
			huh.NewOption("← Back", "back"),
		).Value(&action),
	)).Run(); err != nil {
		return cancelable(err)
	}
	switch action {
	case "edit":
		a := AnswersFrom(combo, s.Cfg.Hotkeys[combo])
		return runHotkeyWizard(s, &a)
	case "delete":
		if reason := DeleteHotkeyBlocked(s.Cfg, combo); reason != "" {
			fmt.Println("Cannot delete: " + reason)
			return nil
		}
		confirmed := false
		if err := huh.NewForm(huh.NewGroup(
			huh.NewConfirm().Title(fmt.Sprintf("Delete hotkey %q?", combo)).
				Affirmative("Delete").Negative("Cancel").Value(&confirmed),
		)).Run(); err != nil {
			return cancelable(err)
		}
		if confirmed {
			DeleteHotkey(s.Cfg, combo)
			s.Dirty = true
		}
	}
	return nil
}

// askCombo picks a key combo: curated list (minus already-bound combos) or
// free text validated by the hotkey parser.
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
	var choice string
	if err := huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().Title("Key combo").Options(opts...).Value(&choice),
	)).Run(); err != nil {
		return "", cancelable(err)
	}
	if choice == pickBack {
		return "", errCancelStep
	}
	if choice != pickTypeOwn {
		return choice, nil
	}
	var combo string
	if err := huh.NewForm(huh.NewGroup(
		huh.NewInput().Title("Key combo").
			Description("Esc goes back to the hotkey list.").
			Placeholder("ctrl+alt+space").
			Validate(func(v string) error {
				v = strings.ToLower(strings.TrimSpace(v))
				if _, _, err := hotkey.Parse(v); err != nil {
					return err
				}
				if _, bound := s.Cfg.Hotkeys[v]; bound {
					return fmt.Errorf("%q is already bound", v)
				}
				return nil
			}).
			Value(&combo),
	)).Run(); err != nil {
		return "", cancelable(err)
	}
	return strings.ToLower(strings.TrimSpace(combo)), nil
}

// askModelRef resolves one stage's model: auto-pick a single option, select
// among several, or (for stages with no preset suggestion) pick an endpoint
// and type a model id.
func askModelRef(s *Session, title string, options []ModelOption) (string, error) {
	if len(options) == 1 {
		return options[0].Ref, nil
	}
	if len(options) > 1 {
		var choice string
		err := huh.NewForm(huh.NewGroup(
			huh.NewSelect[string]().Title(title).Options(func() []huh.Option[string] {
				var opts []huh.Option[string]
				for _, o := range options {
					opts = append(opts, huh.NewOption(o.Label, o.Ref))
				}
				return opts
			}()...).Value(&choice),
		)).Run()
		return choice, err
	}
	// No suggestions — endpoint + free-text model id.
	ids := OpenAIEndpointIDs(s.Cfg)
	if len(ids) == 0 {
		return "", fmt.Errorf("no endpoint can serve %q — add a provider first", title)
	}
	endpointID := ids[0]
	var fields []huh.Field
	if len(ids) > 1 {
		var opts []huh.Option[string]
		for _, id := range ids {
			opts = append(opts, huh.NewOption(id, id))
		}
		fields = append(fields, huh.NewSelect[string]().Title(title+" — endpoint").Options(opts...).Value(&endpointID))
	}
	var model string
	fields = append(fields, huh.NewInput().Title(title+" — model id").
		Validate(requireNonEmpty("model id")).Value(&model))
	if err := huh.NewForm(huh.NewGroup(fields...)).Run(); err != nil {
		return "", cancelable(err)
	}
	return endpointID + ":" + strings.TrimSpace(model), nil
}

// runHotkeyWizard runs the recipe-first add/edit wizard. existing == nil
// adds a new binding; otherwise edits (combo unchanged).
func runHotkeyWizard(s *Session, existing *HotkeyAnswers) error {
	var a HotkeyAnswers
	if existing != nil {
		a = *existing
	} else {
		a.Mode = string(config.ModeHold)
		combo, err := askCombo(s)
		if err != nil {
			return err
		}
		a.Combo = combo
	}

	chatAvailable := len(ChatModelOptions(s.Cfg)) > 0 || len(OpenAIEndpointIDs(s.Cfg)) > 0
	recipeOpts := []huh.Option[string]{
		huh.NewOption("Just transcribe", RecipeTranscribe),
	}
	desc := "What should this hotkey do with your speech?"
	if chatAvailable {
		recipeOpts = append(recipeOpts,
			huh.NewOption("Transcribe + clean up", RecipeCleanup),
			huh.NewOption("Translate to another language", RecipeTranslate),
			huh.NewOption("Compose (rewrite with instructions)", RecipeCompose),
		)
	} else {
		desc += " (Clean up, translate and compose need a cloud provider — add one first.)"
	}
	if err := huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().Title("Recipe").Description(desc).
			Options(recipeOpts...).Value(&a.Recipe),
	)).Run(); err != nil {
		return cancelable(err)
	}

	ref, err := askModelRef(s, "Transcription model", TranscribeModelOptions(s.Cfg))
	if err != nil {
		return err
	}
	a.TranscribeRef = ref

	if a.Recipe != RecipeTranscribe {
		ref, err := askModelRef(s, "Language model", ChatModelOptions(s.Cfg))
		if err != nil {
			return err
		}
		a.ChatRef = ref
	}
	if a.Recipe == RecipeTranslate {
		var langOpts []huh.Option[string]
		for _, code := range config.Languages() {
			langOpts = append(langOpts, huh.NewOption(
				fmt.Sprintf("%s (%s)", config.LanguageName(code), code), code))
		}
		if err := huh.NewForm(huh.NewGroup(
			huh.NewSelect[string]().Title("Translate to").Options(langOpts...).Value(&a.TargetLang),
		)).Run(); err != nil {
			return cancelable(err)
		}
	}
	if a.Recipe == RecipeCompose {
		if err := huh.NewForm(huh.NewGroup(
			huh.NewText().Title("Compose instructions").
				Description("e.g. \"Write in a formal, business-email register.\"").
				Value(&a.Instructions),
		)).Run(); err != nil {
			return cancelable(err)
		}
	}

	advanced := false
	if err := huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().Title("Mode").
			Description("Hold: record while pressed. Toggle: press to start, press to stop.").
			Options(
				huh.NewOption("Hold (push-to-talk)", string(config.ModeHold)),
				huh.NewOption("Toggle", string(config.ModeToggle)),
			).Value(&a.Mode),
		huh.NewConfirm().Title("Set a hotkey-specific microphone?").Value(&advanced),
	)).Run(); err != nil {
		return cancelable(err)
	}
	if advanced {
		opts, err := micOptions()
		if err != nil {
			return err
		}
		if err := huh.NewForm(huh.NewGroup(
			huh.NewSelect[string]().Title("Microphone for this hotkey").
				Description("Overrides the global default for this hotkey only.").
				Options(opts...).Value(&a.Microphone),
		)).Run(); err != nil {
			return cancelable(err)
		}
	}

	UpsertHotkey(s.Cfg, a.Combo, BuildHotkey(a))
	s.Dirty = true
	fmt.Printf("Hotkey saved: %s\n", HotkeySummary(a.Combo, s.Cfg.Hotkeys[a.Combo]))
	return nil
}
