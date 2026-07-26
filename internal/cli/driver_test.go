package cli

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/dmtrkzntsv/gosaid/internal/config"
	"github.com/dmtrkzntsv/gosaid/internal/drivers"
)

func driverFixture(t *testing.T) *config.Config {
	t.Helper()
	return &config.Config{
		Version: config.CurrentVersion,
		Drivers: []config.Driver{{
			Driver: config.DriverOpenAICompatible,
			Endpoints: []config.Endpoint{{
				ID: "openai",
				Config: config.EndpointConfig{
					APIBase: "https://api.openai.com/v1",
					APIKey:  "openai-secret",
				},
			}},
		}},
	}
}

func TestAddAndConfigureHostedEndpoint(t *testing.T) {
	cfg := driverFixture(t)
	preset, ok := findHostedDriverPreset("openrouter")
	if !ok {
		t.Fatal("openrouter preset is missing")
	}
	if err := addHostedEndpoint(cfg, preset.ID, preset.APIBase, "router-secret"); err != nil {
		t.Fatal(err)
	}

	endpoint, driverType := findEndpoint(cfg, "openrouter")
	if endpoint == nil {
		t.Fatal("openrouter endpoint was not added")
	}
	if driverType != config.DriverOpenAICompatible {
		t.Fatalf("driver type = %q, want %q", driverType, config.DriverOpenAICompatible)
	}
	if endpoint.Config.APIBase != "https://openrouter.ai/api/v1" ||
		endpoint.Config.APIKey != "router-secret" {
		t.Fatalf("unexpected endpoint: %+v", endpoint)
	}
	if endpoint.Config.TranscribeModel != "" ||
		endpoint.Config.ChatModel != "openai/gpt-5.4-nano" {
		t.Fatalf("OpenRouter model defaults = %+v", endpoint.Config)
	}

	if err := configureHostedEndpoint(
		cfg,
		"openrouter",
		"https://router.example/v1",
		"new-router-secret",
	); err != nil {
		t.Fatal(err)
	}
	endpoint, _ = findEndpoint(cfg, "openrouter")
	if endpoint.Config.APIBase != "https://router.example/v1" ||
		endpoint.Config.APIKey != "new-router-secret" {
		t.Fatalf("configured endpoint = %+v", endpoint)
	}
}

func TestAddCustomCompatibleEndpoint(t *testing.T) {
	cfg := driverFixture(t)
	err := addHostedEndpoint(
		cfg,
		"local-api",
		"http://localhost:11434/v1",
		"local-key",
	)
	if err != nil {
		t.Fatal(err)
	}
	endpoint, _ := findEndpoint(cfg, "local-api")
	if endpoint == nil || endpoint.Config.APIBase != "http://localhost:11434/v1" {
		t.Fatalf("custom endpoint not added: %+v", endpoint)
	}
}

func TestAddHostedEndpointRejectsDuplicate(t *testing.T) {
	cfg := driverFixture(t)
	err := addHostedEndpoint(
		cfg,
		"openai",
		"https://api.openai.com/v1",
		"another-key",
	)
	if err == nil {
		t.Fatal("expected duplicate endpoint error")
	}
	if len(cfg.Drivers[0].Endpoints) != 1 {
		t.Fatal("duplicate add changed the config")
	}
}

func TestAddHostedEndpointBesideIncompletePlaceholder(t *testing.T) {
	cfg := config.Default() // the programmatic default intentionally has an empty Groq key
	if err := addHostedEndpoint(
		cfg,
		"openrouter",
		"https://openrouter.ai/api/v1",
		"router-secret",
	); err != nil {
		t.Fatal(err)
	}
	if endpoint, _ := findEndpoint(cfg, "openrouter"); endpoint == nil {
		t.Fatal("openrouter endpoint was not added beside the placeholder")
	}
}

func TestSaveDriverConfig(t *testing.T) {
	cfg := driverFixture(t)
	path := filepath.Join(t.TempDir(), "config.json")
	if err := saveDriverConfig(path, cfg); err != nil {
		t.Fatal(err)
	}
	saved, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	endpoint, _ := findEndpoint(saved, "openai")
	if endpoint == nil || endpoint.Config.APIKey != "openai-secret" {
		t.Fatalf("saved endpoint = %+v", endpoint)
	}
}

