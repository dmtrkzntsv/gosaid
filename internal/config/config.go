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
	// LicenseKey is reserved for a future licensing UI. Currently unused by
	// the daemon.
	LicenseKey string `json:"license_key,omitempty"`
	// PIDFile and DaemonBinary are written by the daemon on startup as hints
	// for external tools (e.g. the desktop UI) that need to locate the
	// running daemon. Any user-provided value is overwritten.
	PIDFile      string `json:"pid_file,omitempty"`
	DaemonBinary string `json:"daemon_binary,omitempty"`
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
	Mode       HotkeyMode      `json:"mode,omitempty"`
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
