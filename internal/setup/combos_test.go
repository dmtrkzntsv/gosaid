package setup

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"

	"github.com/dmtrkzntsv/gosaid/internal/config"
)

func TestShortcutOptionsMarkConfiguredCombos(t *testing.T) {
	configured := map[string]config.Hotkey{
		"option+right": {},
		"option+down":  {},
	}
	opts := shortcutOptions(configured)

	if got, want := len(opts), len(SuggestedCombos)+1; got != want {
		t.Fatalf("option count = %d, want %d", got, want)
	}
	for _, opt := range opts {
		switch opt.Value {
		case "option+right", "option+down":
			if want := opt.Value + " ✓"; opt.Key != want {
				t.Errorf("configured option %q label = %q, want %q", opt.Value, opt.Key, want)
			}
		case pickTypeOwn:
			// Action rows have their own labels.
		default:
			if opt.Key != opt.Value {
				t.Errorf("available option %q unexpectedly marked as %q", opt.Value, opt.Key)
			}
		}
	}
}

func TestSuggestedCombosAreTheCuratedThree(t *testing.T) {
	want := []string{"option+left", "option+down", "option+right"}
	if !slices.Equal(SuggestedCombos, want) {
		t.Fatalf("SuggestedCombos = %v, want %v", SuggestedCombos, want)
	}
}

func TestCustomShortcutDownArrowMovesToCancel(t *testing.T) {
	value := "ctrl+alt+space"
	field := newShortcutInput(&value, customShortcutHelp)
	field.Focus()

	field.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	if !field.cancelFocused || !strings.Contains(field.View(), "> Cancel") {
		t.Fatalf("down did not focus Cancel: focused=%v view=%q", field.cancelFocused, field.View())
	}
	_, cmd := field.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if !field.canceled || cmd == nil {
		t.Fatalf("enter on Cancel: canceled=%v cmd=%v", field.canceled, cmd)
	}
}

func TestCustomShortcutEnterSubmitsInput(t *testing.T) {
	value := "ctrl+alt+space"
	field := newShortcutInput(&value, customShortcutHelp)
	field.WithKeyMap(huh.NewDefaultKeyMap())
	field.WithPosition(huhLastFieldPosition())
	field.Focus()

	_, cmd := field.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if field.canceled || cmd == nil {
		t.Fatalf("enter in input: canceled=%v cmd=%v", field.canceled, cmd)
	}
}

func huhLastFieldPosition() huh.FieldPosition {
	return huh.FieldPosition{
		Field:      0,
		FirstField: 0,
		LastField:  0,
		FirstGroup: 0,
		LastGroup:  0,
	}
}

func TestCustomShortcutHelpListsSupportedKeys(t *testing.T) {
	for _, text := range []string{
		"ctrl", "shift", "alt/option", "cmd/win",
		"a–z", "0–9", "f1–f12",
		"left", "right", "up", "down",
		"space", "tab", "enter", "esc",
	} {
		if !strings.Contains(customShortcutHelp, text) {
			t.Errorf("custom shortcut help does not mention %q", text)
		}
	}
}

func TestValidateCustomShortcut(t *testing.T) {
	configured := map[string]config.Hotkey{"ctrl+alt+space": {}}

	if got, err := validateCustomShortcut(configured, "  OPTION+F8 "); err != nil || got != "option+f8" {
		t.Fatalf("valid shortcut = %q, %v; want option+f8", got, err)
	}
	if _, err := validateCustomShortcut(configured, "ctrl+alt+space"); err == nil {
		t.Fatal("configured shortcut accepted")
	}
	if _, err := validateCustomShortcut(configured, "space"); err == nil {
		t.Fatal("shortcut without a modifier accepted")
	}
}
