// Package setup implements the interactive `gosaid setup` flows. It is split
// into a pure layer (option builders and config mutators, unit-tested) and
// thin huh form wiring.
package setup

// ProviderPreset describes one entry in the add-provider picker. Cloud
// presets prefill APIBase and carry suggested model ids; Local routes into
// the local model manager; Custom asks for everything.
type ProviderPreset struct {
	Key             string // default endpoint id ("openai")
	Label           string // picker label
	APIBase         string
	TranscribeModel string // "" = provider has no transcription API
	ChatModel       string
	Local           bool
	Custom          bool
}

// ProviderPresets is ordered as shown in the picker: Local Whisper first,
// Custom last (per the design spec). Anything not listed here is reachable
// through Custom, which asks for the api_base and model ids directly.
var ProviderPresets = []ProviderPreset{
	{Key: "local", Label: "Local Whisper (on-device)", Local: true},
	{Key: "openai", Label: "OpenAI", APIBase: "https://api.openai.com/v1",
		TranscribeModel: "whisper-1", ChatModel: "gpt-5.4-nano"},
	{Key: "groq", Label: "Groq", APIBase: "https://api.groq.com/openai/v1",
		TranscribeModel: "whisper-large-v3-turbo", ChatModel: "llama-3.3-70b-versatile"},
	{Key: "openrouter", Label: "OpenRouter", APIBase: "https://openrouter.ai/api/v1",
		TranscribeModel: "", ChatModel: "openai/gpt-5.4-nano"},
	{Key: "custom", Label: "Custom (OpenAI-compatible)", Custom: true},
}

// PresetForAPIBase matches an endpoint's api_base against the preset table,
// so the hotkey wizard can suggest models for endpoints added by preset (or
// hand-edited to a known base). Returns nil for unknown bases.
func PresetForAPIBase(apiBase string) *ProviderPreset {
	for i := range ProviderPresets {
		p := &ProviderPresets[i]
		if !p.Local && !p.Custom && p.APIBase == apiBase {
			return p
		}
	}
	return nil
}
