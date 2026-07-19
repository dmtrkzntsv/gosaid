package setup

import (
	"reflect"
	"testing"

	"github.com/dmtrkzntsv/gosaid/internal/config"
)

func TestAddOpenAIEndpoint(t *testing.T) {
	cfg := &config.Config{}
	if err := AddOpenAIEndpoint(cfg, "groq", "https://api.groq.com/openai/v1", "gsk-x"); err != nil {
		t.Fatal(err)
	}
	if len(cfg.Drivers) != 1 || cfg.Drivers[0].Driver != config.DriverOpenAICompatible {
		t.Fatalf("driver block not created: %+v", cfg.Drivers)
	}
	if err := AddOpenAIEndpoint(cfg, "groq", "x", "y"); err == nil {
		t.Fatal("duplicate id must error")
	}
	if err := AddOpenAIEndpoint(cfg, "bad:id", "x", "y"); err == nil {
		t.Fatal("id containing ':' must error (model refs split on colon)")
	}
	if err := AddOpenAIEndpoint(cfg, "", "x", "y"); err == nil {
		t.Fatal("empty id must error")
	}
	// Second endpoint appends to the same driver block.
	if err := AddOpenAIEndpoint(cfg, "openai", "https://api.openai.com/v1", "sk-x"); err != nil {
		t.Fatal(err)
	}
	if len(cfg.Drivers) != 1 || len(cfg.Drivers[0].Endpoints) != 2 {
		t.Fatalf("expected one block with two endpoints: %+v", cfg.Drivers)
	}
}

func TestUpdateAndDeleteEndpoint(t *testing.T) {
	cfg := testConfig()
	if err := UpdateOpenAIEndpoint(cfg, "openai", "https://api.openai.com/v2", "sk-new"); err != nil {
		t.Fatal(err)
	}
	if got := cfg.Drivers[0].Endpoints[0].Config.APIKey; got != "sk-new" {
		t.Fatalf("key not updated: %q", got)
	}
	if err := UpdateOpenAIEndpoint(cfg, "nope", "x", "y"); err == nil {
		t.Fatal("unknown id must error")
	}

	if err := DeleteEndpoint(cfg, "openai"); err != nil {
		t.Fatal(err)
	}
	if EndpointIDInUse(cfg, "openai") {
		t.Fatal("endpoint still present after delete")
	}
	// openai_compatible block had one endpoint — it must be pruned.
	for _, d := range cfg.Drivers {
		if d.Driver == config.DriverOpenAICompatible {
			t.Fatal("empty driver block must be pruned")
		}
	}
	if err := DeleteEndpoint(cfg, "nope"); err == nil {
		t.Fatal("unknown id must error")
	}
}

func TestHotkeysUsingEndpointAndModel(t *testing.T) {
	cfg := testConfig()
	cfg.Hotkeys = map[string]config.Hotkey{
		"option+left": {Transcribe: config.TranscribeStage{Model: "local:base"}},
		"option+right": {
			Transcribe: config.TranscribeStage{Model: "openai:whisper-1"},
			Enhance:    &config.EnhanceStage{Model: "openai:gpt-5.4-nano"},
		},
		"option+up": {Transcribe: config.TranscribeStage{Model: "local:tiny"}},
	}
	if got := HotkeysUsingEndpoint(cfg, "openai"); !reflect.DeepEqual(got, []string{"option+right"}) {
		t.Errorf("HotkeysUsingEndpoint = %v", got)
	}
	want := []string{"option+left"}
	if got := HotkeysUsingModel(cfg, "local", "base"); !reflect.DeepEqual(got, want) {
		t.Errorf("HotkeysUsingModel = %v, want %v", got, want)
	}
	if got := HotkeysUsingEndpoint(cfg, "ghost"); len(got) != 0 {
		t.Errorf("unknown endpoint: %v", got)
	}
}

func TestReassignEndpoint(t *testing.T) {
	cfg := testConfig()
	// Add a groq endpoint (preset-known base) as the reassignment target.
	if err := AddOpenAIEndpoint(cfg, "groq", "https://api.groq.com/openai/v1", "gsk-x"); err != nil {
		t.Fatal(err)
	}
	cfg.Hotkeys = map[string]config.Hotkey{
		"option+right": {
			Transcribe: config.TranscribeStage{Model: "openai:whisper-1"},
			Enhance:    &config.EnhanceStage{Model: "openai:gpt-5.4-nano"},
		},
	}
	ReassignEndpoint(cfg, "openai", "groq")
	hk := cfg.Hotkeys["option+right"]
	if hk.Transcribe.Model != "groq:whisper-large-v3-turbo" {
		t.Errorf("transcribe ref = %q, want preset-suggested groq transcribe model", hk.Transcribe.Model)
	}
	if hk.Enhance.Model != "groq:llama-3.3-70b-versatile" {
		t.Errorf("enhance ref = %q, want preset-suggested groq chat model", hk.Enhance.Model)
	}
}

