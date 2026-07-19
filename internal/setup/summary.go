package setup

import (
	"fmt"
	"net/url"
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

// EndpointSummary renders one picker line for an endpoint:
// "openai · api.openai.com" or "local · whisper_cpp (2 models)".
func EndpointSummary(driver string, e config.Endpoint) string {
	if driver == config.DriverWhisperCPP {
		n := len(e.Config.Models)
		noun := "models"
		if n == 1 {
			noun = "model"
		}
		return fmt.Sprintf("%s · %s (%d %s)", e.ID, driver, n, noun)
	}
	host := e.Config.APIBase
	if u, err := url.Parse(host); err == nil && u.Host != "" {
		host = u.Host
	}
	return fmt.Sprintf("%s · %s", e.ID, host)
}
