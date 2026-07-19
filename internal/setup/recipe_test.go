package setup

import (
	"testing"

	"github.com/dmtrkzntsv/gosaid/internal/config"
)

func TestRecipeOf(t *testing.T) {
	cases := []struct {
		name string
		hk   config.Hotkey
		want string
	}{
		{"plain", config.Hotkey{}, RecipeTranscribe},
		{"cleanup", config.Hotkey{Enhance: &config.EnhanceStage{Model: "x:y"}}, RecipeCleanup},
		{"translate wins over enhance", config.Hotkey{
			Enhance:   &config.EnhanceStage{Model: "x:y"},
			Translate: &config.TranslateStage{OutputLanguage: "en", Model: "x:y"},
		}, RecipeTranslate},
		{"compose", config.Hotkey{Compose: &config.ComposeStage{Model: "x:y"}}, RecipeCompose},
	}
	for _, c := range cases {
		if got := RecipeOf(c.hk); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

func TestBuildHotkeyRoundTrip(t *testing.T) {
	a := HotkeyAnswers{
		Combo: "option+right", Mode: "hold", Recipe: RecipeTranslate,
		TranscribeRef: "openai:whisper-1", ChatRef: "openai:gpt-5.4-nano",
		TargetLang: "en",
	}
	hk := BuildHotkey(a)
	if hk.Transcribe.Model != "openai:whisper-1" {
		t.Errorf("transcribe = %q", hk.Transcribe.Model)
	}
	if !hk.Enhance.IsEnabled() || hk.Enhance.Model != "openai:gpt-5.4-nano" {
		t.Errorf("translate recipe must include enhance (matches example config): %+v", hk.Enhance)
	}
	if !hk.Translate.IsEnabled() || hk.Translate.OutputLanguage != "en" {
		t.Errorf("translate stage: %+v", hk.Translate)
	}
	if hk.Compose != nil {
		t.Error("compose must be absent")
	}

	back := AnswersFrom("option+right", hk)
	if back.Recipe != RecipeTranslate || back.ChatRef != "openai:gpt-5.4-nano" ||
		back.TargetLang != "en" || back.TranscribeRef != "openai:whisper-1" {
		t.Errorf("AnswersFrom lost data: %+v", back)
	}
}

func TestBuildHotkeyCompose(t *testing.T) {
	hk := BuildHotkey(HotkeyAnswers{
		Combo: "option+up", Mode: "hold", Recipe: RecipeCompose,
		TranscribeRef: "openai:whisper-1", ChatRef: "openai:gpt-5.4-nano",
		Instructions: "Formal register.",
	})
	if !hk.Compose.IsEnabled() || hk.Compose.Instructions != "Formal register." {
		t.Errorf("compose: %+v", hk.Compose)
	}
	if hk.Enhance != nil || hk.Translate != nil {
		t.Error("compose recipe must not add enhance/translate")
	}
}
