package daemon

import (
	"strings"
	"testing"
)

func TestRenderTranslate_IncludesLanguagesAndInstructions(t *testing.T) {
	out, err := RenderTranslate(TranslateData{
		SourceLanguage:   "Russian",
		TargetLanguage:   "English",
		UserInstructions: "Use formal register.",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Russian", "English", "Use formal register."} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\n---\n%s", want, out)
		}
	}
	// No vocabulary/replacement sections when empty.
	if strings.Contains(out, "Vocabulary") {
		t.Errorf("unexpected vocabulary section when empty:\n%s", out)
	}
}

func TestRenderTranslate_DefaultInstructionWhenEmpty(t *testing.T) {
	out, err := RenderTranslate(TranslateData{
		SourceLanguage: "French", TargetLanguage: "English",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Match the exact wording") {
		t.Errorf("default translate instruction missing:\n%s", out)
	}
}

func TestRenderEnhance_DefaultInstructionWhenEmpty(t *testing.T) {
	out, err := RenderEnhance(EnhanceData{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "speech disfluencies") {
		t.Errorf("default enhance instruction missing:\n%s", out)
	}
}

func TestRenderEnhance_DefaultInstructionWhenBlank(t *testing.T) {
	out, err := RenderEnhance(EnhanceData{UserInstructions: "   "})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "speech disfluencies") {
		t.Errorf("default enhance instruction missing for whitespace prompt:\n%s", out)
	}
}

func TestRenderTranslate_VocabularyAndReplacements(t *testing.T) {
	out, err := RenderTranslate(TranslateData{
		SourceLanguage:   "English",
		TargetLanguage:   "French",
		UserInstructions: "Technical content.",
		Vocabulary:       []string{"Kubernetes", "Goroutine"},
		Replacements:     []string{"new line", "new paragraph"},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Vocabulary", "Kubernetes", "Goroutine", "new line", "new paragraph"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestRenderEnhance_RequiresInstructions(t *testing.T) {
	out, err := RenderEnhance(EnhanceData{UserInstructions: "format as email"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "format as email") {
		t.Errorf("missing user instruction:\n%s", out)
	}
}
