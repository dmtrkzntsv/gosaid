package daemon

import (
	"strings"
	"testing"
)

func TestRenderTranslate_IncludesLanguages(t *testing.T) {
	out, err := RenderTranslate(TranslateData{
		SourceLanguage: "Russian",
		TargetLanguage: "English",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Russian", "English", "Match the exact wording"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\n---\n%s", want, out)
		}
	}
}

func TestRenderTranslate_OmitsSourceWhenUnknown(t *testing.T) {
	out, err := RenderTranslate(TranslateData{TargetLanguage: "English"})
	if err != nil {
		t.Fatal(err)
	}
	firstLine, _, _ := strings.Cut(out, "\n")
	want := "You are a translator. Translate the following text to English."
	if firstLine != want {
		t.Errorf("first line = %q, want %q", firstLine, want)
	}
}

func TestRenderEnhance_ContainsDefaultInstruction(t *testing.T) {
	out, err := RenderEnhance(EnhanceData{})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"speech disfluencies",
		"Preserve the original language",
		"code-switching",
		"capitalization",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("enhance prompt missing %q:\n%s", want, out)
		}
	}
}

func TestRenderCompose_ContainsExpectedMarkers(t *testing.T) {
	out, err := RenderCompose(ComposeData{})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"finished written artifact", "spoken instruction"} {
		if !strings.Contains(out, want) {
			t.Errorf("compose prompt missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "About the user (use for personalization") {
		t.Errorf("user-context block must be absent when UserContext is empty:\n%s", out)
	}
	if strings.Contains(out, "Additional instructions for this hotkey") {
		t.Errorf("instructions block must be absent when Instructions is empty:\n%s", out)
	}
}

func TestRenderCompose_WithUserContext(t *testing.T) {
	out, err := RenderCompose(ComposeData{UserContext: "My name is Dmitry."})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"About the user (use for personalization", "My name is Dmitry."} {
		if !strings.Contains(out, want) {
			t.Errorf("compose prompt missing %q:\n%s", want, out)
		}
	}
}

func TestRenderCompose_TrimsBlankUserContext(t *testing.T) {
	out, err := RenderCompose(ComposeData{UserContext: "   \n\t  "})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "About the user (use for personalization") {
		t.Errorf("whitespace-only UserContext must be treated as empty:\n%s", out)
	}
}

func TestRenderCompose_WithInstructions(t *testing.T) {
	out, err := RenderCompose(ComposeData{Instructions: "Always use formal register."})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Additional instructions for this hotkey", "Always use formal register."} {
		if !strings.Contains(out, want) {
			t.Errorf("compose prompt missing %q:\n%s", want, out)
		}
	}
}

func TestRenderCompose_TrimsBlankInstructions(t *testing.T) {
	out, err := RenderCompose(ComposeData{Instructions: "  \n  "})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "Additional instructions for this hotkey") {
		t.Errorf("whitespace-only Instructions must be treated as empty:\n%s", out)
	}
}

func TestRenderVocabulary_AllStages(t *testing.T) {
	const vocab = "Kubernetes, PostHog, gosaid"
	renderers := map[string]func() (string, error){
		"translate": func() (string, error) {
			return RenderTranslate(TranslateData{TargetLanguage: "English", Vocabulary: vocab})
		},
		"enhance": func() (string, error) {
			return RenderEnhance(EnhanceData{Vocabulary: vocab})
		},
		"compose": func() (string, error) {
			return RenderCompose(ComposeData{Vocabulary: vocab})
		},
		"transform": func() (string, error) {
			return RenderTransform(TransformData{Vocabulary: vocab})
		},
	}
	for name, render := range renderers {
		out, err := render()
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !strings.Contains(out, "Custom vocabulary") {
			t.Errorf("%s: missing vocabulary heading:\n%s", name, out)
		}
		if !strings.Contains(out, vocab) {
			t.Errorf("%s: missing vocabulary list:\n%s", name, out)
		}
	}
}

func TestRenderVocabulary_OmittedWhenEmpty(t *testing.T) {
	out, err := RenderCompose(ComposeData{Vocabulary: "   "})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "Custom vocabulary") {
		t.Errorf("blank vocabulary must be omitted:\n%s", out)
	}
}
