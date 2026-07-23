package setup

import (
	"fmt"
	"strings"

	"github.com/dmtrkzntsv/gosaid/internal/config"
)

// HotkeySummary renders one picker line for a hotkey:
// "option+right · hold · transcribe → enhance → translate(en)".
func HotkeySummary(combo string, hk config.Hotkey) string {
	mode := hk.Mode
	if mode == "" {
		mode = config.ModeHold
	}
	stages := []string{"transcribe"}
	if hk.Enhance.IsEnabled() {
		stages = append(stages, "enhance")
	}
	if hk.Translate.IsEnabled() {
		stages = append(stages, fmt.Sprintf("translate(%s)", hk.Translate.OutputLanguage))
	}
	if hk.Compose.IsEnabled() {
		stages = append(stages, "compose")
	}
	return fmt.Sprintf("%s · %s · %s", combo, mode, strings.Join(stages, " → "))
}