func TestConfiguredDriversIncludesLocalDrivers(t *testing.T) {
	cfg := &config.Config{Drivers: []config.Driver{
		{
			Driver: config.DriverWhisperCPP,
			Endpoints: []config.Endpoint{{
				ID:     "speech",
				Config: config.EndpointConfig{Models: map[string]string{"turbo": "/tmp/turbo.bin"}},
			}},
		},
		{
			Driver: config.DriverLlamaCPP,
			Endpoints: []config.Endpoint{{
				ID:     "text",
				Config: config.EndpointConfig{Models: map[string]string{"small": "/tmp/small.gguf", "large": "/tmp/large.gguf"}},
			}},
		},
	}}
	rows := configuredDrivers(cfg)
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(rows))
	}
	if rows[0].Provider != "Local Whisper" || rows[0].Configuration != "1 model" {
		t.Fatalf("speech row = %+v", rows[0])
	}
	if rows[1].Provider != "Local Llama" || rows[1].Configuration != "2 models" {
		t.Fatalf("text row = %+v", rows[1])
	}
}

func TestFindHostedDriverPresetAliases(t *testing.T) {
	for _, name := range []string{"openai-compat", "openai-compatible", "openai_compatible"} {
		preset, ok := findHostedDriverPreset(name)
		if !ok || preset.Key != "openai-compat" {
			t.Errorf("%q resolved to %+v, %v", name, preset, ok)
		}
	}
}

func TestPredefinedHostedAPIBase(t *testing.T) {
	for _, apiBase := range []string{
		"https://api.openai.com/v1",
		"https://openrouter.ai/api/v1/",
	} {
		if !isPredefinedHostedAPIBase(apiBase) {
			t.Errorf("%q should be predefined", apiBase)
		}
	}
	if isPredefinedHostedAPIBase("http://localhost:11434/v1") {
		t.Error("custom compatible API should not be predefined")
	}
}

func TestDeleteEndpointPrunesEmptyDriverGroup(t *testing.T) {
	cfg := driverFixture(t)
	if err := addHostedEndpoint(
		cfg,
		"openrouter",
		"https://openrouter.ai/api/v1",
		"router-secret",
	); err != nil {
		t.Fatal(err)
	}

	if err := deleteEndpoint(cfg, "openai"); err != nil {
		t.Fatal(err)
	}
	if endpoint, _ := findEndpoint(cfg, "openai"); endpoint != nil {
		t.Fatal("openai endpoint still exists")
	}
	if len(cfg.Drivers) != 1 || len(cfg.Drivers[0].Endpoints) != 1 {
		t.Fatalf("driver group was pruned too early: %+v", cfg.Drivers)
	}

	if err := deleteEndpoint(cfg, "openrouter"); err != nil {
		t.Fatal(err)
	}
	if len(cfg.Drivers) != 0 {
		t.Fatalf("empty driver group was not pruned: %+v", cfg.Drivers)
	}
	if err := deleteEndpoint(cfg, "missing"); err == nil {
		t.Fatal("expected an error deleting an unknown driver")
	}
}

func TestHotkeysUsingEndpoint(t *testing.T) {
	cfg := &config.Config{Hotkeys: map[string]config.Hotkey{
		"cmd+shift+a": {
			Transcribe: config.TranscribeStage{Model: "speech:turbo"},
			Enhance:    &config.EnhanceStage{Model: "openai:gpt-5.4-mini"},
		},
		"cmd+shift+b": {
			Transcribe: config.TranscribeStage{Model: "openai:whisper-1"},
		},
		"cmd+shift+c": {
			Transcribe: config.TranscribeStage{Model: "openai-backup:whisper-1"},
		},
	}}
	got := hotkeysUsingEndpoint(cfg, "openai")
	if len(got) != 2 || got[0] != "cmd+shift+a" || got[1] != "cmd+shift+b" {
		t.Fatalf("hotkeys using openai = %v", got)
	}
}

func TestDriverFormKeyMapSupportsArrowNavigation(t *testing.T) {
	keymap := driverFormKeyMap()
	if !slices.Contains(keymap.Input.Next.Keys(), "down") {
		t.Fatalf("input next keys = %v, want down arrow", keymap.Input.Next.Keys())
	}
	if !slices.Contains(keymap.Input.Prev.Keys(), "up") {
		t.Fatalf("input previous keys = %v, want up arrow", keymap.Input.Prev.Keys())
	}
}

func TestSubmitDriverInputEnterSubmitsAndDownDoesNot(t *testing.T) {
	emptyValue := ""
	submitted := false
	validate := requireDriverValue("api key")
	downInput := &submitDriverInput{
		Input: huh.NewInput().
			Value(&emptyValue).
			Validate(validate),
		submitted: &submitted,
		validate:  validate,
	}
	downInput.WithKeyMap(driverFormKeyMap())

	if _, cmd := downInput.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown})); cmd == nil {
		t.Fatal("down arrow should move to Back")
	}
	if submitted {
		t.Fatal("down arrow must not submit the driver")
	}
	if err := downInput.Error(); err != nil {
		t.Fatalf("down arrow triggered validation: %v", err)
	}

	value := "secret"
	enterInput := &submitDriverInput{
		Input: huh.NewInput().
			Value(&value).
			Validate(validate),
		submitted: &submitted,
		validate:  validate,
	}
	enterInput.WithKeyMap(driverFormKeyMap())
	if _, cmd := enterInput.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter})); cmd == nil {
		t.Fatal("enter should submit the driver")
	}
	if !submitted {
		t.Fatal("enter did not mark the driver form submitted")
	}
}

