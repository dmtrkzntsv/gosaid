package daemon

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/dmtrkzntsv/gosaid/internal/config"
	"github.com/dmtrkzntsv/gosaid/internal/drivers"
	"github.com/dmtrkzntsv/gosaid/internal/inject"
)

type mockDriver struct {
	transcribe      func(model string, opts drivers.TranscribeOptions) (drivers.TranscribeResult, error)
	translateSpeech func(model string, opts drivers.TranslateSpeechOptions) (string, error)
	chat            func(model, system, user string) (string, error)
}

func (m *mockDriver) Transcribe(_ context.Context, _ []float32, _ int, model string, opts drivers.TranscribeOptions) (drivers.TranscribeResult, error) {
	return m.transcribe(model, opts)
}

func (m *mockDriver) TranslateSpeech(_ context.Context, _ []float32, _ int, model string, opts drivers.TranslateSpeechOptions) (string, error) {
	return m.translateSpeech(model, opts)
}

func (m *mockDriver) Chat(_ context.Context, model, system, user string) (string, error) {
	return m.chat(model, system, user)
}

type stopSamples struct{ samples []float32 }

func (s *stopSamples) Stop() ([]float32, error) { return s.samples, nil }

// newPipeline builds a Pipeline wired to a single-endpoint registry named "m".
func newPipeline(t *testing.T, drv drivers.Driver, cfg *config.Config, sink *strings.Builder) *Pipeline {
	t.Helper()
	cfg.Drivers = []config.Driver{{
		Driver: config.DriverOpenAICompatible,
		Endpoints: []config.Endpoint{{
			ID:     "m",
			Config: config.OpenAICompatibleConfig{APIBase: "http://x", APIKey: "k"},
		}},
	}}
	if err := config.Validate(cfg); err != nil {
		t.Fatalf("validate: %v", err)
	}
	reg, err := drivers.BuildRegistry(cfg)
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	// Override the endpoint with the mock driver via unexported access: we can
	// simply rebuild a registry-like wrapper. Easier: plug the mock into a
	// local registry by shadowing the "m" driver.
	_ = reg
	reg2 := mockRegistry{m: drv}
	return &Pipeline{
		Core:       NewCore(),
		Capture:    &stopSamples{samples: []float32{0, 0.1}},
		Registry:   reg2.asRegistry(),
		Injector:   inject.Stub{Writer: func(s string) { sink.WriteString(s) }},
		Config:     cfg,
		SampleRate: 16000,
	}
}

// mockRegistry replaces drivers.Registry for tests. drivers.Registry holds
// unexported fields; we expose a constructor here via the embedded type.
type mockRegistry struct{ m drivers.Driver }

func (r mockRegistry) asRegistry() *drivers.Registry {
	reg, _ := drivers.BuildRegistry(&config.Config{
		Version: 1,
		Drivers: []config.Driver{{
			Driver: config.DriverOpenAICompatible,
			Endpoints: []config.Endpoint{{
				ID:     "m",
				Config: config.OpenAICompatibleConfig{APIBase: "http://x", APIKey: "k"},
			}},
		}},
		Hotkeys:          map[string]config.Hotkey{"ctrl+alt+space": {Transcribe: config.TranscribeStage{Model: "m:x"}}},
		ToggleMaxSeconds: 1,
		InjectionMode:    config.InjectionModePaste,
	})
	// Swap the real OpenAI driver with the mock via a reflection-free
	// reinjection: we rely on the fact that BuildRegistry created the entry
	// keyed by "m", and Endpoint() returns whatever is stored. Rebuild via
	// the exported SetEndpoint helper we expose for tests.
	drivers.SetEndpointForTest(reg, "m", r.m)
	return reg
}

func baseHotkey(t config.TranscribeStage) config.Hotkey {
	return config.Hotkey{Mode: config.ModeHold, Transcribe: t}
}

func baseConfig() *config.Config {
	c := config.Default()
	c.Drivers[0].Endpoints[0].Config.APIKey = "k"
	return c
}

func TestPipeline_TranscribeInputLanguageReachesDriver(t *testing.T) {
	var sink strings.Builder
	var gotLang string
	drv := &mockDriver{
		transcribe: func(_ string, opts drivers.TranscribeOptions) (drivers.TranscribeResult, error) {
			gotLang = opts.Language
			return drivers.TranscribeResult{Text: "ok", DetectedLanguage: "ru"}, nil
		},
	}
	cfg := baseConfig()
	cfg.Hotkeys = map[string]config.Hotkey{
		"ctrl+alt+space": baseHotkey(config.TranscribeStage{Model: "m:x", InputLanguage: "ru"}),
	}
	p := newPipeline(t, drv, cfg, &sink)
	if err := p.Run(context.Background(), cfg.Hotkeys["ctrl+alt+space"]); err != nil {
		t.Fatal(err)
	}
	if gotLang != "ru" {
		t.Errorf("Language passed to driver = %q, want %q", gotLang, "ru")
	}
}

func TestPipeline_TranscribeOnly(t *testing.T) {
	var sink strings.Builder
	drv := &mockDriver{
		transcribe: func(model string, _ drivers.TranscribeOptions) (drivers.TranscribeResult, error) {
			if model != "x" {
				t.Errorf("model = %s", model)
			}
			return drivers.TranscribeResult{Text: "hello world", DetectedLanguage: "en"}, nil
		},
	}
	cfg := baseConfig()
	cfg.Hotkeys = map[string]config.Hotkey{
		"ctrl+alt+space": baseHotkey(config.TranscribeStage{Model: "m:x"}),
	}
	p := newPipeline(t, drv, cfg, &sink)

	err := p.Run(context.Background(), cfg.Hotkeys["ctrl+alt+space"])
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if sink.String() != "hello world" {
		t.Errorf("injected = %q", sink.String())
	}
}

