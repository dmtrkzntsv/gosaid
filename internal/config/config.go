package config

type Config struct {
	Version          int               `json:"version"`
	Drivers          []Driver          `json:"drivers"`
	Hotkeys          map[string]Hotkey `json:"hotkeys"`
	ToggleMaxSeconds int               `json:"toggle_max_seconds"`
	InjectionMode    string            `json:"injection_mode"`
	SoundFeedback    bool              `json:"sound_feedback"`
	LogLevel         string            `json:"log_level"`
	// UserContext is free-form personal context (name, role, tone preferences)
	// injected into the compose stage system prompt. Can be written in any
	// language; the model is instructed to match the user's instruction
	// language for the output.
	UserContext string `json:"user_context,omitempty"`
	// Microphone is the default input device for all hotkeys, matched
	// case-insensitively as a substring of the device name. Empty uses the
	// system default. A hotkey's own Microphone field overrides this.
	Microphone string `json:"microphone,omitempty"`
}

type Driver struct {
	Driver    string     `json:"driver"`
	Endpoints []Endpoint `json:"endpoints"`
}

type Endpoint struct {
	ID     string         `json:"id"`
	Config EndpointConfig `json:"config"`
}

// EndpointConfig is the per-endpoint configuration. Which fields are required
// depends on the driver type: openai_compatible needs api_base/api_key;
// whisper_cpp needs models (name → GGML model file path);
// llama_cpp needs models (name → GGUF model file path).
type EndpointConfig struct {
	APIBase string            `json:"api_base,omitempty"`
	APIKey  string            `json:"api_key,omitempty"`
	Models  map[string]string `json:"models,omitempty"`
	// UnloadAfterSeconds (whisper_cpp / llama_cpp only) frees a loaded model after this
	// many seconds without use; it reloads lazily on the next dictation.
	// 0 or absent keeps models resident once loaded.
	UnloadAfterSeconds int `json:"unload_after_seconds,omitempty"`
}

// MicrophoneFor resolves the input device for a hotkey: the hotkey's own
// Microphone if set, else the global default, else "" (system default).
func (c *Config) MicrophoneFor(hk Hotkey) string {
	if hk.Microphone != "" {
		return hk.Microphone
	}
	return c.Microphone
}

type HotkeyMode string

const (
	ModeHold   HotkeyMode = "hold"
	ModeToggle HotkeyMode = "toggle"
)

type Hotkey struct {
	Mode HotkeyMode `json:"mode,omitempty"`
	// Microphone selects the input device for this hotkey by name —
	// a case-insensitive substring match (pick devices interactively with
	// `gosaid mic`). Empty falls back to the global Microphone
	// setting, then the system default. If the device is absent when
	// recording starts, capture falls back to the default (logged).
	Microphone string          `json:"microphone,omitempty"`
	Transcribe TranscribeStage `json:"transcribe"`
	Translate  *TranslateStage `json:"translate,omitempty"`
	Enhance    *EnhanceStage   `json:"enhance,omitempty"`
	Compose    *ComposeStage   `json:"compose,omitempty"`
}

type TranscribeStage struct {
	Model          string `json:"model"`
	InputLanguage  string `json:"input_language,omitempty"`
	OutputLanguage string `json:"output_language,omitempty"`
}

type TranslateStage struct {
	// Enable toggles the stage without removing the section. Nil or true →
	// stage runs when present; false → stage is skipped and its fields are
	// not validated.
	Enable         *bool  `json:"enable,omitempty"`
	OutputLanguage string `json:"output_language"`
	Model          string `json:"model"`
}

type EnhanceStage struct {
	// Enable toggles the stage without removing the section. Nil or true →
	// stage runs when present; false → stage is skipped and its fields are
	// not validated.
	Enable *bool  `json:"enable,omitempty"`
	Model  string `json:"model"`
}

type ComposeStage struct {
	// Enable toggles the stage without removing the section. Nil or true →
	// stage runs when present; false → stage is skipped and its fields are
	// not validated.
	Enable *bool  `json:"enable,omitempty"`
	Model  string `json:"model"`
	// Instructions is appended to the compose system prompt as an additional
	// per-hotkey directive (e.g. "always write in formal register" for a
	// business-email hotkey). Does not replace the defaults.
	Instructions string `json:"instructions,omitempty"`
}

// IsEnabled reports whether the stage should run. A nil receiver (absent
// section) or an explicit false returns false; any other state returns true.
func (s *TranslateStage) IsEnabled() bool {
	return s != nil && (s.Enable == nil || *s.Enable)
}

// IsEnabled reports whether the stage should run. A nil receiver (absent
// section) or an explicit false returns false; any other state returns true.
func (s *EnhanceStage) IsEnabled() bool {
	return s != nil && (s.Enable == nil || *s.Enable)
}

// IsEnabled reports whether the stage should run. A nil receiver (absent
// section) or an explicit false returns false; any other state returns true.
func (s *ComposeStage) IsEnabled() bool {
	return s != nil && (s.Enable == nil || *s.Enable)
}

const (
	CurrentVersion         = 2
	DriverOpenAICompatible = "openai_compatible"
	DriverWhisperCPP       = "whisper_cpp"
	DriverLlamaCPP         = "llama_cpp"
	InjectionModePaste     = "paste"
	DefaultToggleSeconds   = 60
)

// Default returns a minimal, valid-structure config that nevertheless requires
// the user to fill in an API key before it will pass validation.
func Default() *Config {
	return &Config{
		Version: CurrentVersion,
		Drivers: []Driver{{
			Driver: DriverOpenAICompatible,
			Endpoints: []Endpoint{{
				ID: "groq",
				Config: EndpointConfig{
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