func TestCredentialFormDownFocusesBack(t *testing.T) {
	apiKey := ""
	submitted := false
	backChoice := "back"
	validate := requireDriverValue("api key")
	input := &submitDriverInput{
		Input: huh.NewInput().
			Title("OpenRouter API key").
			Value(&apiKey).
			Validate(validate),
		submitted: &submitted,
		validate:  validate,
	}
	form := huh.NewForm(huh.NewGroup(
		input,
		newDriverBackSelect(&backChoice),
	)).
		WithKeyMap(driverFormKeyMap()).
		WithTheme(huh.ThemeFunc(driverCredentialTheme))

	var model huh.Model = form
	model = runHuhCommands(model, form.Init())
	model, cmd := model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	model = runHuhCommands(model, cmd)

	view := ansi.Strip(model.View())
	if !strings.Contains(view, "> Back") {
		t.Fatalf("down arrow did not focus Back:\n%s", view)
	}
	if strings.Contains(view, "api key is required") {
		t.Fatalf("down arrow triggered API-key validation:\n%s", view)
	}
}

func TestDriverCredentialThemeHighlightsBackOnlyWhenFocused(t *testing.T) {
	for _, isDark := range []bool{false, true} {
		styles := driverCredentialTheme(isDark)
		blurred := styles.Blurred.SelectedOption.GetForeground()
		neutral := styles.Blurred.UnselectedOption.GetForeground()
		focused := styles.Focused.SelectedOption.GetForeground()
		if blurred != neutral {
			t.Errorf("dark=%v: blurred Back color = %v, want neutral %v", isDark, blurred, neutral)
		}
		if focused == blurred {
			t.Errorf("dark=%v: focused Back color should differ from blurred color %v", isDark, blurred)
		}
	}
}

type fakeHostedModelTester struct {
	gotTranscribeModel string
	gotChatModel       string
	transcribeErr      error
	chatErr            error
}

func (f *fakeHostedModelTester) Transcribe(
	_ context.Context,
	_ []float32,
	_ int,
	model string,
	_ drivers.TranscribeOptions,
) (drivers.TranscribeResult, error) {
	f.gotTranscribeModel = model
	return drivers.TranscribeResult{}, f.transcribeErr
}

func (f *fakeHostedModelTester) Chat(
	_ context.Context,
	model, _, _ string,
) (string, error) {
	f.gotChatModel = model
	return "OK", f.chatErr
}

func TestHostedModelsAreTestedBeforeAdd(t *testing.T) {
	tester := &fakeHostedModelTester{}
	if err := testHostedModelsWithDriver(
		context.Background(),
		tester,
		"speech-model",
		"chat-model",
	); err != nil {
		t.Fatal(err)
	}
	if tester.gotTranscribeModel != "speech-model" || tester.gotChatModel != "chat-model" {
		t.Fatalf(
			"tested transcription=%q chat=%q",
			tester.gotTranscribeModel,
			tester.gotChatModel,
		)
	}
}

func TestHostedModelTestRejectsMissingOrFailingModels(t *testing.T) {
	if err := testHostedModelsWithDriver(
		context.Background(),
		&fakeHostedModelTester{},
		"",
		"",
	); err == nil {
		t.Fatal("missing model names should fail without a request")
	}

	err := testHostedModelsWithDriver(
		context.Background(),
		&fakeHostedModelTester{chatErr: errors.New("model not found")},
		"",
		"missing-chat",
	)
	if err == nil || !strings.Contains(err.Error(), `chat model "missing-chat"`) {
		t.Fatalf("failure = %v", err)
	}
}

func runHuhCommands(model huh.Model, cmd tea.Cmd) huh.Model {
	for steps := 0; cmd != nil && steps < 20; steps++ {
		msg := cmd()
		if batch, ok := msg.(tea.BatchMsg); ok {
			for _, batchCmd := range batch {
				model = runHuhCommands(model, batchCmd)
			}
			return model
		}
		model, cmd = model.Update(msg)
	}
	return model
}

func TestRunDriverRejectsSubcommands(t *testing.T) {
	if code := RunDriver([]string{"list"}); code != 2 {
		t.Fatalf("gosaid driver list exit = %d, want 2", code)
	}
}
