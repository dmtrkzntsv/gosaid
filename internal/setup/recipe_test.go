package setup

import (
	"testing"

	"github.com/dmtrkzntsv/gosaid/internal/config"
)

func TestNeedsChatModel(t *testing.T) {
	if (HotkeyAnswers{}).NeedsChatModel() {
		t.Error("transcribe-only needs no chat model")
	}
	if !(HotkeyAnswers{Enhance: true}).NeedsChatModel() {
		t.Error("enhance needs a chat model")
	}
	if !(HotkeyAnswers{Translate: true}).NeedsChatModel() {
		t.Error("translate needs a chat model")
	}
	if !(HotkeyAnswers{Compose: true}).NeedsChatModel() {
		t.Error("compose needs a chat model")
	}
}

func TestBuildHotkeyStages(t *testing.T) {
	a := HotkeyAnswers{
		Combo: "option+space", Mode: "hold",
		TranscribeRef: "speech:turbo", ChatRef: "text:gemma-4-e2b",
		Enhance: true, Translate: true, TargetLang: "en",
	}
	hk := BuildHotkey(a)
	if hk.Transcribe.Model != "speech:turbo" {
		t.Errorf("transcribe = %q", hk.Transcribe.Model)
	}
	if !hk.Enhance.IsEnabled() || hk.Enhance.Model != "text:gemma-4-e2b" {
		t.Errorf("enhance = %+v", hk.Enhance)
	}
	if !hk.Translate.IsEnabled() || hk.Translate.OutputLanguage != "en" || hk.Translate.Model != "text:gemma-4-e2b" {
		t.Errorf("translate = %+v", hk.Translate)
	}
	if hk.Compose != nil {
		t.Error("compose must be absent when not enabled")
	}
}

func TestBuildHotkeyIndependentStages(t *testing.T) {
	// Compose without enhance — proves stages are independent, not a recipe
	// ladder (the old model forced enhance whenever translate was on).
	hk := BuildHotkey(HotkeyAnswers{
		Combo: "option+up", Mode: "toggle",
		TranscribeRef: "speech:small", ChatRef: "text:qwen3.5-0.8b",
		Compose: true, Instructions: "Formal register.",
	})
	if hk.Enhance != nil || hk.Translate != nil {
		t.Error("only compose should be set")
	}
	if !hk.Compose.IsEnabled() || hk.Compose.Instructions != "Formal register." {
		t.Errorf("compose = %+v", hk.Compose)
	}
	if hk.Mode != config.ModeToggle {
		t.Errorf("mode = %q", hk.Mode)
	}
}

func TestAnswersFromRoundTrip(t *testing.T) {
	orig := config.Hotkey{
		Mode:       config.ModeHold,
		Transcribe: config.TranscribeStage{Model: "speech:turbo"},
		Enhance:    &config.EnhanceStage{Model: "text:gemma-4-e2b"},
		Compose:    &config.ComposeStage{Model: "text:gemma-4-e2b", Instructions: "x"},
	}
	a := AnswersFrom("option+right", orig)
	if a.Combo != "option+right" || a.Mode != "hold" {
		t.Errorf("combo/mode = %q/%q", a.Combo, a.Mode)
	}
	if !a.Enhance || a.Translate || !a.Compose {
		t.Errorf("stage flags = %v/%v/%v", a.Enhance, a.Translate, a.Compose)
	}
	if a.ChatRef != "text:gemma-4-e2b" || a.Instructions != "x" {
		t.Errorf("chat/instructions = %q/%q", a.ChatRef, a.Instructions)
	}
	if a.TranscribeRef != "speech:turbo" {
		t.Errorf("transcribe = %q", a.TranscribeRef)
	}
	// An empty-mode hotkey round-trips as hold.
	if got := AnswersFrom("x", config.Hotkey{}).Mode; got != "hold" {
		t.Errorf("default mode = %q, want hold", got)
	}
}
