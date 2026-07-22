package setup

import (
	"github.com/dmtrkzntsv/gosaid/internal/config"
)

// HotkeyAnswers is everything the wizard collects for one hotkey. The three
// stage flags are independent — unlike the old recipe model, enabling
// translate does not force enhance. One ChatRef backs every enabled stage.
type HotkeyAnswers struct {
	Combo         string
	Mode          string // "hold" or "toggle"
	TranscribeRef string // "speech:model"
	ChatRef       string // "text:model"; empty when no stage is enabled
	Enhance       bool
	Translate     bool
	Compose       bool
	TargetLang    string // Translate only
	Instructions  string // Compose only
}

// NeedsChatModel reports whether any stage that runs a chat model is enabled,
// so the wizard knows whether to ask for one.
func (a HotkeyAnswers) NeedsChatModel() bool {
	return a.Enhance || a.Translate || a.Compose
}

// BuildHotkey materializes wizard answers into a hotkey. Each enabled stage is
// set independently and shares the single ChatRef.
func BuildHotkey(a HotkeyAnswers) config.Hotkey {
	hk := config.Hotkey{
		Mode:       config.HotkeyMode(a.Mode),
		Transcribe: config.TranscribeStage{Model: a.TranscribeRef},
	}
	if a.Enhance {
		hk.Enhance = &config.EnhanceStage{Model: a.ChatRef}
	}
	if a.Translate {
		hk.Translate = &config.TranslateStage{OutputLanguage: a.TargetLang, Model: a.ChatRef}
	}
	if a.Compose {
		hk.Compose = &config.ComposeStage{Model: a.ChatRef, Instructions: a.Instructions}
	}
	return hk
}

// AnswersFrom pre-fills the wizard from an existing hotkey (the edit path).
// The chat ref is taken from whichever enabled stage has one — they share it.
func AnswersFrom(combo string, hk config.Hotkey) HotkeyAnswers {
	mode := hk.Mode
	if mode == "" {
		mode = config.ModeHold
	}
	a := HotkeyAnswers{
		Combo:         combo,
		Mode:          string(mode),
		TranscribeRef: hk.Transcribe.Model,
		Enhance:       hk.Enhance.IsEnabled(),
		Translate:     hk.Translate.IsEnabled(),
		Compose:       hk.Compose.IsEnabled(),
	}
	switch {
	case a.Enhance:
		a.ChatRef = hk.Enhance.Model
	case a.Translate:
		a.ChatRef = hk.Translate.Model
	case a.Compose:
		a.ChatRef = hk.Compose.Model
	}
	if a.Translate {
		a.TargetLang = hk.Translate.OutputLanguage
	}
	if a.Compose {
		a.Instructions = hk.Compose.Instructions
	}
	return a
}
