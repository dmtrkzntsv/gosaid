package config

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func validCfg() *Config {
	c := Default()
	c.Drivers[0].Endpoints[0].Config.APIKey = "sk-test"
	return c
}

func boolPtr(b bool) *bool { return &b }

func TestDefaultStructure(t *testing.T) {
	c := Default()
	if c.Version != 2 {
		t.Fatalf("version = %d, want 2", c.Version)
	}
	if len(c.Drivers) != 1 {
		t.Fatalf("drivers = %d, want 1", len(c.Drivers))
	}
	if _, ok := c.Hotkeys["ctrl+alt+space"]; !ok {
		t.Fatal("default ctrl+alt+space hotkey missing")
	}
}

func TestValidate_MissingAPIKeyFails(t *testing.T) {
	c := Default() // key is empty by design
	if err := Validate(c); err == nil {
		t.Fatal("expected validation error for empty api_key")
	}
}

func TestValidate_Valid(t *testing.T) {
	if err := Validate(validCfg()); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}
}

func TestValidate_UnknownDriver(t *testing.T) {
	c := validCfg()
	c.Drivers[0].Driver = "nope"
	if err := Validate(c); err == nil {
		t.Fatal("expected error for unknown driver")
	}
}

func TestValidate_DuplicateEndpointID(t *testing.T) {
	c := validCfg()
	c.Drivers[0].Endpoints = append(c.Drivers[0].Endpoints, Endpoint{
		ID:     "groq",
		Config: OpenAICompatibleConfig{APIBase: "x", APIKey: "y"},
	})
	if err := Validate(c); err == nil {
		t.Fatal("expected error for duplicate endpoint id")
	}
}

func TestValidate_HotkeyModelRefUnknownEndpoint(t *testing.T) {
	c := validCfg()
	c.Hotkeys["ctrl+alt+space"] = Hotkey{
		Mode:       ModeHold,
		Transcribe: TranscribeStage{Model: "ghost:whisper"},
	}
	if err := Validate(c); err == nil {
		t.Fatal("expected error for model referencing unknown endpoint")
	}
}

func TestValidate_TranscribeOutputLanguageMustBeEnglish(t *testing.T) {
	c := validCfg()
	hk := c.Hotkeys["ctrl+alt+space"]
	hk.Transcribe.OutputLanguage = "fr"
	c.Hotkeys["ctrl+alt+space"] = hk
	if err := Validate(c); err == nil {
		t.Fatal("expected error when transcribe.output_language != 'en'")
	}
}

func TestValidate_TranscribeInputLanguage(t *testing.T) {
	c := validCfg()
	hk := c.Hotkeys["ctrl+alt+space"]
	hk.Transcribe.InputLanguage = "ru"
	c.Hotkeys["ctrl+alt+space"] = hk
	if err := Validate(c); err != nil {
		t.Fatalf("valid input_language 'ru' rejected: %v", err)
	}

	hk.Transcribe.InputLanguage = "xx"
	c.Hotkeys["ctrl+alt+space"] = hk
	if err := Validate(c); err == nil {
		t.Fatal("expected error for unknown input_language 'xx'")
	}
}

func TestValidate_ComposeRequiresModel(t *testing.T) {
	c := validCfg()
	hk := c.Hotkeys["ctrl+alt+space"]
	hk.Compose = &ComposeStage{}
	c.Hotkeys["ctrl+alt+space"] = hk
	if err := Validate(c); err == nil {
		t.Fatal("expected error for compose stage without model")
	}
}

func TestValidate_ComposeUnknownEndpoint(t *testing.T) {
	c := validCfg()
	hk := c.Hotkeys["ctrl+alt+space"]
	hk.Compose = &ComposeStage{Model: "ghost:gpt-4"}
	c.Hotkeys["ctrl+alt+space"] = hk
	if err := Validate(c); err == nil {
		t.Fatal("expected error for compose.model referencing unknown endpoint")
	}
}

func TestValidate_DisabledComposeSkipsModelCheck(t *testing.T) {
	c := validCfg()
	hk := c.Hotkeys["ctrl+alt+space"]
	hk.Compose = &ComposeStage{Enable: boolPtr(false)}
	c.Hotkeys["ctrl+alt+space"] = hk
	if err := Validate(c); err != nil {
		t.Fatalf("disabled compose should skip model check: %v", err)
	}
}

func TestValidate_EnabledComposeRequiresModel(t *testing.T) {
	c := validCfg()
	hk := c.Hotkeys["ctrl+alt+space"]
	hk.Compose = &ComposeStage{Enable: boolPtr(true)}
	c.Hotkeys["ctrl+alt+space"] = hk
	if err := Validate(c); err == nil {
		t.Fatal("expected error for explicitly enabled compose without model")
	}
}

func TestValidate_DisabledTranslateSkipsLanguageCheck(t *testing.T) {
	c := validCfg()
	hk := c.Hotkeys["ctrl+alt+space"]
	hk.Translate = &TranslateStage{Enable: boolPtr(false)}
	c.Hotkeys["ctrl+alt+space"] = hk
	if err := Validate(c); err != nil {
		t.Fatalf("disabled translate should skip field checks: %v", err)
	}
}

func TestValidate_InvalidCombo(t *testing.T) {
	c := validCfg()
	c.Hotkeys = map[string]Hotkey{
		"space": c.Hotkeys["ctrl+alt+space"], // single-part combo
	}
	if err := Validate(c); err == nil {
		t.Fatal("expected error for combo with no modifier")
	}
}

func TestValidate_ToggleMaxSecondsPositive(t *testing.T) {
	c := validCfg()
	c.ToggleMaxSeconds = 0
	if err := Validate(c); err == nil {
		t.Fatal("expected error for zero toggle_max_seconds")
	}
}

func TestLoadSaveRoundTrip(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "c.json")

	// Missing file: should write default.
	c1, err := Load(p)
	if err != nil {
		t.Fatalf("initial load: %v", err)
	}
	if c1.Version != 2 {
		t.Fatalf("default version = %d", c1.Version)
	}

	// Modify and save.
	c1.LogLevel = "debug"
	if err := Save(p, c1); err != nil {
		t.Fatalf("save: %v", err)
	}
	c2, err := Load(p)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if c2.LogLevel != "debug" {
		t.Fatalf("round-trip lost log_level, got %q", c2.LogLevel)
	}
}

// When the config file is missing, Load must write the embedded example
// (not the minimal programmatic Default) so users get a documented starting
// point with multiple hotkey examples.
func TestLoad_MissingWritesEmbeddedExample(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "c.json")

	if _, err := Load(p); err != nil {
		t.Fatalf("load: %v", err)
	}
	written, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read written config: %v", err)
	}
	if !bytes.Equal(written, exampleConfig) {
		t.Fatalf("written config does not match embedded example")
	}
}