func TestFirstRun(t *testing.T) {
	if FirstRun(testConfig()) {
		t.Fatal("config with a real api key is not first-run")
	}
	example := &config.Config{Drivers: []config.Driver{{
		Driver: config.DriverOpenAICompatible,
		Endpoints: []config.Endpoint{{ID: "openai", Config: config.EndpointConfig{
			APIBase: "https://api.openai.com/v1", APIKey: "REPLACE_ME",
		}}},
	}}}
	if !FirstRun(example) {
		t.Fatal("shipped example config (REPLACE_ME key) must count as first-run")
	}
	if !FirstRun(&config.Config{}) {
		t.Fatal("empty config must count as first-run")
	}
	local := &config.Config{Drivers: []config.Driver{{
		Driver:    config.DriverWhisperCPP,
		Endpoints: []config.Endpoint{{ID: "local", Config: config.EndpointConfig{Models: map[string]string{"base": "/m/b.bin"}}}},
	}}}
	if FirstRun(local) {
		t.Fatal("a whisper_cpp endpoint means configured")
	}
}

func TestResetForFirstRun(t *testing.T) {
	cfg := config.Default()
	cfg.SoundFeedback = true
	ResetForFirstRun(cfg)
	if len(cfg.Drivers) != 0 || len(cfg.Hotkeys) != 0 {
		t.Fatalf("drivers/hotkeys must be cleared: %+v", cfg)
	}
	if cfg.ToggleMaxSeconds != config.DefaultToggleSeconds || !cfg.SoundFeedback {
		t.Fatal("global settings must survive the reset")
	}
}

func TestDeleteHotkeyBlocked(t *testing.T) {
	cases := []struct {
		name    string
		hotkeys map[string]config.Hotkey
		combo   string
		blocked bool
	}{
		{
			name:    "last hotkey blocked",
			hotkeys: map[string]config.Hotkey{"option+space": {}},
			combo:   "option+space",
			blocked: true,
		},
		{
			name: "one of several allowed",
			hotkeys: map[string]config.Hotkey{
				"option+space": {}, "option+left": {},
			},
			combo:   "option+space",
			blocked: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Config{Hotkeys: tc.hotkeys}
			reason := DeleteHotkeyBlocked(cfg, tc.combo)
			if tc.blocked && reason == "" {
				t.Fatal("expected a block reason, got none")
			}
			if !tc.blocked && reason != "" {
				t.Fatalf("expected no block, got %q", reason)
			}
		})
	}
}

func TestDeleteEndpointBlocked(t *testing.T) {
	cases := []struct {
		name    string
		cfg     *config.Config
		id      string
		blocked bool
	}{
		{
			name: "only endpoint blocked",
			cfg: &config.Config{Drivers: []config.Driver{
				{Driver: config.DriverOpenAICompatible, Endpoints: []config.Endpoint{{ID: "openai"}}},
			}},
			id:      "openai",
			blocked: true,
		},
		{
			name:    "two endpoints across drivers allowed",
			cfg:     testConfig(),
			id:      "openai",
			blocked: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			reason := DeleteEndpointBlocked(tc.cfg, tc.id)
			if tc.blocked && reason == "" {
				t.Fatal("expected a block reason, got none")
			}
			if !tc.blocked && reason != "" {
				t.Fatalf("expected no block, got %q", reason)
			}
		})
	}
}

func TestRemoveModelBlocked(t *testing.T) {
	oneEndpointOneModel := &config.Config{Drivers: []config.Driver{
		{Driver: config.DriverWhisperCPP, Endpoints: []config.Endpoint{
			{ID: "local", Config: config.EndpointConfig{Models: map[string]string{"base": "/m/b.bin"}}},
		}},
	}}
	cases := []struct {
		name       string
		cfg        *config.Config
		endpointID string
		model      string
		blocked    bool
	}{
		{
			name:       "last model of the only endpoint blocked",
			cfg:        oneEndpointOneModel,
			endpointID: "local",
			model:      "base",
			blocked:    true,
		},
		{
			name:       "one of two models on the only endpoint allowed",
			cfg:        testConfig(),
			endpointID: "local",
			model:      "base",
			blocked:    false,
		},
		{
			name: "last model but other endpoints exist allowed",
			cfg: &config.Config{Drivers: []config.Driver{
				{Driver: config.DriverOpenAICompatible, Endpoints: []config.Endpoint{{ID: "openai"}}},
				{Driver: config.DriverWhisperCPP, Endpoints: []config.Endpoint{
					{ID: "local", Config: config.EndpointConfig{Models: map[string]string{"base": "/m/b.bin"}}},
				}},
			}},
			endpointID: "local",
			model:      "base",
			blocked:    false,
		},
		{
			name:       "unknown model name allowed",
			cfg:        oneEndpointOneModel,
			endpointID: "local",
			model:      "ghost",
			blocked:    false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			reason := RemoveModelBlocked(tc.cfg, tc.endpointID, tc.model)
			if tc.blocked && reason == "" {
				t.Fatal("expected a block reason, got none")
			}
			if !tc.blocked && reason != "" {
				t.Fatalf("expected no block, got %q", reason)
			}
		})
	}
}

func TestDiffModelSelection(t *testing.T) {
	registered := map[string]string{"base": "/m/b.bin", "tiny": "/m/t.bin"}
	d := DiffModelSelection(registered, []string{"base", "large-v3-turbo"})
	if !reflect.DeepEqual(d.Add, []string{"large-v3-turbo"}) {
		t.Errorf("Add = %v", d.Add)
	}
	if !reflect.DeepEqual(d.Remove, []string{"tiny"}) {
		t.Errorf("Remove = %v", d.Remove)
	}
}
