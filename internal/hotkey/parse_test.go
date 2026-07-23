package hotkey

import (
	"strings"
	"testing"
)

func TestParse_Valid(t *testing.T) {
	cases := []string{
		"ctrl+alt+space",
		"cmd+shift+r",
		"ctrl+alt+f1",
		"ctrl+alt+left",
	}
	for _, c := range cases {
		if _, _, err := Parse(c); err != nil {
			t.Errorf("%q rejected: %v", c, err)
		}
	}
}

func TestParse_Invalid(t *testing.T) {
	cases := []string{
		"",      // empty
		"space", // no modifier
		"ctrl+", // empty tail
		"ctrl+fictional",
		"bogus+space",
	}
	for _, c := range cases {
		if _, _, err := Parse(c); err == nil {
			t.Errorf("%q should be rejected", c)
		}
	}
}

// Locking keys can't be bound, and the rejection must say why — "unknown
// modifier capslock" reads like a typo and sends users hunting for the right
// spelling of something that will never work.
func TestParse_LockingKeysExplainThemselves(t *testing.T) {
	cases := map[string]string{
		"capslock":       "Caps Lock",
		"capslock+space": "Caps Lock",
		"ctrl+capslock":  "Caps Lock",
		"CapsLock+a":     "Caps Lock", // case-insensitive like every other combo
		"caps+space":     "Caps Lock",
		"numlock+space":  "Num Lock",
		"fn+f1":          "Fn",
	}
	for combo, want := range cases {
		_, _, err := Parse(combo)
		if err == nil {
			t.Errorf("%q should be rejected", combo)
			continue
		}
		if !strings.Contains(err.Error(), want) {
			t.Errorf("%q: error should name %q, got: %v", combo, want, err)
		}
		if strings.Contains(err.Error(), "unknown modifier") ||
			strings.Contains(err.Error(), "unknown key") {
			t.Errorf("%q: error reads like a typo instead of a limitation: %v", combo, err)
		}
		if !strings.Contains(err.Error(), "Remap") {
			t.Errorf("%q: error should point at the remap workaround, got: %v", combo, err)
		}
	}
}
