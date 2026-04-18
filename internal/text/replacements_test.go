package text

import "testing"

func TestApply(t *testing.T) {
	cases := []struct {
		name  string
		in    string
		rules map[string]string
		want  string
	}{
		{
			"simple case-insensitive",
			"Say New Line here",
			map[string]string{"new line": "\n"},
			"Say \n here",
		},
		{
			"multi-word wins over shorter",
			"new line and new paragraph",
			map[string]string{"new": "X", "new line": "NL", "new paragraph": "NP"},
			"NL and NP",
		},
		{
			"word boundary prevents partial match",
			"airline tickets",
			map[string]string{"line": "LN"},
			"airline tickets",
		},
		{
			"punctuation counts as boundary",
			"comma, then something",
			map[string]string{"comma": ","},
			",, then something",
		},
		{
			"unicode whole-word (Russian)",
			"скажи новая строка сейчас",
			map[string]string{"новая строка": "\n"},
			"скажи \n сейчас",
		},
		{
			"no rules returns input",
			"hello",
			nil,
			"hello",
		},
		{
			"empty key is ignored",
			"hello",
			map[string]string{"": "x"},
			"hello",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Apply(tc.in, tc.rules)
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}
