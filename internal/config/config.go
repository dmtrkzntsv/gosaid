package config

type Config struct {
	Version          int                          `json:"version"`
	Drivers          []Driver                     `json:"drivers"`
	Vocabulary       map[string][]string          `json:"vocabulary,omitempty"`
	Replacements     map[string]map[string]string `json:"replacements,omitempty"`
	Hotkeys          map[string]Hotkey            `json:"hotkeys"`
	ToggleMaxSeconds int                          `json:"toggle_max_seconds"`
	InjectionMode    string                       `json:"injection_mode"`
	SoundFeedback    bool                         `json:"sound_feedback"`
	LogLevel         string                       `json:"log_level"`
}

type Driver struct {
	Driver    string     `json:"driver"`
	Endpoints []Endpoint `json:"endpoints"`
}

type Endpoint struct {
	ID     string                 `json:"id"`
	Config OpenAICompatibleConfig `json:"config"`
}

type OpenAICompatibleConfig struct {
	APIBase string `json:"api_base"`
	APIKey  string `json:"api_key"`
}

type HotkeyMode string

const (
	ModeHold   HotkeyMode = "hold"
	ModeToggle HotkeyMode = "toggle"
)

type Hotkey struct {
	Mode       HotkeyMode       `json:"mode,omitempty"`
	Transcribe TranscribeStage  `json:"transcribe"`
	Translate  *TranslateStage  `json:"translate,omitempty"`
	Enhance    *EnhanceStage    `json:"enhance,omitempty"`
}

type TranscribeStage struct {
	Model          string `json:"model"`
	InputLanguage  string `json:"input_language,omitempty"`
	OutputLanguage string `json:"output_language,omitempty"`
}

type TranslateStage struct {
	OutputLanguage string `json:"output_language"`
	Model          string `json:"model"`
	Prompt         string `json:"prompt,omitempty"`
}

type EnhanceStage struct {
	Prompt string `json:"prompt"`
	Model  string `json:"model"`
}

const (
	DriverOpenAICompatible = "openai_compatible"
	InjectionModePaste     = "paste"
	DefaultToggleSeconds   = 60
)

// Default returns a minimal, valid-structure config that nevertheless requires
// the user to fill in an API key before it will pass validation.
func Default() *Config {
	return &Config{
		Version: 1,
		Drivers: []Driver{{
			Driver: DriverOpenAICompatible,
			Endpoints: []Endpoint{{
				ID: "groq",
				Config: OpenAICompatibleConfig{
					APIBase: "https://api.groq.com/openai/v1",
					APIKey:  "",
				},
			}},
		}},
		Hotkeys: map[string]Hotkey{
			"ctrl+alt+space": {
				Mode:       ModeHold,
				Transcribe: TranscribeStage{Model: "groq:whisper-large-v3"},
			},
		},
		ToggleMaxSeconds: DefaultToggleSeconds,
		InjectionMode:    InjectionModePaste,
		SoundFeedback:    true,
		LogLevel:         "info",
	}
}