func TestPipeline_EnglishFastPath(t *testing.T) {
	var sink strings.Builder
	drv := &mockDriver{
		translateSpeech: func(model string, _ drivers.TranslateSpeechOptions) (string, error) {
			return "translated english", nil
		},
		transcribe: func(string, drivers.TranscribeOptions) (drivers.TranscribeResult, error) {
			t.Fatal("Transcribe should not be called on English fast path")
			return drivers.TranscribeResult{}, nil
		},
	}
	cfg := baseConfig()
	cfg.Hotkeys = map[string]config.Hotkey{
		"ctrl+alt+space": baseHotkey(config.TranscribeStage{Model: "m:x", OutputLanguage: "en"}),
	}
	p := newPipeline(t, drv, cfg, &sink)

	if err := p.Run(context.Background(), cfg.Hotkeys["ctrl+alt+space"]); err != nil {
		t.Fatal(err)
	}
	if sink.String() != "translated english" {
		t.Errorf("got %q", sink.String())
	}
}

func TestPipeline_TranslateSkippedWhenLanguageMatches(t *testing.T) {
	var sink strings.Builder
	drv := &mockDriver{
		transcribe: func(string, drivers.TranscribeOptions) (drivers.TranscribeResult, error) {
			return drivers.TranscribeResult{Text: "bonjour", DetectedLanguage: "fr"}, nil
		},
		chat: func(string, string, string) (string, error) {
			t.Fatal("chat should not be called when detected == target")
			return "", nil
		},
	}
	cfg := baseConfig()
	cfg.Hotkeys = map[string]config.Hotkey{
		"ctrl+alt+space": {
			Mode:       config.ModeHold,
			Transcribe: config.TranscribeStage{Model: "m:x"},
			Translate:  &config.TranslateStage{OutputLanguage: "fr", Model: "m:x"},
		},
	}
	p := newPipeline(t, drv, cfg, &sink)
	if err := p.Run(context.Background(), cfg.Hotkeys["ctrl+alt+space"]); err != nil {
		t.Fatal(err)
	}
	if sink.String() != "bonjour" {
		t.Errorf("got %q", sink.String())
	}
}

func TestPipeline_TranslateAndEnhance(t *testing.T) {
	var sink strings.Builder
	drv := &mockDriver{
		transcribe: func(string, drivers.TranscribeOptions) (drivers.TranscribeResult, error) {
			return drivers.TranscribeResult{Text: "привет", DetectedLanguage: "ru"}, nil
		},
		chat: func(_, system, user string) (string, error) {
			if strings.Contains(system, "translator") {
				return "hello", nil
			}
			if strings.Contains(system, "text editor") {
				return user + "!", nil
			}
			t.Fatalf("unexpected system prompt: %s", system)
			return "", nil
		},
	}
	cfg := baseConfig()
	cfg.Hotkeys = map[string]config.Hotkey{
		"ctrl+alt+space": {
			Mode:       config.ModeHold,
			Transcribe: config.TranscribeStage{Model: "m:x"},
			Translate:  &config.TranslateStage{OutputLanguage: "en", Model: "m:x"},
			Enhance:    &config.EnhanceStage{Prompt: "add !", Model: "m:x"},
		},
	}
	p := newPipeline(t, drv, cfg, &sink)
	if err := p.Run(context.Background(), cfg.Hotkeys["ctrl+alt+space"]); err != nil {
		t.Fatal(err)
	}
	if sink.String() != "hello!" {
		t.Errorf("got %q", sink.String())
	}
}

func TestPipeline_ReplacementsApplied(t *testing.T) {
	var sink strings.Builder
	drv := &mockDriver{
		transcribe: func(string, drivers.TranscribeOptions) (drivers.TranscribeResult, error) {
			return drivers.TranscribeResult{Text: "write a line new line then stop", DetectedLanguage: "en"}, nil
		},
	}
	cfg := baseConfig()
	cfg.Replacements = map[string]map[string]string{
		"en": {"new line": "\n"},
	}
	cfg.Hotkeys = map[string]config.Hotkey{
		"ctrl+alt+space": {
			Mode:       config.ModeHold,
			Transcribe: config.TranscribeStage{Model: "m:x", OutputLanguage: "en"},
		},
	}
	drv.translateSpeech = func(string, drivers.TranslateSpeechOptions) (string, error) {
		return "write a line new line then stop", nil
	}
	p := newPipeline(t, drv, cfg, &sink)
	if err := p.Run(context.Background(), cfg.Hotkeys["ctrl+alt+space"]); err != nil {
		t.Fatal(err)
	}
	if sink.String() != "write a line \n then stop" {
		t.Errorf("got %q", sink.String())
	}
}

func TestPipeline_TranscribeErrorTransitionsToError(t *testing.T) {
	var sink strings.Builder
	drv := &mockDriver{
		transcribe: func(string, drivers.TranscribeOptions) (drivers.TranscribeResult, error) {
			return drivers.TranscribeResult{}, errors.New("boom")
		},
	}
	cfg := baseConfig()
	cfg.Hotkeys = map[string]config.Hotkey{
		"ctrl+alt+space": baseHotkey(config.TranscribeStage{Model: "m:x"}),
	}
	p := newPipeline(t, drv, cfg, &sink)
	err := p.Run(context.Background(), cfg.Hotkeys["ctrl+alt+space"])
	if err == nil {
		t.Fatal("expected error")
	}
	if p.Core.State() != StateError {
		t.Errorf("state = %s, want Error", p.Core.State())
	}
}
