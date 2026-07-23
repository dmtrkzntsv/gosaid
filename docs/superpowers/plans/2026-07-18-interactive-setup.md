# Interactive Setup CLI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** An interactive `gosaid setup` command (hub + `hotkey`/`provider`/`model`/`mic` topics) that manages config.json without hand-editing, per the approved spec at `docs/superpowers/specs/2026-07-18-interactive-setup-design.md`.

**Architecture:** New package `internal/setup` split into a pure layer (option builders, config mutators — fully unit-tested) and thin `charm.land/huh/v2` form wiring (untested, by design). Model-download internals move from `internal/cli` into a new `internal/models` package so both the CLI and setup share them. One additive config field (global `microphone`); the daemon gains a two-line resolution rule.

**Tech Stack:** Go 1.25, `charm.land/huh/v2` (forms), `golang.org/x/term` (TTY detection). Module path `github.com/dmtrkzntsv/gosaid`.

## Global Constraints

- Commands after this plan: `gosaid setup [hotkey|provider|model|mic]`, `gosaid config` (unchanged), `gosaid model download` (unchanged). `gosaid mic` is **removed**.
- Provider preset order: **Local Whisper first**, then OpenAI, Groq, OpenRouter, DeepSeek, Together, Custom last.
- Nothing writes config.json until the finish step; model file downloads are the only immediate side effect.
- Endpoint ids must never contain `:` (model refs split on the first colon).
- All new pure logic gets table-driven tests; `huh` form wiring gets none.
- Run tests with `go test ./...` (CGO whisper build makes the first run slow — that's normal). Format with `go fmt ./...`, vet with `go vet ./...`.
- Commit after every task with the message given in the task.

---

### Task 1: Global default microphone

**Files:**
- Modify: `internal/config/config.go` (add field + method, update per-hotkey doc comment)
- Modify: `internal/daemon/daemon.go:140-148` (resolution)
- Test: `internal/config/config_test.go` (append)

**Interfaces:**
- Consumes: existing `config.Config`, `config.Hotkey`.
- Produces: `Config.Microphone string` field (JSON `microphone,omitempty`) and `func (c *Config) MicrophoneFor(hk Hotkey) string`. Later tasks call `MicrophoneFor` never re-implement the fallback.

- [ ] **Step 1: Write the failing test** — append to `internal/config/config_test.go`:

```go
func TestMicrophoneFor(t *testing.T) {
	cfg := &Config{Microphone: "MacBook"}
	if got := cfg.MicrophoneFor(Hotkey{}); got != "MacBook" {
		t.Errorf("global fallback: got %q, want %q", got, "MacBook")
	}
	if got := cfg.MicrophoneFor(Hotkey{Microphone: "AirPods"}); got != "AirPods" {
		t.Errorf("hotkey override: got %q, want %q", got, "AirPods")
	}
	if got := (&Config{}).MicrophoneFor(Hotkey{}); got != "" {
		t.Errorf("both empty: got %q, want system default (empty)", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/config/ -run TestMicrophoneFor -v`
Expected: FAIL — `cfg.MicrophoneFor undefined`

- [ ] **Step 3: Implement.** In `internal/config/config.go`, add to the `Config` struct (after `LogLevel`):

```go
	// Microphone is the default input device for all hotkeys, matched
	// case-insensitively as a substring of the device name. Empty uses the
	// system default. A hotkey's own Microphone field overrides this.
	Microphone string `json:"microphone,omitempty"`
```

Update the per-hotkey `Microphone` doc comment (currently references `gosaid mic list`, which Task 5 removes):

```go
	// Microphone selects the input device for this hotkey by name —
	// a case-insensitive substring match (pick devices interactively with
	// `gosaid setup mic`). Empty falls back to the global Microphone
	// setting, then the system default. If the device is absent when
	// recording starts, capture falls back to the default (logged).
	Microphone string `json:"microphone,omitempty"`
```

Add the method after the `Config` struct:

```go
// MicrophoneFor resolves the input device for a hotkey: the hotkey's own
// Microphone if set, else the global default, else "" (system default).
func (c *Config) MicrophoneFor(hk Hotkey) string {
	if hk.Microphone != "" {
		return hk.Microphone
	}
	return c.Microphone
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/config/ -run TestMicrophoneFor -v`
Expected: PASS

- [ ] **Step 5: Wire the daemon.** In `internal/daemon/daemon.go` inside the `for combo, hk := range cfg.Hotkeys` loop's `OnTrigger` closure, replace the three uses of `hk.Microphone`:

```go
				mic := cfg.MicrophoneFor(hk)
				opened, err := capturer.Start(mic)
				if err != nil {
					core.Transition(StateError, err)
					return
				}
				if mic != "" && !audio.MatchesDevice(opened, mic) {
					log.Warn("configured microphone not found — using fallback",
						"combo", combo, "want", mic, "using", opened)
				}
```

- [ ] **Step 6: Full test run + commit**

Run: `go test ./... && go vet ./...`
Expected: PASS

```bash
git add internal/config/config.go internal/config/config_test.go internal/daemon/daemon.go
git commit -m "feat: global default microphone with per-hotkey override"
```

---

### Task 2: Extract `internal/models` package

Move the model download/registration internals out of `internal/cli` so `internal/setup` can use them without an import cycle (cli → setup, so setup must not import cli). Add an unregister helper and the curated model catalog.

**Files:**
- Create: `internal/models/models.go` (moved internals, exported)
- Create: `internal/models/catalog.go`
- Create: `internal/models/models_test.go` (moved from `internal/cli/model_test.go` + new tests)
- Modify: `internal/cli/model.go` (becomes a thin wrapper)
- Delete: `internal/cli/model_test.go` (content moves)

**Interfaces:**
- Consumes: `config.Config`, `config.Load/Save`, `config.DriverWhisperCPP`.
- Produces (package `models`, import `github.com/dmtrkzntsv/gosaid/internal/models`):
  - `type DownloadOpts struct { Repo, File, Name, EndpointID, CfgPath, ModelsDir, BaseURL string; Force bool }`
  - `func Download(o DownloadOpts) error` — full operation: load config, checks, fetch, register, save (exact behavior of today's `modelDownload`).
  - `func FetchModelFile(baseURL, repo, file, modelsDir string, force bool) (dest string, size int64, err error)` — download only, no config touch. Returns the existing dest without downloading when the file exists and `!force`.
  - `func Register(cfg *config.Config, endpointID, name, path string)`
  - `func Unregister(cfg *config.Config, endpointID, name string) (path string, ok bool)` — removes the model; prunes the endpoint when its models map becomes empty and the whisper_cpp driver block when it has no endpoints left (an empty models map fails validation).
  - `func RegisteredModels(cfg *config.Config, endpointID string) map[string]string` — nil when the endpoint doesn't exist.
  - `func DeriveName(file string) string` — `"ggml-base.bin"` → `"base"`.
  - `const CatalogRepo = "ggerganov/whisper.cpp"`, `const HuggingFaceBase = "https://huggingface.co"`
  - `type CatalogEntry struct { Name, File, Size string }`, `var Catalog []CatalogEntry`

- [ ] **Step 1: Move the internals.** Create `internal/models/models.go` with package clause `package models`, importing `fmt`, `io`, `net/http`, `os`, `path/filepath`, `strings`, and `github.com/dmtrkzntsv/gosaid/internal/config`. Move from `internal/cli/model.go`, renaming:

| old (cli, unexported) | new (models, exported) |
|---|---|
| `modelDownloadOpts` | `DownloadOpts` (fields exported: `Repo, File, Name, EndpointID, CfgPath, ModelsDir, BaseURL string; Force bool`) |
| `modelDownload` | `Download` |
| `deriveModelName` | `DeriveName` |
| `registerModel` | `Register` |
| `findWhisperModel` | keep unexported as `findModel` (superseded by `RegisteredModels`) |
| `otherDriverForEndpoint` | keep unexported |
| `fetchToFile`, `progressReader` | keep unexported |

Restructure `Download` to route its fetch through the new `FetchModelFile`:

```go
// FetchModelFile downloads repo/file from baseURL into modelsDir, returning
// the destination path. When the file already exists and force is false it
// returns the existing path with size 0 and no download.
func FetchModelFile(baseURL, repo, file, modelsDir string, force bool) (string, int64, error) {
	if err := os.MkdirAll(modelsDir, 0o755); err != nil {
		return "", 0, err
	}
	dest := filepath.Join(modelsDir, filepath.Base(file))
	if _, err := os.Stat(dest); err == nil && !force {
		return dest, 0, nil
	}
	url := fmt.Sprintf("%s/%s/resolve/main/%s", baseURL, repo, file)
	size, err := fetchToFile(url, dest)
	if err != nil {
		return "", 0, err
	}
	return dest, size, nil
}
```

`Download` keeps its exact current semantics — including erroring (not silently reusing) when the file exists without `Force`:

```go
func Download(o DownloadOpts) error {
	cfg, err := config.Load(o.CfgPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if driver := otherDriverForEndpoint(cfg, o.EndpointID); driver != "" {
		return fmt.Errorf("endpoint %q already exists with driver %q; choose a different --endpoint id",
			o.EndpointID, driver)
	}
	if existing := findModel(cfg, o.EndpointID, o.Name); existing != "" && !o.Force {
		return fmt.Errorf("model %q is already registered on endpoint %q → %s (use --force to overwrite)",
			o.Name, o.EndpointID, existing)
	}
	if err := os.MkdirAll(o.ModelsDir, 0o755); err != nil {
		return err
	}
	dest := filepath.Join(o.ModelsDir, filepath.Base(o.File))
	if _, err := os.Stat(dest); err == nil && !o.Force {
		return fmt.Errorf("file already exists: %s (use --force to overwrite)", dest)
	}
	url := fmt.Sprintf("%s/%s/resolve/main/%s", o.BaseURL, o.Repo, o.File)
	size, err := fetchToFile(url, dest)
	if err != nil {
		return err
	}
	Register(cfg, o.EndpointID, o.Name, dest)
	if err := config.Save(o.CfgPath, cfg); err != nil {
		return fmt.Errorf("update config: %w", err)
	}
	fmt.Printf("downloaded %s (%.1f MB)\nregistered model %q on endpoint %q\n\nuse it in a hotkey:\n  \"transcribe\": { \"model\": \"%s:%s\" }\n",
		dest, float64(size)/(1<<20), o.Name, o.EndpointID, o.EndpointID, o.Name)
	return nil
}
```

- [ ] **Step 2: Thin the CLI wrapper.** `internal/cli/model.go` keeps only `RunModel` (flag parsing unchanged) and calls:

```go
			err = models.Download(models.DownloadOpts{
				Repo: rest[0], File: rest[1], Name: *name, EndpointID: *endpoint,
				CfgPath: cfgPath, ModelsDir: modelsDir,
				BaseURL: models.HuggingFaceBase, Force: *force,
			})
```

with `*name` defaulted via `models.DeriveName(rest[1])`. Delete everything else from the file.

- [ ] **Step 3: Move the tests.** Create `internal/models/models_test.go` from `internal/cli/model_test.go`: package `models`, rename `deriveModelName`→`DeriveName`, `modelDownload`→`Download`, `modelDownloadOpts`→`DownloadOpts` with exported field names. Keep `TestRunModelRejectsInterleavedFlagAfterName` and `TestRunModelRejectsFlagAsFilePositional` in a new small `internal/cli/model_test.go` (they test `RunModel` flag parsing). Delete the old file content otherwise.

- [ ] **Step 4: Write the failing tests for the new helpers** — append to `internal/models/models_test.go`:

```go
func TestUnregisterPrunesEmptyEndpointAndDriver(t *testing.T) {
	cfg := config.Default()
	Register(cfg, "local", "base", "/models/ggml-base.bin")
	Register(cfg, "local", "tiny", "/models/ggml-tiny.bin")

	path, ok := Unregister(cfg, "local", "base")
	if !ok || path != "/models/ggml-base.bin" {
		t.Fatalf("Unregister = %q, %v", path, ok)
	}
	if got := RegisteredModels(cfg, "local"); len(got) != 1 || got["tiny"] == "" {
		t.Fatalf("RegisteredModels after first removal = %v", got)
	}

	if _, ok := Unregister(cfg, "local", "tiny"); !ok {
		t.Fatal("second Unregister failed")
	}
	if got := RegisteredModels(cfg, "local"); got != nil {
		t.Fatalf("endpoint should be pruned, RegisteredModels = %v", got)
	}
	for _, d := range cfg.Drivers {
		if d.Driver == config.DriverWhisperCPP {
			t.Fatal("empty whisper_cpp driver block should be pruned")
		}
	}

	if _, ok := Unregister(cfg, "local", "missing"); ok {
		t.Fatal("Unregister of unknown model must return ok=false")
	}
}

func TestFetchModelFileSkipsExisting(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "ggml-base.bin")
	if err := os.WriteFile(existing, []byte("OLD"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Server that fails the test if contacted.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("server should not be contacted for an existing file")
	}))
	t.Cleanup(srv.Close)
	dest, size, err := FetchModelFile(srv.URL, "ggerganov/whisper.cpp", "ggml-base.bin", dir, false)
	if err != nil || dest != existing || size != 0 {
		t.Fatalf("FetchModelFile = %q, %d, %v", dest, size, err)
	}
}

func TestCatalog(t *testing.T) {
	if len(Catalog) < 6 {
		t.Fatalf("catalog too small: %d entries", len(Catalog))
	}
	for _, e := range Catalog {
		if e.Name == "" || e.File == "" || e.Size == "" {
			t.Errorf("incomplete entry: %+v", e)
		}
		if DeriveName(e.File) != e.Name {
			t.Errorf("entry %q: DeriveName(%q) = %q, want match", e.Name, e.File, DeriveName(e.File))
		}
	}
}
```

- [ ] **Step 5: Run to verify failures**

Run: `go test ./internal/models/ -v`
Expected: FAIL — `Unregister`, `RegisteredModels`, `FetchModelFile`, `Catalog` undefined (moved tests should pass once the package compiles).

- [ ] **Step 6: Implement the helpers.** In `internal/models/models.go`:

```go
// RegisteredModels returns the models map for a whisper_cpp endpoint, or nil.
func RegisteredModels(cfg *config.Config, endpointID string) map[string]string {
	for _, d := range cfg.Drivers {
		if d.Driver != config.DriverWhisperCPP {
			continue
		}
		for _, e := range d.Endpoints {
			if e.ID == endpointID {
				return e.Config.Models
			}
		}
	}
	return nil
}

// Unregister removes a model from a whisper_cpp endpoint, returning the
// removed file path. Endpoints left with no models are removed (an empty
// models map fails validation), as are driver blocks left with no endpoints.
func Unregister(cfg *config.Config, endpointID, name string) (string, bool) {
	for di := range cfg.Drivers {
		d := &cfg.Drivers[di]
		if d.Driver != config.DriverWhisperCPP {
			continue
		}
		for ei := range d.Endpoints {
			e := &d.Endpoints[ei]
			if e.ID != endpointID {
				continue
			}
			path, ok := e.Config.Models[name]
			if !ok {
				return "", false
			}
			delete(e.Config.Models, name)
			if len(e.Config.Models) == 0 {
				d.Endpoints = append(d.Endpoints[:ei], d.Endpoints[ei+1:]...)
			}
			if len(d.Endpoints) == 0 {
				cfg.Drivers = append(cfg.Drivers[:di], cfg.Drivers[di+1:]...)
			}
			return path, true
		}
	}
	return "", false
}
```

And `internal/models/catalog.go`:

```go
package models

const (
	// CatalogRepo is the official whisper.cpp GGML model repository on
	// Hugging Face; every curated catalog entry downloads from it.
	CatalogRepo     = "ggerganov/whisper.cpp"
	HuggingFaceBase = "https://huggingface.co"
)

// CatalogEntry is one curated local Whisper model. Size is a human label
// shown in the picker, not used programmatically.
type CatalogEntry struct {
	Name string // registered model name ("base") and DeriveName(File)
	File string // file inside CatalogRepo ("ggml-base.bin")
	Size string // approximate download size ("~148 MB")
}

var Catalog = []CatalogEntry{
	{Name: "tiny", File: "ggml-tiny.bin", Size: "~78 MB"},
	{Name: "base", File: "ggml-base.bin", Size: "~148 MB"},
	{Name: "small", File: "ggml-small.bin", Size: "~488 MB"},
	{Name: "medium", File: "ggml-medium.bin", Size: "~1.5 GB"},
	{Name: "large-v3", File: "ggml-large-v3.bin", Size: "~3.1 GB"},
	{Name: "large-v3-turbo", File: "ggml-large-v3-turbo.bin", Size: "~1.6 GB"},
}
```

- [ ] **Step 7: Run all tests + commit**

Run: `go test ./... && go vet ./...`
Expected: PASS (including the moved download tests and remaining cli flag tests)

```bash
git add internal/models internal/cli/model.go internal/cli/model_test.go
git commit -m "refactor: extract model download internals into internal/models with catalog"
```

---

### Task 3: Setup pure layer — presets, summaries, option builders

**Files:**
- Create: `internal/setup/presets.go`
- Create: `internal/setup/summary.go`
- Create: `internal/setup/options.go`
- Modify: `internal/config/languages.go` (add `Languages()`)
- Test: `internal/setup/presets_test.go`, `internal/setup/summary_test.go`, `internal/setup/options_test.go`, `internal/config/languages_test.go` (create)

**Interfaces:**
- Consumes: `config.Config`, `config.Hotkey`, stage `IsEnabled()`, `config.LanguageName`, `routing.ParseModelRef`, `models.RegisteredModels`.
- Produces (package `setup`):
  - `type ProviderPreset struct { Key, Label, APIBase, TranscribeModel, ChatModel string; Local, Custom bool }`
  - `var ProviderPresets []ProviderPreset` — order: local, openai, groq, openrouter, deepseek, together, custom.
  - `func PresetForAPIBase(apiBase string) *ProviderPreset` — nil when no match.
  - `func HotkeySummary(combo string, hk config.Hotkey) string` — `"option+right · hold · transcribe → enhance → translate(en)"`.
  - `func EndpointSummary(driver string, e config.Endpoint) string` — `"openai · api.openai.com"` / `"local · whisper_cpp (2 models)"`.
  - `var SuggestedCombos []string`
  - `type ModelOption struct { Label, Ref string }`
  - `func TranscribeModelOptions(cfg *config.Config) []ModelOption`
  - `func ChatModelOptions(cfg *config.Config) []ModelOption`
  - `func OpenAIEndpointIDs(cfg *config.Config) []string` (sorted)
  - `config.Languages() []string` (sorted codes)

- [ ] **Step 1: Write failing tests.** `internal/config/languages_test.go`:

```go
package config

import "testing"

func TestLanguagesSortedAndValid(t *testing.T) {
	langs := Languages()
	if len(langs) != len(languageNames) {
		t.Fatalf("Languages() returned %d codes, want %d", len(langs), len(languageNames))
	}
	for i, c := range langs {
		if !IsValidLanguage(c) {
			t.Errorf("invalid code %q", c)
		}
		if i > 0 && langs[i-1] >= c {
			t.Errorf("not sorted at %d: %q >= %q", i, langs[i-1], c)
		}
	}
}
```

`internal/setup/presets_test.go`:

```go
package setup

import "testing"

func TestProviderPresetsOrderAndShape(t *testing.T) {
	if ProviderPresets[0].Key != "local" || !ProviderPresets[0].Local {
		t.Fatalf("first preset must be local whisper, got %+v", ProviderPresets[0])
	}
	last := ProviderPresets[len(ProviderPresets)-1]
	if last.Key != "custom" || !last.Custom {
		t.Fatalf("last preset must be custom, got %+v", last)
	}
	for _, p := range ProviderPresets {
		if p.Local || p.Custom {
			continue
		}
		if p.APIBase == "" || p.ChatModel == "" {
			t.Errorf("cloud preset %q must have APIBase and ChatModel: %+v", p.Key, p)
		}
	}
}

func TestPresetForAPIBase(t *testing.T) {
	if p := PresetForAPIBase("https://api.openai.com/v1"); p == nil || p.Key != "openai" {
		t.Fatalf("openai base not matched: %+v", p)
	}
	if p := PresetForAPIBase("https://example.com/v1"); p != nil {
		t.Fatalf("unknown base must return nil, got %+v", p)
	}
}
```

`internal/setup/summary_test.go`:

```go
package setup

import (
	"testing"

	"github.com/dmtrkzntsv/gosaid/internal/config"
)

func TestHotkeySummary(t *testing.T) {
	hk := config.Hotkey{
		Mode:       config.ModeHold,
		Transcribe: config.TranscribeStage{Model: "openai:whisper-1"},
		Enhance:    &config.EnhanceStage{Model: "openai:gpt-5.4-nano"},
		Translate:  &config.TranslateStage{OutputLanguage: "en", Model: "openai:gpt-5.4-nano"},
	}
	want := "option+right · hold · transcribe → enhance → translate(en)"
	if got := HotkeySummary("option+right", hk); got != want {
		t.Errorf("got %q\nwant %q", got, want)
	}

	plain := config.Hotkey{Transcribe: config.TranscribeStage{Model: "local:base"}}
	want = "option+left · hold · transcribe"
	if got := HotkeySummary("option+left", plain); got != want {
		t.Errorf("empty mode defaults to hold: got %q, want %q", got, want)
	}

	comp := config.Hotkey{
		Mode:       config.ModeToggle,
		Transcribe: config.TranscribeStage{Model: "openai:whisper-1"},
		Compose:    &config.ComposeStage{Model: "openai:gpt-5.4-nano"},
	}
	want = "option+up · toggle · transcribe → compose"
	if got := HotkeySummary("option+up", comp); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestEndpointSummary(t *testing.T) {
	cloud := config.Endpoint{ID: "openai", Config: config.EndpointConfig{APIBase: "https://api.openai.com/v1", APIKey: "sk-x"}}
	if got := EndpointSummary(config.DriverOpenAICompatible, cloud); got != "openai · api.openai.com" {
		t.Errorf("got %q", got)
	}
	local := config.Endpoint{ID: "local", Config: config.EndpointConfig{Models: map[string]string{"base": "/m/b.bin", "tiny": "/m/t.bin"}}}
	if got := EndpointSummary(config.DriverWhisperCPP, local); got != "local · whisper_cpp (2 models)" {
		t.Errorf("got %q", got)
	}
	one := config.Endpoint{ID: "local", Config: config.EndpointConfig{Models: map[string]string{"base": "/m/b.bin"}}}
	if got := EndpointSummary(config.DriverWhisperCPP, one); got != "local · whisper_cpp (1 model)" {
		t.Errorf("got %q", got)
	}
}
```

`internal/setup/options_test.go`:

```go
package setup

import (
	"testing"

	"github.com/dmtrkzntsv/gosaid/internal/config"
)

// testConfig returns a config with one openai preset endpoint and one local
// whisper endpoint with two models.
func testConfig() *config.Config {
	return &config.Config{
		Version: config.CurrentVersion,
		Drivers: []config.Driver{
			{Driver: config.DriverOpenAICompatible, Endpoints: []config.Endpoint{
				{ID: "openai", Config: config.EndpointConfig{APIBase: "https://api.openai.com/v1", APIKey: "sk-x"}},
			}},
			{Driver: config.DriverWhisperCPP, Endpoints: []config.Endpoint{
				{ID: "local", Config: config.EndpointConfig{Models: map[string]string{
					"base": "/m/ggml-base.bin", "tiny": "/m/ggml-tiny.bin",
				}}},
			}},
		},
		Hotkeys:          map[string]config.Hotkey{},
		ToggleMaxSeconds: 60,
	}
}

func TestTranscribeModelOptions(t *testing.T) {
	opts := TranscribeModelOptions(testConfig())
	refs := map[string]bool{}
	for _, o := range opts {
		refs[o.Ref] = true
	}
	for _, want := range []string{"local:base", "local:tiny", "openai:whisper-1"} {
		if !refs[want] {
			t.Errorf("missing option %q in %v", want, opts)
		}
	}
}

func TestChatModelOptions(t *testing.T) {
	opts := ChatModelOptions(testConfig())
	if len(opts) != 1 || opts[0].Ref != "openai:gpt-5.4-nano" {
		t.Fatalf("got %v, want single openai chat option", opts)
	}
	// whisper_cpp endpoints must never appear as chat options.
	for _, o := range opts {
		if o.Ref == "local:base" || o.Ref == "local:tiny" {
			t.Errorf("whisper_cpp leaked into chat options: %v", o)
		}
	}
}

func TestOpenAIEndpointIDs(t *testing.T) {
	got := OpenAIEndpointIDs(testConfig())
	if len(got) != 1 || got[0] != "openai" {
		t.Fatalf("got %v", got)
	}
}

func TestSuggestedCombosParse(t *testing.T) {
	if len(SuggestedCombos) < 4 {
		t.Fatalf("want a useful list, got %v", SuggestedCombos)
	}
}
```

- [ ] **Step 2: Run to verify failures**

Run: `go test ./internal/setup/ ./internal/config/ -v 2>&1 | head -30`
Expected: FAIL — package setup doesn't compile / symbols undefined.

- [ ] **Step 3: Implement.** `internal/config/languages.go` — add (import `"sort"`):

```go
// Languages returns all known language codes, sorted.
func Languages() []string {
	codes := make([]string, 0, len(languageNames))
	for c := range languageNames {
		codes = append(codes, c)
	}
	sort.Strings(codes)
	return codes
}
```

`internal/setup/presets.go`:

```go
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
// Custom last (per the design spec).
var ProviderPresets = []ProviderPreset{
	{Key: "local", Label: "Local Whisper (on-device, no API key)", Local: true},
	{Key: "openai", Label: "OpenAI", APIBase: "https://api.openai.com/v1",
		TranscribeModel: "whisper-1", ChatModel: "gpt-5.4-nano"},
	{Key: "groq", Label: "Groq", APIBase: "https://api.groq.com/openai/v1",
		TranscribeModel: "whisper-large-v3-turbo", ChatModel: "llama-3.3-70b-versatile"},
	{Key: "openrouter", Label: "OpenRouter", APIBase: "https://openrouter.ai/api/v1",
		TranscribeModel: "", ChatModel: "openai/gpt-5.4-nano"},
	{Key: "deepseek", Label: "DeepSeek", APIBase: "https://api.deepseek.com/v1",
		TranscribeModel: "", ChatModel: "deepseek-chat"},
	{Key: "together", Label: "Together", APIBase: "https://api.together.xyz/v1",
		TranscribeModel: "", ChatModel: "meta-llama/Llama-3.3-70B-Instruct-Turbo"},
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
```

`internal/setup/summary.go`:

```go
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
```

`internal/setup/options.go`:

```go
package setup

import (
	"fmt"
	"sort"

	"github.com/dmtrkzntsv/gosaid/internal/config"
)

// SuggestedCombos is the curated key-combo list for the hotkey wizard. All
// entries parse with the hotkey package on every platform ("option" and
// "alt" are aliases everywhere).
var SuggestedCombos = []string{
	"option+left", "option+right", "option+up", "option+down",
	"ctrl+alt+space", "ctrl+alt+enter",
	"option+f8", "option+f9", "option+f10",
}

// ModelOption is one selectable model reference.
type ModelOption struct {
	Label string
	Ref   string // "endpoint:model"
}

// TranscribeModelOptions lists every model the transcribe stage can use:
// each whisper_cpp model, plus the preset-suggested transcription model of
// each recognized openai_compatible endpoint.
func TranscribeModelOptions(cfg *config.Config) []ModelOption {
	var out []ModelOption
	for _, d := range cfg.Drivers {
		for _, e := range d.Endpoints {
			switch d.Driver {
			case config.DriverWhisperCPP:
				names := make([]string, 0, len(e.Config.Models))
				for name := range e.Config.Models {
					names = append(names, name)
				}
				sort.Strings(names)
				for _, name := range names {
					out = append(out, ModelOption{
						Label: fmt.Sprintf("%s (local)", name),
						Ref:   e.ID + ":" + name,
					})
				}
			case config.DriverOpenAICompatible:
				if p := PresetForAPIBase(e.Config.APIBase); p != nil && p.TranscribeModel != "" {
					out = append(out, ModelOption{
						Label: fmt.Sprintf("%s (%s)", p.TranscribeModel, e.ID),
						Ref:   e.ID + ":" + p.TranscribeModel,
					})
				}
			}
		}
	}
	return out
}

// ChatModelOptions lists the preset-suggested chat model of each recognized
// openai_compatible endpoint. whisper_cpp endpoints never appear (they are
// transcription-only).
func ChatModelOptions(cfg *config.Config) []ModelOption {
	var out []ModelOption
	for _, d := range cfg.Drivers {
		if d.Driver != config.DriverOpenAICompatible {
			continue
		}
		for _, e := range d.Endpoints {
			if p := PresetForAPIBase(e.Config.APIBase); p != nil && p.ChatModel != "" {
				out = append(out, ModelOption{
					Label: fmt.Sprintf("%s (%s)", p.ChatModel, e.ID),
					Ref:   e.ID + ":" + p.ChatModel,
				})
			}
		}
	}
	return out
}

// OpenAIEndpointIDs returns the ids of all openai_compatible endpoints,
// sorted — the free-text model path lets the user pick one and type a model
// id when no preset suggestion exists.
func OpenAIEndpointIDs(cfg *config.Config) []string {
	var out []string
	for _, d := range cfg.Drivers {
		if d.Driver != config.DriverOpenAICompatible {
			continue
		}
		for _, e := range d.Endpoints {
			out = append(out, e.ID)
		}
	}
	sort.Strings(out)
	return out
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/setup/ ./internal/config/ -v 2>&1 | tail -20`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/setup internal/config/languages.go internal/config/languages_test.go
git commit -m "feat: setup pure layer — provider presets, summaries, option builders"
```

---

### Task 4: Setup pure layer — mutators, recipes, first-run

**Files:**
- Create: `internal/setup/apply.go`
- Create: `internal/setup/recipe.go`
- Test: `internal/setup/apply_test.go`, `internal/setup/recipe_test.go`

**Interfaces:**
- Consumes: `config` types, `routing.ParseModelRef`, `models.RegisteredModels`.
- Produces (package `setup`):
  - `func UpsertHotkey(cfg *config.Config, combo string, hk config.Hotkey)`
  - `func DeleteHotkey(cfg *config.Config, combo string)`
  - `func EndpointIDInUse(cfg *config.Config, id string) bool`
  - `func AddOpenAIEndpoint(cfg *config.Config, id, apiBase, apiKey string) error` — errors on empty/`:`-containing/duplicate id; appends to the openai_compatible driver block, creating it if absent.
  - `func UpdateOpenAIEndpoint(cfg *config.Config, id, apiBase, apiKey string) error`
  - `func DeleteEndpoint(cfg *config.Config, id string) error` — removes from any driver; prunes empty driver blocks.
  - `func HotkeysUsingEndpoint(cfg *config.Config, id string) []string` (sorted combos)
  - `func HotkeysUsingModel(cfg *config.Config, endpointID, model string) []string` (sorted combos; transcribe stage refs)
  - `func ReassignEndpoint(cfg *config.Config, fromID, toID string)` — rewrites stage refs from one endpoint to another, substituting `toID`'s preset-suggested model per stage kind when known, else keeping the model name.
  - `func SetDefaultMicrophone(cfg *config.Config, name string)`
  - Recipes: `const RecipeTranscribe/RecipeCleanup/RecipeTranslate/RecipeCompose`, `type HotkeyAnswers struct { Combo, Mode, Recipe, TranscribeRef, ChatRef, TargetLang, Instructions, Microphone string }`, `func RecipeOf(hk config.Hotkey) string`, `func BuildHotkey(a HotkeyAnswers) config.Hotkey`, `func AnswersFrom(combo string, hk config.Hotkey) HotkeyAnswers`
  - `func FirstRun(cfg *config.Config) bool` — true when no usable endpoint exists (no whisper_cpp endpoints, and every openai_compatible endpoint has an empty or `"REPLACE_ME"` api key — the shipped example config counts as first-run).
  - `func ResetForFirstRun(cfg *config.Config)` — clears `Drivers` and `Hotkeys`, keeps global settings, so the guided chain builds a clean config.
  - `type ModelDiff struct { Add, Remove []string }`, `func DiffModelSelection(registered map[string]string, selected []string) ModelDiff` (sorted slices)

- [ ] **Step 1: Write failing tests.** `internal/setup/apply_test.go`:

```go
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
```

`internal/setup/recipe_test.go`:

```go
package setup

import (
	"testing"

	"github.com/dmtrkzntsv/gosaid/internal/config"
)

func TestRecipeOf(t *testing.T) {
	cases := []struct {
		name string
		hk   config.Hotkey
		want string
	}{
		{"plain", config.Hotkey{}, RecipeTranscribe},
		{"cleanup", config.Hotkey{Enhance: &config.EnhanceStage{Model: "x:y"}}, RecipeCleanup},
		{"translate wins over enhance", config.Hotkey{
			Enhance:   &config.EnhanceStage{Model: "x:y"},
			Translate: &config.TranslateStage{OutputLanguage: "en", Model: "x:y"},
		}, RecipeTranslate},
		{"compose", config.Hotkey{Compose: &config.ComposeStage{Model: "x:y"}}, RecipeCompose},
	}
	for _, c := range cases {
		if got := RecipeOf(c.hk); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

func TestBuildHotkeyRoundTrip(t *testing.T) {
	a := HotkeyAnswers{
		Combo: "option+right", Mode: "hold", Recipe: RecipeTranslate,
		TranscribeRef: "openai:whisper-1", ChatRef: "openai:gpt-5.4-nano",
		TargetLang: "en",
	}
	hk := BuildHotkey(a)
	if hk.Transcribe.Model != "openai:whisper-1" {
		t.Errorf("transcribe = %q", hk.Transcribe.Model)
	}
	if !hk.Enhance.IsEnabled() || hk.Enhance.Model != "openai:gpt-5.4-nano" {
		t.Errorf("translate recipe must include enhance (matches example config): %+v", hk.Enhance)
	}
	if !hk.Translate.IsEnabled() || hk.Translate.OutputLanguage != "en" {
		t.Errorf("translate stage: %+v", hk.Translate)
	}
	if hk.Compose != nil {
		t.Error("compose must be absent")
	}

	back := AnswersFrom("option+right", hk)
	if back.Recipe != RecipeTranslate || back.ChatRef != "openai:gpt-5.4-nano" ||
		back.TargetLang != "en" || back.TranscribeRef != "openai:whisper-1" {
		t.Errorf("AnswersFrom lost data: %+v", back)
	}
}

func TestBuildHotkeyCompose(t *testing.T) {
	hk := BuildHotkey(HotkeyAnswers{
		Combo: "option+up", Mode: "hold", Recipe: RecipeCompose,
		TranscribeRef: "openai:whisper-1", ChatRef: "openai:gpt-5.4-nano",
		Instructions: "Formal register.",
	})
	if !hk.Compose.IsEnabled() || hk.Compose.Instructions != "Formal register." {
		t.Errorf("compose: %+v", hk.Compose)
	}
	if hk.Enhance != nil || hk.Translate != nil {
		t.Error("compose recipe must not add enhance/translate")
	}
}
```

- [ ] **Step 2: Run to verify failures**

Run: `go test ./internal/setup/ 2>&1 | head -20`
Expected: FAIL — symbols undefined.

- [ ] **Step 3: Implement.** `internal/setup/apply.go`:

```go
package setup

import (
	"fmt"
	"sort"
	"strings"

	"github.com/dmtrkzntsv/gosaid/internal/config"
	"github.com/dmtrkzntsv/gosaid/internal/routing"
)

// UpsertHotkey adds or replaces a hotkey binding.
func UpsertHotkey(cfg *config.Config, combo string, hk config.Hotkey) {
	if cfg.Hotkeys == nil {
		cfg.Hotkeys = map[string]config.Hotkey{}
	}
	cfg.Hotkeys[combo] = hk
}

// DeleteHotkey removes a hotkey binding.
func DeleteHotkey(cfg *config.Config, combo string) {
	delete(cfg.Hotkeys, combo)
}

// EndpointIDInUse reports whether any driver owns an endpoint with this id.
func EndpointIDInUse(cfg *config.Config, id string) bool {
	for _, d := range cfg.Drivers {
		for _, e := range d.Endpoints {
			if e.ID == id {
				return true
			}
		}
	}
	return false
}

func validateEndpointID(cfg *config.Config, id string) error {
	if id == "" {
		return fmt.Errorf("endpoint id is required")
	}
	if strings.Contains(id, ":") {
		return fmt.Errorf("endpoint id must not contain ':' (model references are 'endpoint:model')")
	}
	if EndpointIDInUse(cfg, id) {
		return fmt.Errorf("endpoint id %q already exists", id)
	}
	return nil
}

// AddOpenAIEndpoint appends an openai_compatible endpoint, creating the
// driver block if absent.
func AddOpenAIEndpoint(cfg *config.Config, id, apiBase, apiKey string) error {
	if err := validateEndpointID(cfg, id); err != nil {
		return err
	}
	ep := config.Endpoint{ID: id, Config: config.EndpointConfig{APIBase: apiBase, APIKey: apiKey}}
	for di := range cfg.Drivers {
		if cfg.Drivers[di].Driver == config.DriverOpenAICompatible {
			cfg.Drivers[di].Endpoints = append(cfg.Drivers[di].Endpoints, ep)
			return nil
		}
	}
	cfg.Drivers = append(cfg.Drivers, config.Driver{
		Driver:    config.DriverOpenAICompatible,
		Endpoints: []config.Endpoint{ep},
	})
	return nil
}

// UpdateOpenAIEndpoint replaces api_base and api_key of an existing endpoint.
func UpdateOpenAIEndpoint(cfg *config.Config, id, apiBase, apiKey string) error {
	for di := range cfg.Drivers {
		if cfg.Drivers[di].Driver != config.DriverOpenAICompatible {
			continue
		}
		for ei := range cfg.Drivers[di].Endpoints {
			e := &cfg.Drivers[di].Endpoints[ei]
			if e.ID == id {
				e.Config.APIBase = apiBase
				e.Config.APIKey = apiKey
				return nil
			}
		}
	}
	return fmt.Errorf("no openai_compatible endpoint %q", id)
}

// DeleteEndpoint removes an endpoint from whichever driver owns it, pruning
// driver blocks left without endpoints.
func DeleteEndpoint(cfg *config.Config, id string) error {
	for di := range cfg.Drivers {
		d := &cfg.Drivers[di]
		for ei := range d.Endpoints {
			if d.Endpoints[ei].ID != id {
				continue
			}
			d.Endpoints = append(d.Endpoints[:ei], d.Endpoints[ei+1:]...)
			if len(d.Endpoints) == 0 {
				cfg.Drivers = append(cfg.Drivers[:di], cfg.Drivers[di+1:]...)
			}
			return nil
		}
	}
	return fmt.Errorf("no endpoint %q", id)
}

// stageRefs returns pointers to every model-ref field of a hotkey, keyed by
// stage kind ("transcribe" or "chat"). Used by the reassignment rewriter.
func stageRefs(hk *config.Hotkey) map[*string]string {
	refs := map[*string]string{&hk.Transcribe.Model: "transcribe"}
	if hk.Enhance != nil {
		refs[&hk.Enhance.Model] = "chat"
	}
	if hk.Translate != nil {
		refs[&hk.Translate.Model] = "chat"
	}
	if hk.Compose != nil {
		refs[&hk.Compose.Model] = "chat"
	}
	return refs
}

// HotkeysUsingEndpoint returns the sorted combos of hotkeys with any stage
// referencing the endpoint.
func HotkeysUsingEndpoint(cfg *config.Config, id string) []string {
	var out []string
	for combo, hk := range cfg.Hotkeys {
		hk := hk
		for ref := range stageRefs(&hk) {
			if m, err := routing.ParseModelRef(*ref); err == nil && m.Endpoint == id {
				out = append(out, combo)
				break
			}
		}
	}
	sort.Strings(out)
	return out
}

// HotkeysUsingModel returns the sorted combos of hotkeys whose transcribe
// stage references endpointID:model.
func HotkeysUsingModel(cfg *config.Config, endpointID, model string) []string {
	want := endpointID + ":" + model
	var out []string
	for combo, hk := range cfg.Hotkeys {
		if hk.Transcribe.Model == want {
			out = append(out, combo)
		}
	}
	sort.Strings(out)
	return out
}

// ReassignEndpoint rewrites every stage ref pointing at fromID to point at
// toID. When toID's api_base matches a preset, the stage-appropriate
// suggested model is substituted; otherwise the model name is kept.
func ReassignEndpoint(cfg *config.Config, fromID, toID string) {
	var preset *ProviderPreset
	for _, d := range cfg.Drivers {
		if d.Driver != config.DriverOpenAICompatible {
			continue
		}
		for _, e := range d.Endpoints {
			if e.ID == toID {
				preset = PresetForAPIBase(e.Config.APIBase)
			}
		}
	}
	for combo, hk := range cfg.Hotkeys {
		for ref, kind := range stageRefs(&hk) {
			m, err := routing.ParseModelRef(*ref)
			if err != nil || m.Endpoint != fromID {
				continue
			}
			model := m.Model
			if preset != nil {
				if kind == "transcribe" && preset.TranscribeModel != "" {
					model = preset.TranscribeModel
				} else if kind == "chat" && preset.ChatModel != "" {
					model = preset.ChatModel
				}
			}
			*ref = toID + ":" + model
		}
		cfg.Hotkeys[combo] = hk
	}
}

// SetDefaultMicrophone sets the global default input device ("" = system
// default).
func SetDefaultMicrophone(cfg *config.Config, name string) {
	cfg.Microphone = name
}

// FirstRun reports whether no usable provider is configured yet. The shipped
// example config (api_key "REPLACE_ME") counts as first-run.
func FirstRun(cfg *config.Config) bool {
	for _, d := range cfg.Drivers {
		for _, e := range d.Endpoints {
			switch d.Driver {
			case config.DriverWhisperCPP:
				return false
			case config.DriverOpenAICompatible:
				if e.Config.APIKey != "" && e.Config.APIKey != "REPLACE_ME" {
					return false
				}
			}
		}
	}
	return true
}

// ResetForFirstRun clears drivers and hotkeys so the guided first-run chain
// builds a clean config, keeping global settings (toggle window, sounds,
// injection mode, log level, user context).
func ResetForFirstRun(cfg *config.Config) {
	cfg.Drivers = nil
	cfg.Hotkeys = map[string]config.Hotkey{}
}

// ModelDiff is the outcome of the local-model multi-select: names to
// download+register and names to unregister.
type ModelDiff struct {
	Add    []string
	Remove []string
}

// DiffModelSelection compares the registered model names with the picker
// selection. Both output slices are sorted.
func DiffModelSelection(registered map[string]string, selected []string) ModelDiff {
	sel := map[string]bool{}
	for _, s := range selected {
		sel[s] = true
	}
	var d ModelDiff
	for _, s := range selected {
		if _, ok := registered[s]; !ok {
			d.Add = append(d.Add, s)
		}
	}
	for name := range registered {
		if !sel[name] {
			d.Remove = append(d.Remove, name)
		}
	}
	sort.Strings(d.Add)
	sort.Strings(d.Remove)
	return d
}
```

Note: `stageRefs` takes `*config.Hotkey` and callers in `HotkeysUsingEndpoint` copy the loop variable before taking pointers — keep that pattern (map values are not addressable).

`internal/setup/recipe.go`:

```go
package setup

import (
	"github.com/dmtrkzntsv/gosaid/internal/config"
)

// Recipes are the four hotkey pipelines offered by the wizard, mirroring the
// shipped example config.
const (
	RecipeTranscribe = "transcribe" // transcribe only
	RecipeCleanup    = "cleanup"    // transcribe → enhance
	RecipeTranslate  = "translate"  // transcribe → enhance → translate
	RecipeCompose    = "compose"    // transcribe → compose
)

// HotkeyAnswers is everything the hotkey wizard collects. BuildHotkey turns
// it into a config.Hotkey; AnswersFrom inverts that for the edit flow.
type HotkeyAnswers struct {
	Combo         string
	Mode          string // "hold" or "toggle"
	Recipe        string
	TranscribeRef string // "endpoint:model"
	ChatRef       string // "endpoint:model"; empty for RecipeTranscribe
	TargetLang    string // RecipeTranslate only
	Instructions  string // RecipeCompose only
	Microphone    string // per-hotkey override; usually empty
}

// RecipeOf classifies an existing hotkey. Compose wins over translate wins
// over enhance (a hotkey with both translate and enhance is the translate
// recipe — that's how the wizard builds them).
func RecipeOf(hk config.Hotkey) string {
	switch {
	case hk.Compose.IsEnabled():
		return RecipeCompose
	case hk.Translate.IsEnabled():
		return RecipeTranslate
	case hk.Enhance.IsEnabled():
		return RecipeCleanup
	default:
		return RecipeTranscribe
	}
}

// BuildHotkey materializes wizard answers into a hotkey.
func BuildHotkey(a HotkeyAnswers) config.Hotkey {
	hk := config.Hotkey{
		Mode:       config.HotkeyMode(a.Mode),
		Microphone: a.Microphone,
		Transcribe: config.TranscribeStage{Model: a.TranscribeRef},
	}
	switch a.Recipe {
	case RecipeCleanup:
		hk.Enhance = &config.EnhanceStage{Model: a.ChatRef}
	case RecipeTranslate:
		hk.Enhance = &config.EnhanceStage{Model: a.ChatRef}
		hk.Translate = &config.TranslateStage{OutputLanguage: a.TargetLang, Model: a.ChatRef}
	case RecipeCompose:
		hk.Compose = &config.ComposeStage{Model: a.ChatRef, Instructions: a.Instructions}
	}
	return hk
}

// AnswersFrom pre-fills the wizard from an existing hotkey (edit flow).
func AnswersFrom(combo string, hk config.Hotkey) HotkeyAnswers {
	mode := hk.Mode
	if mode == "" {
		mode = config.ModeHold
	}
	a := HotkeyAnswers{
		Combo:         combo,
		Mode:          string(mode),
		Recipe:        RecipeOf(hk),
		TranscribeRef: hk.Transcribe.Model,
		Microphone:    hk.Microphone,
	}
	switch a.Recipe {
	case RecipeCleanup:
		a.ChatRef = hk.Enhance.Model
	case RecipeTranslate:
		a.ChatRef = hk.Translate.Model
		a.TargetLang = hk.Translate.OutputLanguage
	case RecipeCompose:
		a.ChatRef = hk.Compose.Model
		a.Instructions = hk.Compose.Instructions
	}
	return a
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/setup/ -v 2>&1 | tail -20`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/setup
git commit -m "feat: setup pure layer — config mutators, recipes, first-run detection"
```

---

### Task 5: Setup runtime — session, dispatch, mic flow (first vertical slice)

Adds the `huh` dependency, the `Session` load/save wrapper, TTY guard, `gosaid setup [topic]` dispatch, and the complete mic flow. Removes `gosaid mic`. After this task, `gosaid setup mic` works end to end.

**Files:**
- Create: `internal/setup/session.go`
- Create: `internal/setup/run.go`
- Create: `internal/setup/flow_mic.go`
- Modify: `internal/cli/cli.go` (dispatch `setup`, remove `mic`, update `Usage`)
- Delete: `internal/cli/mic.go`
- Test: `internal/setup/session_test.go`, `internal/cli/cli_test.go` (create)

**Interfaces:**
- Consumes: `config.Path/Load/Save/Validate`, `audio.ListCaptureDevices() ([]audio.CaptureDevice, error)` (fields `Name string`, `Default bool`), pure layer from Tasks 3–4.
- Produces:
  - `type Session struct { Path string; Cfg *config.Config; Dirty bool }`
  - `func LoadSession() (*Session, error)`
  - `func (s *Session) Save() error` — `config.Validate` then `config.Save`.
  - `func Run(args []string) int` — entry point called by cli. Topics: `""` (hub), `hotkey`, `provider`, `model`, `mic`. Unknown topic → usage message, exit 2. Non-TTY stdin → error, exit 1.
  - Flow functions used by later tasks: each flow has signature `func(s *Session) error` and sets `s.Dirty` on changes. This task ships `runMicFlow`; later tasks add `runModelFlow`, `runProviderFlow`, `runHotkeyFlow`, `runHub` with the same shape.
  - `func finish(s *Session) error` — no-op when clean; saves when dirty; prints the restart hint (Task 6 upgrades it to the interactive offer).
  - `func confirmDiscardOnAbort(s *Session, err error) (abort bool, ferr error)` — helper handling `huh.ErrUserAborted`.

- [ ] **Step 1: Add dependencies**

```bash
go get charm.land/huh/v2@latest
go get golang.org/x/term@latest
go mod tidy
```

Expected: go.mod gains `charm.land/huh/v2` (plus charm transitive deps) and `golang.org/x/term`.

- [ ] **Step 2: Write failing session test.** `internal/setup/session_test.go`:

```go
package setup

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dmtrkzntsv/gosaid/internal/config"
)

func TestSessionSaveValidates(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	s := &Session{Path: path, Cfg: &config.Config{}, Dirty: true}
	if err := s.Save(); err == nil {
		t.Fatal("saving an invalid (empty) config must fail validation")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("invalid config must not be written to disk")
	}
}
```

(A full valid-save round trip is impractical here — validation stats whisper model files — but the invalid path proves Save validates before writing; `config.Save` is already covered by config tests.)

- [ ] **Step 3: Run to verify failure**

Run: `go test ./internal/setup/ -run TestSessionSaveValidates -v`
Expected: FAIL — `Session` undefined.

- [ ] **Step 4: Implement session + run + mic flow.** `internal/setup/session.go`:

```go
package setup

import (
	"github.com/dmtrkzntsv/gosaid/internal/config"
)

// Session is one interactive setup run: the loaded config, its path, and
// whether anything changed. All flows mutate Cfg in memory; nothing is
// written until finish() calls Save.
type Session struct {
	Path  string
	Cfg   *config.Config
	Dirty bool
}

// LoadSession resolves the config path and loads (or creates) the config.
func LoadSession() (*Session, error) {
	path, err := config.Path()
	if err != nil {
		return nil, err
	}
	cfg, err := config.Load(path)
	if err != nil {
		return nil, err
	}
	return &Session{Path: path, Cfg: cfg}, nil
}

// Save validates and writes the config atomically. An invalid config is
// never written.
func (s *Session) Save() error {
	if err := config.Validate(s.Cfg); err != nil {
		return err
	}
	return config.Save(s.Path, s.Cfg)
}
```

`internal/setup/run.go`:

```go
package setup

import (
	"errors"
	"fmt"
	"os"

	"charm.land/huh/v2"
	"golang.org/x/term"
)

const setupUsage = "usage: gosaid setup [hotkey|provider|model|mic]"

// Run is the `gosaid setup` entry point. An empty args runs the hub; a topic
// arg jumps straight to that manager and then to save.
func Run(args []string) int {
	topic := ""
	if len(args) > 0 {
		topic = args[0]
	}
	var flow func(*Session) error
	switch topic {
	case "":
		flow = runHub
	case "hotkey":
		flow = runHotkeyFlow
	case "provider":
		flow = runProviderFlow
	case "model":
		flow = runModelFlow
	case "mic":
		flow = runMicFlow
	default:
		fmt.Fprintf(os.Stderr, "unknown setup topic: %s\n%s\n", topic, setupUsage)
		return 2
	}
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		fmt.Fprintln(os.Stderr, "error: gosaid setup requires an interactive terminal")
		return 1
	}
	s, err := LoadSession()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	if err := flow(s); err != nil {
		if abort, ferr := confirmDiscardOnAbort(s, err); abort {
			if ferr != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", ferr)
				return 1
			}
			fmt.Println("Changes discarded.")
			return 0
		} else if !errors.Is(err, huh.ErrUserAborted) {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 1
		}
	}
	if err := finish(s); err != nil {
		fmt.Fprintf(os.Stderr, "error: config not saved: %v\n", err)
		return 1
	}
	return 0
}

// confirmDiscardOnAbort handles Ctrl+C/Esc out of a form. With no unsaved
// changes it's a plain exit. With changes, ask: discarding returns
// abort=true; declining returns abort=false so the caller proceeds to save.
func confirmDiscardOnAbort(s *Session, err error) (bool, error) {
	if !errors.Is(err, huh.ErrUserAborted) {
		return false, nil
	}
	if !s.Dirty {
		return true, nil
	}
	discard := false
	cerr := huh.NewForm(huh.NewGroup(
		huh.NewConfirm().
			Title("Discard unsaved changes?").
			Affirmative("Discard").Negative("Save them").
			Value(&discard),
	)).Run()
	if cerr != nil {
		return true, nil // second abort: discard
	}
	return discard, nil
}

// finish writes the config when anything changed and tells the user how to
// apply it. Task 6 replaces the hint with an interactive restart offer.
func finish(s *Session) error {
	if !s.Dirty {
		fmt.Println("No changes.")
		return nil
	}
	if err := s.Save(); err != nil {
		return err
	}
	fmt.Printf("Saved %s\n", s.Path)
	fmt.Println("Restart the daemon to apply changes (e.g. `brew services restart gosaid`).")
	return nil
}
```

Temporary stubs in `run.go` so the package compiles until Tasks 7–10 replace them (each later task deletes its stub):

```go
func runHub(s *Session) error      { return fmt.Errorf("hub not implemented yet") }
func runHotkeyFlow(s *Session) error   { return fmt.Errorf("hotkey manager not implemented yet") }
func runProviderFlow(s *Session) error { return fmt.Errorf("provider manager not implemented yet") }
func runModelFlow(s *Session) error    { return fmt.Errorf("model manager not implemented yet") }
```

`internal/setup/flow_mic.go`:

```go
package setup

import (
	"charm.land/huh/v2"

	"github.com/dmtrkzntsv/gosaid/internal/audio"
)

// micOptions builds the device picker: "System default" first, then every
// capture device (the current system default labeled).
func micOptions() ([]huh.Option[string], error) {
	devices, err := audio.ListCaptureDevices()
	if err != nil {
		return nil, err
	}
	opts := []huh.Option[string]{huh.NewOption("System default", "")}
	for _, d := range devices {
		label := d.Name
		if d.Default {
			label += " (system default)"
		}
		opts = append(opts, huh.NewOption(label, d.Name))
	}
	return opts, nil
}

// runMicFlow selects the global default microphone.
func runMicFlow(s *Session) error {
	opts, err := micOptions()
	if err != nil {
		return err
	}
	choice := s.Cfg.Microphone
	form := huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().
			Title("Default microphone").
			Description("Used by every hotkey unless the hotkey sets its own microphone.").
			Options(opts...).
			Value(&choice),
	))
	if err := form.Run(); err != nil {
		return err
	}
	if choice != s.Cfg.Microphone {
		SetDefaultMicrophone(s.Cfg, choice)
		s.Dirty = true
	}
	return nil
}
```

- [ ] **Step 5: Wire the CLI.** In `internal/cli/cli.go`: add `"github.com/dmtrkzntsv/gosaid/internal/setup"` import; in the switch, add `case "setup": return setup.Run(args[1:])` and **delete** `case "mic"`. Delete `internal/cli/mic.go`. Replace `Usage()`:

```go
func Usage() {
	fmt.Println(`gosaid - headless push-to-talk voice dictation daemon

usage:
  gosaid           run the daemon
  gosaid setup     interactive configuration (hotkeys, providers, models, mic)
  gosaid setup hotkey|provider|model|mic
                   jump straight to one setup topic
  gosaid config    open the config file in $EDITOR
  gosaid model download <hf-repo> <file>
                   download a GGML model from Hugging Face and register it
  gosaid version   print version
  gosaid help      print this message`)
}
```

- [ ] **Step 6: Write failing CLI test.** `internal/cli/cli_test.go`:

```go
package cli

import "testing"

func TestDispatchMicRemoved(t *testing.T) {
	if code := Dispatch("test", []string{"mic", "list"}); code != 2 {
		t.Fatalf("gosaid mic must be an unknown command now, exit = %d", code)
	}
}

func TestDispatchSetupUnknownTopic(t *testing.T) {
	if code := Dispatch("test", []string{"setup", "bogus"}); code != 2 {
		t.Fatalf("unknown setup topic must exit 2, got %d", code)
	}
}

func TestDispatchSetupNonTTY(t *testing.T) {
	// go test runs with a non-TTY stdin, so any valid topic must refuse.
	if code := Dispatch("test", []string{"setup", "mic"}); code != 1 {
		t.Fatalf("setup without a TTY must exit 1, got %d", code)
	}
}
```

Note the ordering requirement this encodes: `Run` must reject an unknown topic (exit 2) **before** the TTY check, or `TestDispatchSetupUnknownTopic` fails — the `run.go` code above already does this.

- [ ] **Step 7: Run all tests**

Run: `go test ./... && go vet ./...`
Expected: PASS

- [ ] **Step 8: Manual smoke test**

Run: `go build -o /tmp/gosaid ./cmd/gosaid && /tmp/gosaid setup mic`
Expected: arrow-key device picker appears; choosing a device prints `Saved …` and the restart hint; choosing System default when it was already default prints `No changes.`; Ctrl+C prints `Changes discarded.` (after the confirm, if dirty).

- [ ] **Step 9: Commit**

```bash
git add go.mod go.sum internal/setup internal/cli
git commit -m "feat: gosaid setup entry point with default-mic flow; remove gosaid mic"
```

---

### Task 6: Restart offer

**Files:**
- Create: `internal/setup/restart.go`
- Create: `internal/setup/restart_unix.go`, `internal/setup/restart_windows.go`
- Modify: `internal/setup/run.go` (`finish` calls `offerRestart`)
- Test: `internal/setup/restart_test.go`

**Interfaces:**
- Consumes: `daemon.StateFilePath()`, `daemon.StateFile` (fields `State string`, `PID int`).
- Produces: `func daemonRunningAt(statePath string) bool` (pure-ish, tested), `func offerRestart()` (wiring), `func restartHint() string`, `func startHint() string`.

- [ ] **Step 1: Write failing test.** `internal/setup/restart_test.go`:

```go
package setup

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestDaemonRunningAt(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "state.json")
	if daemonRunningAt(missing) {
		t.Fatal("missing state file = not running")
	}

	garbage := filepath.Join(dir, "garbage.json")
	os.WriteFile(garbage, []byte("{not json"), 0o644)
	if daemonRunningAt(garbage) {
		t.Fatal("unparseable state file = not running")
	}

	// Our own test process pid is definitely alive.
	alive := filepath.Join(dir, "alive.json")
	os.WriteFile(alive, []byte(fmt.Sprintf(`{"state":"idle","pid":%d}`, os.Getpid())), 0o644)
	if !daemonRunningAt(alive) {
		t.Fatal("state file with a live pid = running")
	}

	// A stale file with an almost-certainly-dead pid.
	dead := filepath.Join(dir, "dead.json")
	os.WriteFile(dead, []byte(`{"state":"idle","pid":99999999}`), 0o644)
	if daemonRunningAt(dead) {
		t.Fatal("dead pid = not running (crash left a stale file)")
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/setup/ -run TestDaemonRunningAt -v`
Expected: FAIL — `daemonRunningAt` undefined.

- [ ] **Step 3: Implement.** `internal/setup/restart.go`:

```go
package setup

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"

	"charm.land/huh/v2"

	"github.com/dmtrkzntsv/gosaid/internal/daemon"
)

// daemonRunningAt reports whether the state file at path describes a live
// daemon. A missing/garbled file or a dead PID (crash leftover) means no.
func daemonRunningAt(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var st daemon.StateFile
	if json.Unmarshal(data, &st) != nil {
		return false
	}
	return st.PID > 0 && pidAlive(st.PID)
}

func daemonRunning() bool {
	path, err := daemon.StateFilePath()
	if err != nil {
		return false
	}
	return daemonRunningAt(path)
}

func restartHint() string {
	switch runtime.GOOS {
	case "windows":
		return "stop the running gosaid.exe and start it again"
	default:
		return "brew services restart gosaid  (or restart your gosaid process)"
	}
}

func startHint() string {
	switch runtime.GOOS {
	case "windows":
		return "gosaid.exe"
	default:
		return "brew services start gosaid  (or run `gosaid` directly)"
	}
}

// offerRestart is called after a successful save. With a running daemon it
// offers an automatic restart (brew when available); otherwise it prints how
// to start one.
func offerRestart() {
	if !daemonRunning() {
		fmt.Println("The daemon is not running. Start it with:\n  " + startHint())
		return
	}
	restart := true
	err := huh.NewForm(huh.NewGroup(
		huh.NewConfirm().
			Title("Restart the daemon to apply changes?").
			Value(&restart),
	)).Run()
	if err != nil || !restart {
		fmt.Println("Restart the daemon later to apply changes:\n  " + restartHint())
		return
	}
	if brew, lerr := exec.LookPath("brew"); lerr == nil {
		cmd := exec.Command(brew, "services", "restart", "gosaid")
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if cmd.Run() == nil {
			fmt.Println("Daemon restarted.")
			return
		}
	}
	fmt.Println("Could not restart automatically. Restart manually:\n  " + restartHint())
}
```

`internal/setup/restart_unix.go`:

```go
//go:build !windows

package setup

import (
	"os"
	"syscall"
)

// pidAlive reports whether a process with this pid exists (signal 0 probe).
func pidAlive(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return p.Signal(syscall.Signal(0)) == nil
}
```

`internal/setup/restart_windows.go`:

```go
//go:build windows

package setup

import "os"

// pidAlive on Windows: FindProcess only succeeds for live processes.
func pidAlive(pid int) bool {
	_, err := os.FindProcess(pid)
	return err == nil
}
```

In `run.go`, replace the two hint lines at the end of `finish` with:

```go
	fmt.Printf("Saved %s\n", s.Path)
	offerRestart()
	return nil
```

- [ ] **Step 4: Run tests + commit**

Run: `go test ./internal/setup/ -v -run TestDaemonRunningAt && go test ./... && go vet ./...`
Expected: PASS

```bash
git add internal/setup
git commit -m "feat: offer daemon restart after setup saves changes"
```

---

### Task 7: Local model manager flow

**Files:**
- Create: `internal/setup/flow_model.go`
- Modify: `internal/setup/run.go` (delete the `runModelFlow` stub)

**Interfaces:**
- Consumes: `models.Catalog`, `models.CatalogRepo`, `models.HuggingFaceBase`, `models.FetchModelFile`, `models.Register`, `models.Unregister`, `models.RegisteredModels`, `models.DeriveName`, `platform.ModelsDir()`, `DiffModelSelection`, `HotkeysUsingModel`.
- Produces: `func runModelFlow(s *Session) error`. The local endpoint id is always `"local"` (matches `gosaid model download`'s default).

- [ ] **Step 1: Implement** `internal/setup/flow_model.go`:

```go
package setup

import (
	"fmt"
	"sort"
	"strings"

	"charm.land/huh/v2"

	"github.com/dmtrkzntsv/gosaid/internal/models"
	"github.com/dmtrkzntsv/gosaid/internal/platform"
)

// localEndpointID is the whisper_cpp endpoint managed by setup; the same
// default `gosaid model download` uses.
const localEndpointID = "local"

// runModelFlow is the local Whisper model manager: a multi-select over the
// curated catalog (plus any custom-registered models), a custom-model
// prompt, and immediate download / deferred-save semantics per the spec.
func runModelFlow(s *Session) error {
	registered := models.RegisteredModels(s.Cfg, localEndpointID)

	// Catalog entries plus registered models outside the catalog.
	opts := make([]huh.Option[string], 0, len(models.Catalog)+2)
	inCatalog := map[string]bool{}
	for _, e := range models.Catalog {
		inCatalog[e.Name] = true
		opts = append(opts, huh.NewOption(
			fmt.Sprintf("%s · %s", e.Name, e.Size), e.Name,
		).Selected(registered[e.Name] != ""))
	}
	var customNames []string
	for name := range registered {
		if !inCatalog[name] {
			customNames = append(customNames, name)
		}
	}
	sort.Strings(customNames)
	for _, name := range customNames {
		opts = append(opts, huh.NewOption(name+" · custom", name).Selected(true))
	}

	var selected []string
	for name := range registered {
		selected = append(selected, name)
	}
	addCustom := false
	form := huh.NewForm(huh.NewGroup(
		huh.NewMultiSelect[string]().
			Title("Local Whisper models").
			Description("Checked models are kept; unchecking removes them. New checks download from Hugging Face.").
			Options(opts...).
			Value(&selected),
		huh.NewConfirm().
			Title("Add a custom model from Hugging Face?").
			Value(&addCustom),
	))
	if err := form.Run(); err != nil {
		return err
	}

	diff := DiffModelSelection(registered, selected)
	modelsDir, err := platform.ModelsDir()
	if err != nil {
		return err
	}

	for _, name := range diff.Add {
		var file string
		for _, e := range models.Catalog {
			if e.Name == name {
				file = e.File
			}
		}
		if file == "" {
			continue // custom models can only be unchecked, never re-added here
		}
		fmt.Printf("Downloading %s…\n", name)
		dest, _, err := models.FetchModelFile(models.HuggingFaceBase, models.CatalogRepo, file, modelsDir, false)
		if err != nil {
			fmt.Printf("Download failed for %s: %v — skipped.\n", name, err)
			continue
		}
		models.Register(s.Cfg, localEndpointID, name, dest)
		s.Dirty = true
	}

	var removedPaths []string
	for _, name := range diff.Remove {
		if refs := HotkeysUsingModel(s.Cfg, localEndpointID, name); len(refs) > 0 {
			proceed := false
			err := huh.NewForm(huh.NewGroup(
				huh.NewConfirm().
					Title(fmt.Sprintf("Remove %q? These hotkeys use it and will break: %s",
						name, strings.Join(refs, ", "))).
					Affirmative("Remove anyway").Negative("Keep it").
					Value(&proceed),
			)).Run()
			if err != nil {
				return err
			}
			if !proceed {
				continue
			}
		}
		if path, ok := models.Unregister(s.Cfg, localEndpointID, name); ok {
			removedPaths = append(removedPaths, path)
			s.Dirty = true
		}
	}
	if len(removedPaths) > 0 {
		deleteFiles := true
		err := huh.NewForm(huh.NewGroup(
			huh.NewConfirm().
				Title(fmt.Sprintf("Also delete %d model file(s) from disk?", len(removedPaths))).
				Description("Model files are large; keeping them lets you re-add without re-downloading.").
				Value(&deleteFiles),
		)).Run()
		if err != nil {
			return err
		}
		if deleteFiles {
			for _, p := range removedPaths {
				if err := osRemove(p); err != nil {
					fmt.Printf("Could not delete %s: %v\n", p, err)
				}
			}
		}
	}

	if addCustom {
		if err := runCustomModelPrompt(s, modelsDir); err != nil {
			return err
		}
	}
	return nil
}

// runCustomModelPrompt downloads and registers one model outside the catalog.
func runCustomModelPrompt(s *Session, modelsDir string) error {
	var repo, file string
	form := huh.NewForm(huh.NewGroup(
		huh.NewInput().
			Title("Hugging Face repository").
			Placeholder("ggerganov/whisper.cpp").
			Validate(requireNonEmpty("repository")).
			Value(&repo),
		huh.NewInput().
			Title("Model file").
			Placeholder("ggml-large-v3-q5_0.bin").
			Validate(requireNonEmpty("file")).
			Value(&file),
	))
	if err := form.Run(); err != nil {
		return err
	}
	name := models.DeriveName(file)
	fmt.Printf("Downloading %s…\n", name)
	dest, _, err := models.FetchModelFile(models.HuggingFaceBase, repo, file, modelsDir, false)
	if err != nil {
		fmt.Printf("Download failed: %v\n", err)
		return nil
	}
	models.Register(s.Cfg, localEndpointID, name, dest)
	s.Dirty = true
	return nil
}

// requireNonEmpty is a shared huh validator.
func requireNonEmpty(what string) func(string) error {
	return func(v string) error {
		if strings.TrimSpace(v) == "" {
			return fmt.Errorf("%s is required", what)
		}
		return nil
	}
}
```

Add at the top of the file (import `"os"` and alias to keep the deletion loop testable-by-eye):

```go
// osRemove is a seam for tests; production uses os.Remove.
var osRemove = os.Remove
```

Delete the `runModelFlow` stub from `run.go`.

- [ ] **Step 2: Build, vet, full tests**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: PASS

- [ ] **Step 3: Manual smoke test**

Run: `go build -o /tmp/gosaid ./cmd/gosaid && /tmp/gosaid setup model`
Expected: checklist with six catalog entries and sizes; checking `tiny` downloads ~78 MB with a progress line, then `Saved …` + restart offer. Re-running shows `tiny` pre-checked; unchecking asks about disk deletion.

- [ ] **Step 4: Commit**

```bash
git add internal/setup
git commit -m "feat: interactive local Whisper model manager (gosaid setup model)"
```

---

### Task 8: Provider manager flow

**Files:**
- Create: `internal/setup/flow_provider.go`
- Modify: `internal/setup/run.go` (delete the `runProviderFlow` stub)

**Interfaces:**
- Consumes: `ProviderPresets`, `EndpointSummary`, `AddOpenAIEndpoint`, `UpdateOpenAIEndpoint`, `DeleteEndpoint`, `HotkeysUsingEndpoint`, `ReassignEndpoint`, `OpenAIEndpointIDs`, `runModelFlow`, `EndpointIDInUse`.
- Produces: `func runProviderFlow(s *Session) error` (manager loop), `func runAddProvider(s *Session) error` (also called by the first-run chain in Task 10).

- [ ] **Step 1: Implement** `internal/setup/flow_provider.go`:

```go
package setup

import (
	"fmt"
	"strings"

	"charm.land/huh/v2"

	"github.com/dmtrkzntsv/gosaid/internal/config"
)

// Picker sentinels — \x00 cannot collide with endpoint ids or combos.
const (
	pickAdd  = "\x00add"
	pickBack = "\x00back"
)

// runProviderFlow is the provider manager: list endpoints, edit/delete one,
// or add a new provider. Loops until Back.
func runProviderFlow(s *Session) error {
	for {
		var opts []huh.Option[string]
		for _, d := range s.Cfg.Drivers {
			for _, e := range d.Endpoints {
				opts = append(opts, huh.NewOption(EndpointSummary(d.Driver, e), e.ID))
			}
		}
		opts = append(opts,
			huh.NewOption("+ Add new provider", pickAdd),
			huh.NewOption("← Back", pickBack),
		)
		var choice string
		if err := huh.NewForm(huh.NewGroup(
			huh.NewSelect[string]().Title("Providers").Options(opts...).Value(&choice),
		)).Run(); err != nil {
			return err
		}
		switch choice {
		case pickBack:
			return nil
		case pickAdd:
			if err := runAddProvider(s); err != nil {
				return err
			}
		default:
			if err := runProviderActions(s, choice); err != nil {
				return err
			}
		}
	}
}

// endpointDriver returns the driver type owning an endpoint id ("" if none).
func endpointDriver(cfg *config.Config, id string) string {
	for _, d := range cfg.Drivers {
		for _, e := range d.Endpoints {
			if e.ID == id {
				return d.Driver
			}
		}
	}
	return ""
}

// runProviderActions shows Edit / Delete / Back for one endpoint.
func runProviderActions(s *Session, id string) error {
	var action string
	if err := huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().Title(id).Options(
			huh.NewOption("Edit", "edit"),
			huh.NewOption("Delete", "delete"),
			huh.NewOption("← Back", "back"),
		).Value(&action),
	)).Run(); err != nil {
		return err
	}
	switch action {
	case "edit":
		if endpointDriver(s.Cfg, id) == config.DriverWhisperCPP {
			return runModelFlow(s)
		}
		return runEditCloudProvider(s, id)
	case "delete":
		return runDeleteProvider(s, id)
	}
	return nil
}

// runEditCloudProvider updates api_base/api_key of an openai_compatible
// endpoint.
func runEditCloudProvider(s *Session, id string) error {
	var apiBase, apiKey string
	for _, d := range s.Cfg.Drivers {
		for _, e := range d.Endpoints {
			if e.ID == id {
				apiBase, apiKey = e.Config.APIBase, e.Config.APIKey
			}
		}
	}
	if err := huh.NewForm(huh.NewGroup(
		huh.NewInput().Title("API base URL").
			Validate(requireNonEmpty("api base")).Value(&apiBase),
		huh.NewInput().Title("API key").Password(true).
			Validate(requireNonEmpty("api key")).Value(&apiKey),
	)).Run(); err != nil {
		return err
	}
	if err := UpdateOpenAIEndpoint(s.Cfg, id, apiBase, apiKey); err != nil {
		return err
	}
	s.Dirty = true
	return nil
}

// runDeleteProvider deletes an endpoint, first resolving hotkeys that
// reference it: reassign to another openai_compatible endpoint when one
// exists, otherwise require explicit confirmation.
func runDeleteProvider(s *Session, id string) error {
	refs := HotkeysUsingEndpoint(s.Cfg, id)
	if len(refs) > 0 {
		var others []string
		for _, other := range OpenAIEndpointIDs(s.Cfg) {
			if other != id {
				others = append(others, other)
			}
		}
		if len(others) > 0 {
			reassignTo := ""
			opts := []huh.Option[string]{huh.NewOption("Don't reassign (hotkeys will break)", "")}
			for _, o := range others {
				opts = append(opts, huh.NewOption("Reassign to "+o, o))
			}
			if err := huh.NewForm(huh.NewGroup(
				huh.NewSelect[string]().
					Title(fmt.Sprintf("Hotkeys using %q: %s", id, strings.Join(refs, ", "))).
					Description("Pick a replacement provider for them, or proceed without one.").
					Options(opts...).Value(&reassignTo),
			)).Run(); err != nil {
				return err
			}
			if reassignTo != "" {
				ReassignEndpoint(s.Cfg, id, reassignTo)
			}
		}
	}
	confirmed := false
	if err := huh.NewForm(huh.NewGroup(
		huh.NewConfirm().Title(fmt.Sprintf("Delete provider %q?", id)).
			Affirmative("Delete").Negative("Cancel").Value(&confirmed),
	)).Run(); err != nil {
		return err
	}
	if !confirmed {
		return nil
	}
	if err := DeleteEndpoint(s.Cfg, id); err != nil {
		return err
	}
	s.Dirty = true
	return nil
}

// runAddProvider is the preset-driven add flow. Also used by the first-run
// guided chain.
func runAddProvider(s *Session) error {
	presetKey := ""
	var presetOpts []huh.Option[string]
	for _, p := range ProviderPresets {
		presetOpts = append(presetOpts, huh.NewOption(p.Label, p.Key))
	}
	if err := huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().Title("Add a provider").Options(presetOpts...).Value(&presetKey),
	)).Run(); err != nil {
		return err
	}
	var preset ProviderPreset
	for _, p := range ProviderPresets {
		if p.Key == presetKey {
			preset = p
		}
	}
	if preset.Local {
		return runModelFlow(s)
	}

	id := preset.Key
	if preset.Custom {
		id = ""
	}
	apiBase := preset.APIBase
	var apiKey string
	fields := []huh.Field{
		huh.NewInput().Title("Endpoint id").
			Description("Short name used in model references (e.g. \"openai:whisper-1\").").
			Validate(func(v string) error {
				return validateEndpointID(s.Cfg, strings.TrimSpace(v))
			}).
			Value(&id),
	}
	if preset.Custom {
		fields = append(fields, huh.NewInput().Title("API base URL").
			Placeholder("https://api.example.com/v1").
			Validate(requireNonEmpty("api base")).Value(&apiBase))
	}
	fields = append(fields, huh.NewInput().Title("API key").Password(true).
		Validate(requireNonEmpty("api key")).Value(&apiKey))
	if err := huh.NewForm(huh.NewGroup(fields...)).Run(); err != nil {
		return err
	}
	if err := AddOpenAIEndpoint(s.Cfg, strings.TrimSpace(id), apiBase, apiKey); err != nil {
		return err
	}
	s.Dirty = true
	return nil
}
```

Delete the `runProviderFlow` stub from `run.go`.

- [ ] **Step 2: Build, vet, full tests**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: PASS

- [ ] **Step 3: Manual smoke test**

Run: `go build -o /tmp/gosaid ./cmd/gosaid && /tmp/gosaid setup provider`
Expected: list shows existing endpoints with summaries; Add → preset list with Local Whisper first; picking OpenAI pre-fills id `openai`, asks for a masked key; the manager loop returns after each action; Back → save step.

- [ ] **Step 4: Commit**

```bash
git add internal/setup
git commit -m "feat: interactive provider manager (gosaid setup provider)"
```

---

### Task 9: Hotkey manager flow

**Files:**
- Create: `internal/setup/flow_hotkey.go`
- Modify: `internal/setup/run.go` (delete the `runHotkeyFlow` stub)

**Interfaces:**
- Consumes: `HotkeySummary`, `SuggestedCombos`, `hotkey.Parse` (`internal/hotkey`), `TranscribeModelOptions`, `ChatModelOptions`, `OpenAIEndpointIDs`, `config.Languages`, `config.LanguageName`, recipes API from Task 4, `UpsertHotkey`, `DeleteHotkey`, `micOptions`.
- Produces: `func runHotkeyFlow(s *Session) error` (manager loop), `func runHotkeyWizard(s *Session, existing *HotkeyAnswers) error` (nil = add; non-nil = edit, combo fixed). Also used by the Task 10 first-run chain.

- [ ] **Step 1: Implement** `internal/setup/flow_hotkey.go`:

```go
package setup

import (
	"fmt"
	"sort"
	"strings"

	"charm.land/huh/v2"

	"github.com/dmtrkzntsv/gosaid/internal/config"
	"github.com/dmtrkzntsv/gosaid/internal/hotkey"
)

const pickTypeOwn = "\x00type-own"

// runHotkeyFlow is the hotkey manager: list bindings, edit/delete one, or
// run the add wizard. Loops until Back.
func runHotkeyFlow(s *Session) error {
	if len(s.Cfg.Drivers) == 0 {
		fmt.Println("No providers configured yet — add one first (gosaid setup provider).")
		return nil
	}
	for {
		combos := make([]string, 0, len(s.Cfg.Hotkeys))
		for combo := range s.Cfg.Hotkeys {
			combos = append(combos, combo)
		}
		sort.Strings(combos)
		var opts []huh.Option[string]
		for _, combo := range combos {
			opts = append(opts, huh.NewOption(HotkeySummary(combo, s.Cfg.Hotkeys[combo]), combo))
		}
		opts = append(opts,
			huh.NewOption("+ Add new hotkey", pickAdd),
			huh.NewOption("← Back", pickBack),
		)
		var choice string
		if err := huh.NewForm(huh.NewGroup(
			huh.NewSelect[string]().Title("Hotkeys").Options(opts...).Value(&choice),
		)).Run(); err != nil {
			return err
		}
		switch choice {
		case pickBack:
			return nil
		case pickAdd:
			if err := runHotkeyWizard(s, nil); err != nil {
				return err
			}
		default:
			if err := runHotkeyActions(s, choice); err != nil {
				return err
			}
		}
	}
}

// runHotkeyActions shows Edit / Delete / Back for one binding.
func runHotkeyActions(s *Session, combo string) error {
	var action string
	if err := huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().Title(HotkeySummary(combo, s.Cfg.Hotkeys[combo])).Options(
			huh.NewOption("Edit", "edit"),
			huh.NewOption("Delete", "delete"),
			huh.NewOption("← Back", "back"),
		).Value(&action),
	)).Run(); err != nil {
		return err
	}
	switch action {
	case "edit":
		a := AnswersFrom(combo, s.Cfg.Hotkeys[combo])
		return runHotkeyWizard(s, &a)
	case "delete":
		confirmed := false
		if err := huh.NewForm(huh.NewGroup(
			huh.NewConfirm().Title(fmt.Sprintf("Delete hotkey %q?", combo)).
				Affirmative("Delete").Negative("Cancel").Value(&confirmed),
		)).Run(); err != nil {
			return err
		}
		if confirmed {
			DeleteHotkey(s.Cfg, combo)
			s.Dirty = true
		}
	}
	return nil
}

// askCombo picks a key combo: curated list (minus already-bound combos) or
// free text validated by the hotkey parser.
func askCombo(s *Session) (string, error) {
	var opts []huh.Option[string]
	for _, c := range SuggestedCombos {
		if _, bound := s.Cfg.Hotkeys[c]; !bound {
			opts = append(opts, huh.NewOption(c, c))
		}
	}
	opts = append(opts, huh.NewOption("Type your own…", pickTypeOwn))
	var choice string
	if err := huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().Title("Key combo").Options(opts...).Value(&choice),
	)).Run(); err != nil {
		return "", err
	}
	if choice != pickTypeOwn {
		return choice, nil
	}
	var combo string
	err := huh.NewForm(huh.NewGroup(
		huh.NewInput().Title("Key combo").
			Placeholder("ctrl+alt+space").
			Validate(func(v string) error {
				v = strings.ToLower(strings.TrimSpace(v))
				if _, _, err := hotkey.Parse(v); err != nil {
					return err
				}
				if _, bound := s.Cfg.Hotkeys[v]; bound {
					return fmt.Errorf("%q is already bound", v)
				}
				return nil
			}).
			Value(&combo),
	)).Run()
	return strings.ToLower(strings.TrimSpace(combo)), err
}

// askModelRef resolves one stage's model: auto-pick a single option, select
// among several, or (for stages with no preset suggestion) pick an endpoint
// and type a model id.
func askModelRef(s *Session, title string, options []ModelOption) (string, error) {
	if len(options) == 1 {
		return options[0].Ref, nil
	}
	if len(options) > 1 {
		var choice string
		err := huh.NewForm(huh.NewGroup(
			huh.NewSelect[string]().Title(title).Options(func() []huh.Option[string] {
				var opts []huh.Option[string]
				for _, o := range options {
					opts = append(opts, huh.NewOption(o.Label, o.Ref))
				}
				return opts
			}()...).Value(&choice),
		)).Run()
		return choice, err
	}
	// No suggestions — endpoint + free-text model id.
	ids := OpenAIEndpointIDs(s.Cfg)
	if len(ids) == 0 {
		return "", fmt.Errorf("no endpoint can serve %q — add a provider first", title)
	}
	endpointID := ids[0]
	var fields []huh.Field
	if len(ids) > 1 {
		var opts []huh.Option[string]
		for _, id := range ids {
			opts = append(opts, huh.NewOption(id, id))
		}
		fields = append(fields, huh.NewSelect[string]().Title(title+" — endpoint").Options(opts...).Value(&endpointID))
	}
	var model string
	fields = append(fields, huh.NewInput().Title(title+" — model id").
		Validate(requireNonEmpty("model id")).Value(&model))
	if err := huh.NewForm(huh.NewGroup(fields...)).Run(); err != nil {
		return "", err
	}
	return endpointID + ":" + strings.TrimSpace(model), nil
}

// runHotkeyWizard runs the recipe-first add/edit wizard. existing == nil
// adds a new binding; otherwise edits (combo unchanged).
func runHotkeyWizard(s *Session, existing *HotkeyAnswers) error {
	var a HotkeyAnswers
	if existing != nil {
		a = *existing
	} else {
		a.Mode = string(config.ModeHold)
		combo, err := askCombo(s)
		if err != nil {
			return err
		}
		a.Combo = combo
	}

	chatAvailable := len(ChatModelOptions(s.Cfg)) > 0 || len(OpenAIEndpointIDs(s.Cfg)) > 0
	recipeOpts := []huh.Option[string]{
		huh.NewOption("Just transcribe", RecipeTranscribe),
	}
	desc := "What should this hotkey do with your speech?"
	if chatAvailable {
		recipeOpts = append(recipeOpts,
			huh.NewOption("Transcribe + clean up", RecipeCleanup),
			huh.NewOption("Translate to another language", RecipeTranslate),
			huh.NewOption("Compose (rewrite with instructions)", RecipeCompose),
		)
	} else {
		desc += " (Clean up, translate and compose need a cloud provider — add one first.)"
	}
	if err := huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().Title("Recipe").Description(desc).
			Options(recipeOpts...).Value(&a.Recipe),
	)).Run(); err != nil {
		return err
	}

	ref, err := askModelRef(s, "Transcription model", TranscribeModelOptions(s.Cfg))
	if err != nil {
		return err
	}
	a.TranscribeRef = ref

	if a.Recipe != RecipeTranscribe {
		ref, err := askModelRef(s, "Language model", ChatModelOptions(s.Cfg))
		if err != nil {
			return err
		}
		a.ChatRef = ref
	}
	if a.Recipe == RecipeTranslate {
		var langOpts []huh.Option[string]
		for _, code := range config.Languages() {
			langOpts = append(langOpts, huh.NewOption(
				fmt.Sprintf("%s (%s)", config.LanguageName(code), code), code))
		}
		if err := huh.NewForm(huh.NewGroup(
			huh.NewSelect[string]().Title("Translate to").Options(langOpts...).Value(&a.TargetLang),
		)).Run(); err != nil {
			return err
		}
	}
	if a.Recipe == RecipeCompose {
		if err := huh.NewForm(huh.NewGroup(
			huh.NewText().Title("Compose instructions").
				Description("e.g. \"Write in a formal, business-email register.\"").
				Value(&a.Instructions),
		)).Run(); err != nil {
			return err
		}
	}

	advanced := false
	if err := huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().Title("Mode").
			Description("Hold: record while pressed. Toggle: press to start, press to stop.").
			Options(
				huh.NewOption("Hold (push-to-talk)", string(config.ModeHold)),
				huh.NewOption("Toggle", string(config.ModeToggle)),
			).Value(&a.Mode),
		huh.NewConfirm().Title("Set a hotkey-specific microphone?").Value(&advanced),
	)).Run(); err != nil {
		return err
	}
	if advanced {
		opts, err := micOptions()
		if err != nil {
			return err
		}
		if err := huh.NewForm(huh.NewGroup(
			huh.NewSelect[string]().Title("Microphone for this hotkey").
				Description("Overrides the global default for this hotkey only.").
				Options(opts...).Value(&a.Microphone),
		)).Run(); err != nil {
			return err
		}
	}

	UpsertHotkey(s.Cfg, a.Combo, BuildHotkey(a))
	s.Dirty = true
	fmt.Printf("Hotkey saved: %s\n", HotkeySummary(a.Combo, s.Cfg.Hotkeys[a.Combo]))
	return nil
}
```

Delete the `runHotkeyFlow` stub from `run.go`.

- [ ] **Step 2: Build, vet, full tests**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: PASS

- [ ] **Step 3: Manual smoke test**

Run: `go build -o /tmp/gosaid ./cmd/gosaid && /tmp/gosaid setup hotkey`
Expected: list of existing hotkeys with summary lines; Add → combo picker (bound combos absent) → recipe → models auto-picked or selected → mode → saved summary printed; Edit pre-fills; Delete confirms.

- [ ] **Step 4: Commit**

```bash
git add internal/setup
git commit -m "feat: recipe-first hotkey wizard and manager (gosaid setup hotkey)"
```

---

### Task 10: Hub and first-run guided chain

**Files:**
- Create: `internal/setup/hub.go`
- Modify: `internal/setup/run.go` (delete the `runHub` stub; hub-mode validation failure re-enters the menu)

**Interfaces:**
- Consumes: all four flows, `FirstRun`, `ResetForFirstRun`, `runAddProvider`, `runHotkeyWizard`, `runMicFlow`.
- Produces: `func runHub(s *Session) error`.

- [ ] **Step 1: Implement** `internal/setup/hub.go`:

```go
package setup

import (
	"fmt"

	"charm.land/huh/v2"

	"github.com/dmtrkzntsv/gosaid/internal/config"
)

// runHub is the `gosaid setup` menu. On a first run (no usable provider) it
// chains the guided flow instead: provider → first hotkey → microphone.
func runHub(s *Session) error {
	if FirstRun(s.Cfg) {
		return runFirstRun(s)
	}
	for {
		var choice string
		if err := huh.NewForm(huh.NewGroup(
			huh.NewSelect[string]().Title("GoSaid setup").Options(
				huh.NewOption("Hotkeys", "hotkey"),
				huh.NewOption("Providers", "provider"),
				huh.NewOption("Local models", "model"),
				huh.NewOption("Default microphone", "mic"),
				huh.NewOption("Done", "done"),
			).Value(&choice),
		)).Run(); err != nil {
			return err
		}
		var err error
		switch choice {
		case "hotkey":
			err = runHotkeyFlow(s)
		case "provider":
			err = runProviderFlow(s)
		case "model":
			err = runModelFlow(s)
		case "mic":
			err = runMicFlow(s)
		case "done":
			return nil
		}
		if err != nil {
			return err
		}
	}
}

// runFirstRun is the guided chain for a fresh install. It rebuilds drivers
// and hotkeys from scratch (dropping the shipped placeholder config) and
// walks: provider → first hotkey → default microphone.
func runFirstRun(s *Session) error {
	fmt.Println("Welcome to GoSaid! Let's set up a provider, a hotkey, and your microphone.")
	ResetForFirstRun(s.Cfg)
	s.Cfg.Version = config.CurrentVersion
	if s.Cfg.ToggleMaxSeconds <= 0 {
		s.Cfg.ToggleMaxSeconds = config.DefaultToggleSeconds
	}
	if s.Cfg.InjectionMode == "" {
		s.Cfg.InjectionMode = config.InjectionModePaste
	}
	s.Dirty = true
	if err := runAddProvider(s); err != nil {
		return err
	}
	if err := runHotkeyWizard(s, nil); err != nil {
		return err
	}
	return runMicFlow(s)
}
```

Delete the `runHub` stub from `run.go`.

- [ ] **Step 2: Hub-mode validation failure re-enters the menu.** In `run.go`, wrap the hub call in `Run` so a failed save loops back (spec: "show the error and re-enter the menu rather than writing"). Replace the plain `flow(s)` + `finish(s)` sequence for the hub case with a helper used by all topics:

```go
	// In Run, after LoadSession:
	for {
		if err := flow(s); err != nil {
			if abort, ferr := confirmDiscardOnAbort(s, err); abort {
				if ferr != nil {
					fmt.Fprintf(os.Stderr, "error: %v\n", ferr)
					return 1
				}
				fmt.Println("Changes discarded.")
				return 0
			} else if !errors.Is(err, huh.ErrUserAborted) {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				return 1
			}
		}
		if err := finish(s); err != nil {
			fmt.Fprintf(os.Stderr, "config not saved: %v\n", err)
			if topic == "" {
				continue // hub: back to the menu to fix it
			}
			return 1
		}
		return 0
	}
```

- [ ] **Step 3: Build, vet, full tests**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: PASS

- [ ] **Step 4: Manual smoke tests**

Run: `go build -o /tmp/gosaid ./cmd/gosaid && /tmp/gosaid setup`
Expected (existing config): the five-entry menu; each manager returns to the menu; Done → save + restart offer.
Then: `mv "$HOME/Library/Application Support/gosaid/config.json"{,.bak} && /tmp/gosaid setup` → welcome message, guided provider → hotkey → mic chain, save. Restore: `mv "$HOME/Library/Application Support/gosaid/config.json"{.bak,}`.

- [ ] **Step 5: Commit**

```bash
git add internal/setup
git commit -m "feat: gosaid setup hub menu and first-run guided chain"
```

---

### Task 11: Documentation

**Files:**
- Modify: `README.md`

**Interfaces:** none (docs only).

- [ ] **Step 1: Update README.** Find every reference with `grep -n "gosaid config\|mic list\|gosaid mic" README.md` and update:

1. In **Installation → macOS & Linux**, replace `gosaid config                  # paste your API key, save` with `gosaid setup                   # guided setup: provider, hotkey, microphone`.
2. In **Installation → Windows** step 3, replace `Run \`gosaid config\`, then \`gosaid\`.` with `Run \`gosaid setup\`, then \`gosaid\`.`
3. In the **Configuration** section, add before the JSON discussion:

```markdown
The easiest way to configure GoSaid is the interactive setup:

```
gosaid setup             # menu: hotkeys, providers, local models, microphone
gosaid setup hotkey      # jump straight to one topic (also: provider, model, mic)
```

On a fresh install `gosaid setup` walks you through everything: pick a
provider, create your first hotkey, choose a microphone. Prefer raw JSON?
`gosaid config` still opens the file in `$EDITOR`.
```

4. Remove any `gosaid mic list` references (replace with `gosaid setup mic`).

- [ ] **Step 2: Verify docs consistency**

Run: `grep -rn "gosaid mic" README.md internal/ docs/ --include='*.go' --include='*.md' | grep -v superpowers`
Expected: no output (all references updated; specs/plans excluded).

- [ ] **Step 3: Full final check**

Run: `go test ./... && go vet ./... && gofmt -l .`
Expected: tests pass, vet clean, no unformatted files.

- [ ] **Step 4: Commit**

```bash
git add README.md
git commit -m "docs: document gosaid setup in README"
```

---

## Plan Self-Review (completed)

- **Spec coverage:** CLI surface (Task 5, 10), huh toolkit (5), architecture split (3–4 pure / 5–10 wiring), hotkey manager + recipe wizard (9), provider manager + presets with Local Whisper first (8), local model manager with catalog/custom/deletion (7), microphone flow + global field + daemon resolution (1, 5), save semantics + discard confirm (5, 10), restart offer (6), first-run chain (10), `gosaid mic` removal + docs (5, 11), non-TTY guard (5), endpoint-delete reassignment (8).
- **Type consistency:** flow signatures are all `func(*Session) error`; model refs always `endpoint:model` strings; `ModelOption{Label, Ref}`, `HotkeyAnswers` fields match between `recipe.go` and `flow_hotkey.go`; `models` package names match between Tasks 2 and 7.
- **Known judgment calls encoded:** first-run resets the placeholder example config (REPLACE_ME detection); editing a hotkey keeps its combo (delete+add to rebind); custom local models can be added and removed but not re-added from the checklist (only via the custom prompt); catalog downloads reuse existing files on disk.
