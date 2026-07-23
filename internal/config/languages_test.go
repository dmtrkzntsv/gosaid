package config

import "testing"

func TestLanguagesSortedAndValid(t *testing.T) {
	langs := Languages()
	if len(langs) != len(languageNames) {
		t.Fatalf("Languages() returned %d codes, want %d", len(langs), len(languageNames))
	}
	for i, c := range langs {
		if !IsValidLanguage(c) {
			t.Errorf("invalid code %q", c)
		}
		if i > 0 && langs[i-1] >= c {
			t.Errorf("not sorted at %d: %q >= %q", i, langs[i-1], c)
		}
	}
}
