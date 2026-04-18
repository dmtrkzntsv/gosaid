package hotkey

import "testing"

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
		"",           // empty
		"space",      // no modifier
		"ctrl+",      // empty tail
		"ctrl+fictional",
		"bogus+space",
	}
	for _, c := range cases {
		if _, _, err := Parse(c); err == nil {
			t.Errorf("%q should be rejected", c)
		}
	}
}
