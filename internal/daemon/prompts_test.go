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

func TestRenderEnhance_ContainsDefaultInstruction(t *testing.T) {
	out, err := RenderEnhance(EnhanceData{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "speech disfluencies") {
		t.Errorf("default enhance instruction missing:\n%s", out)
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
}
