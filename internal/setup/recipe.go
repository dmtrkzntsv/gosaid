package setup

import (
	"github.com/dmtrkzntsv/gosaid/internal/config"
)

// Recipes are the four hotkey pipelines offered by the wizard, mirroring the
// shipped example config.
const (
	RecipeTranscribe = "transcribe" // transcribe only
	RecipeCleanup    = "cleanup"    // transcribe → enhance
	RecipeTranslate  = "translate"  // transcribe → enhance → translate
	RecipeCompose    = "compose"    // transcribe → compose
)

// HotkeyAnswers is everything the hotkey wizard collects. BuildHotkey turns
// it into a config.Hotkey; AnswersFrom inverts that for the edit flow.
type HotkeyAnswers struct {
	Combo         string
	Mode          string // "hold" or "toggle"
	Recipe        string
	TranscribeRef string // "endpoint:model"
	ChatRef       string // "endpoint:model"; empty for RecipeTranscribe
	TargetLang    string // RecipeTranslate only
	Instructions  string // RecipeCompose only
	Microphone    string // per-hotkey override; usually empty
}

// RecipeOf classifies an existing hotkey. Compose wins over translate wins
// over enhance (a hotkey with both translate and enhance is the translate
// recipe — that's how the wizard builds them).
func RecipeOf(hk config.Hotkey) string {
	switch {
	case hk.Compose.IsEnabled():
		return RecipeCompose
	case hk.Translate.IsEnabled():
		return RecipeTranslate
	case hk.Enhance.IsEnabled():
		return RecipeCleanup
	default:
		return RecipeTranscribe
	}
}

// BuildHotkey materializes wizard answers into a hotkey.
func BuildHotkey(a HotkeyAnswers) config.Hotkey {
	hk := config.Hotkey{
		Mode:       config.HotkeyMode(a.Mode),
		Microphone: a.Microphone,
		Transcribe: config.TranscribeStage{Model: a.TranscribeRef},
	}
	switch a.Recipe {
	case RecipeCleanup:
		hk.Enhance = &config.EnhanceStage{Model: a.ChatRef}
	case RecipeTranslate:
		hk.Enhance = &config.EnhanceStage{Model: a.ChatRef}
		hk.Translate = &config.TranslateStage{OutputLanguage: a.TargetLang, Model: a.ChatRef}
	case RecipeCompose:
		hk.Compose = &config.ComposeStage{Model: a.ChatRef, Instructions: a.Instructions}
	}
	return hk
}

// AnswersFrom pre-fills the wizard from an existing hotkey (edit flow).
func AnswersFrom(combo string, hk config.Hotkey) HotkeyAnswers {
	mode := hk.Mode
	if mode == "" {
		mode = config.ModeHold
	}
	a := HotkeyAnswers{
		Combo:         combo,
		Mode:          string(mode),
		Recipe:        RecipeOf(hk),
		TranscribeRef: hk.Transcribe.Model,
		Microphone:    hk.Microphone,
	}
	switch a.Recipe {
	case RecipeCleanup:
		a.ChatRef = hk.Enhance.Model
	case RecipeTranslate:
		a.ChatRef = hk.Translate.Model
		a.TargetLang = hk.Translate.OutputLanguage
	case RecipeCompose:
		a.ChatRef = hk.Compose.Model
		a.Instructions = hk.Compose.Instructions
	}
	return a
}
