# Local-Only Setup Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the interactive `gosaid setup` (hub, provider manager, presets) with one linear local-only wizard, per `docs/superpowers/specs/2026-07-19-local-only-setup-design.md`.

**Architecture:** `gosaid setup` becomes a single eight-step wizard (mic → transcription model → shortcut → mode → enhance? → translate? → compose? → chat model) that installs local models from a curated catalog and writes one hotkey. Cloud setup moves entirely to config.json. The provider manager, preset table, hub menu, and topic subcommands are deleted; the catalog gains local chat (GGUF) models; the local endpoint ids become `speech`/`text`.

**Tech Stack:** Go 1.25, `charm.land/huh/v2` (forms), `internal/models` (download/register, already driver-aware after the llama.cpp merge). Module `github.com/dmtrkzntsv/gosaid`.

## Global Constraints

- Local endpoint ids: transcription = `speech` (whisper_cpp), chat = `text` (llama_cpp). These are the defaults `gosaid model download` uses too.
- Transcription catalog: `small` (`ggml-small.bin`, ~488 MB), `turbo` (`ggml-large-v3-turbo-q5_0.bin`, ~550 MB), from `ggerganov/whisper.cpp`.
- Chat catalog: `qwen3.5-0.8b` (`ggml-org/Qwen3.5-0.8B-GGUF` / `Qwen3.5-0.8B-Q4_0.gguf`, ~563 MB), `gemma-4-e2b` (`ggml-org/gemma-4-E2B-it-GGUF` / `gemma-4-E2B-it-Q4_0.gguf`, ~2.8 GB).
- The registered model name is always the catalog's explicit `Name` (lowercase, friendly), never `DeriveName(File)`.
- Entry branch: no config → fresh wizard; config exists → "Start from scratch?" (yes = reset config to empty incl. cloud endpoints, keep model files on disk; no = "Which hotkey?" list + "Add new", editing the chosen hotkey pre-filled).
- One chat model backs every enabled stage. Per-stage models, toggle beyond the one question, per-hotkey mic, cloud providers → config.json only.
- Wizard only adds/edits; it never re-downloads a model whose file is already on disk.
- Nothing writes config.json until `finish`; `Save` validates first.
- Non-TTY stdin → "requires an interactive terminal", exit 1. Any `setup <topic>` arg → usage, exit 2.
- Flow wiring (huh forms) is untested by design; pure logic gets table-driven tests.
- Verify with `go test ./...`, `go vet ./...`, `gofmt -l internal/ cmd/`. First test run is slow (CGO). The `-lobjc` duplicate-library linker warning is pre-existing and expected.
- Commit after each task with the message given.

---

### Task 1: Rename local endpoint ids to speech/text

**Files:**
- Modify: `internal/models/models.go` (constants + a doc comment)
- Modify: `internal/cli/model.go:24` (flag help text)
- Test: `internal/models/models_test.go` (existing `TestDownloadDefaults` updates)

**Interfaces:**
- Consumes: nothing new.
- Produces: `models.DefaultWhisperEndpoint == "speech"`, `models.DefaultLlamaEndpoint == "text"`. Later tasks register under these.

- [ ] **Step 1: Update the failing test.** In `internal/models/models_test.go`, `TestDownloadDefaults` currently asserts `DefaultWhisperEndpoint`/`DefaultLlamaEndpoint` equal `"local"`/`"local-llm"` indirectly. Make the expectation explicit:

```go
func TestDownloadDefaults(t *testing.T) {
	if DefaultWhisperEndpoint != "speech" {
		t.Errorf("DefaultWhisperEndpoint = %q, want speech", DefaultWhisperEndpoint)
	}
	if DefaultLlamaEndpoint != "text" {
		t.Errorf("DefaultLlamaEndpoint = %q, want text", DefaultLlamaEndpoint)
	}
	if d, e := DownloadDefaults("ggml-base.bin"); d != config.DriverWhisperCPP || e != "speech" {
		t.Fatalf("bin defaults = %q/%q", d, e)
	}
	if d, e := DownloadDefaults("gemma-3-4b-it-Q4_K_M.gguf"); d != config.DriverLlamaCPP || e != "text" {
		t.Fatalf("gguf defaults = %q/%q", d, e)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/models/ -run TestDownloadDefaults -v`
Expected: FAIL — endpoints are still `local`/`local-llm`.

- [ ] **Step 3: Rename the constants.** In `internal/models/models.go`:

```go
// Default endpoint ids per driver, used when the caller doesn't name one.
const (
	DefaultWhisperEndpoint = "speech"
	DefaultLlamaEndpoint   = "text"
)
```

- [ ] **Step 4: Update the CLI help text.** In `internal/cli/model.go:24`:

```go
	endpoint := fs.String("endpoint", "", "endpoint id to register under (default: speech for whisper models, text for .gguf chat models)")
```

- [ ] **Step 5: Fix the other model_test.go references.** In `internal/models/models_test.go`, `TestDownloadGGUFRegistersUnderLlamaCPP` uses `opts.EndpointID = DefaultLlamaEndpoint` and checks `e.ID == DefaultLlamaEndpoint` — these follow the constant automatically, no change needed. `TestModelDownloadHappyPath` and `TestUnregisterPrunesEmptyEndpointAndDriver` hardcode the string `"local"` for whisper — leave them: they pass an explicit `"local"` endpoint id and never rely on the default, so they still exercise a valid whisper endpoint id.

- [ ] **Step 6: Run tests + commit**

Run: `go test ./internal/models/ ./internal/cli/ && go vet ./...`
Expected: PASS

```bash
git add internal/models/models.go internal/models/models_test.go internal/cli/model.go
git commit -m "refactor: rename local endpoint ids to speech and text"
```

---

### Task 2: Catalog — add Repo field and chat models

**Files:**
- Modify: `internal/models/catalog.go`
- Modify: `internal/models/models_test.go` (`TestCatalog`)

**Interfaces:**
- Consumes: nothing new.
- Produces:
  - `CatalogEntry` gains `Repo string // "" falls back to CatalogRepo` and `Kind string` (`"whisper"` or `"chat"`).
  - `models.Catalog` holds the two whisper + two chat entries.
  - `func CatalogRepoFor(e CatalogEntry) string` — returns `e.Repo` if set, else `CatalogRepo`.
  - Chat entries: whisper names unchanged; chat names `qwen3.5-0.8b`, `gemma-4-e2b`.

- [ ] **Step 1: Write the failing test.** Replace `TestCatalog` in `internal/models/models_test.go`:

```go
func TestCatalog(t *testing.T) {
	if len(Catalog) == 0 {
		t.Fatal("catalog must offer at least one model")
	}
	var whisper, chat int
	seenNames := map[string]bool{}
	seenFiles := map[string]bool{}
	for _, e := range Catalog {
		if e.Name == "" || e.File == "" || e.Size == "" || e.Note == "" || e.Kind == "" {
			t.Errorf("incomplete entry: %+v", e)
		}
		if seenNames[e.Name] {
			t.Errorf("duplicate model name %q — refs would collide", e.Name)
		}
		seenNames[e.Name] = true
		if strings.Contains(e.Name, ":") {
			t.Errorf("model name %q must not contain ':'", e.Name)
		}
		if seenFiles[e.File] {
			t.Errorf("duplicate model file %q", e.File)
		}
		seenFiles[e.File] = true
		switch e.Kind {
		case "whisper":
			whisper++
			if !strings.HasSuffix(e.File, ".bin") {
				t.Errorf("whisper entry %q: file %q should be a GGML .bin", e.Name, e.File)
			}
			if e.Repo != "" {
				t.Errorf("whisper entry %q should use the shared CatalogRepo, got Repo %q", e.Name, e.Repo)
			}
		case "chat":
			chat++
			if !strings.HasSuffix(e.File, ".gguf") {
				t.Errorf("chat entry %q: file %q should be a .gguf", e.Name, e.File)
			}
			if e.Repo == "" {
				t.Errorf("chat entry %q must carry its own Repo", e.Name)
			}
		default:
			t.Errorf("entry %q: unknown Kind %q", e.Name, e.Kind)
		}
		// CatalogRepoFor falls back to the shared repo for whisper.
		if got := CatalogRepoFor(e); got == "" {
			t.Errorf("entry %q: CatalogRepoFor returned empty", e.Name)
		}
	}
	if whisper == 0 || chat == 0 {
		t.Fatalf("want both whisper and chat entries, got %d/%d", whisper, chat)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/models/ -run TestCatalog -v`
Expected: FAIL — `Kind`, `Repo`, `CatalogRepoFor` undefined.

- [ ] **Step 3: Rewrite the catalog.** Replace the type and var in `internal/models/catalog.go`:

```go
// CatalogEntry is one curated local model. Size and Note are human labels
// shown in the picker. Repo is the Hugging Face repo the file lives in; an
// empty Repo falls back to CatalogRepo (the shared whisper.cpp repo). Kind is
// "whisper" (transcription, .bin) or "chat" (llama_cpp, .gguf).
type CatalogEntry struct {
	Name string // registered model name, referenced as "endpoint:name"
	Repo string // HF repo; "" → CatalogRepo
	File string // file inside the repo
	Size string // approximate download size ("~148 MB")
	Note string // one-phrase guidance ("fast on plain CPU")
	Kind string // "whisper" or "chat"
}

// CatalogRepoFor returns the Hugging Face repo an entry downloads from,
// defaulting to the shared whisper.cpp repo.
func CatalogRepoFor(e CatalogEntry) string {
	if e.Repo != "" {
		return e.Repo
	}
	return CatalogRepo
}

// Catalog is the curated set offered by `gosaid setup`: two Whisper
// transcription models and two llama.cpp chat models, all from ggml-org (or
// the shared whisper.cpp repo), so quantizations track llama.cpp's format.
var Catalog = []CatalogEntry{
	{Name: "small", File: "ggml-small.bin", Size: "~488 MB", Note: "fast on plain CPU", Kind: "whisper"},
	{Name: "turbo", File: "ggml-large-v3-turbo-q5_0.bin", Size: "~550 MB", Note: "recommended — best accuracy/latency balance", Kind: "whisper"},
	{Name: "qwen3.5-0.8b", Repo: "ggml-org/Qwen3.5-0.8B-GGUF", File: "Qwen3.5-0.8B-Q4_0.gguf", Size: "~563 MB", Note: "fast, light on RAM", Kind: "chat"},
	{Name: "gemma-4-e2b", Repo: "ggml-org/gemma-4-E2B-it-GGUF", File: "gemma-4-E2B-it-Q4_0.gguf", Size: "~2.8 GB", Note: "better quality", Kind: "chat"},
}
```

Keep the existing `CatalogRepo`/`HuggingFaceBase` const block above this.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/models/ -run TestCatalog -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/models/catalog.go internal/models/models_test.go
git commit -m "feat: catalog carries per-entry repo and kind, adds local chat models"
```

---

### Task 3: models — driver-aware RegisteredModelsFor

**Files:**
- Modify: `internal/models/models.go`
- Test: `internal/models/models_test.go` (append)

**Interfaces:**
- Consumes: nothing new.
- Produces:
  - `func RegisteredModelsFor(cfg *config.Config, driver, endpointID string) map[string]string`
  - `RegisteredModels` becomes a thin wrapper: `RegisteredModelsFor(cfg, config.DriverWhisperCPP, endpointID)`.

- [ ] **Step 1: Write the failing test** — append to `internal/models/models_test.go`:

```go
func TestRegisteredModelsFor(t *testing.T) {
	cfg := config.Default()
	RegisterFor(cfg, config.DriverLlamaCPP, "text", "gemma", "/m/gemma.gguf")
	Register(cfg, "speech", "turbo", "/m/turbo.bin") // whisper wrapper

	if got := RegisteredModelsFor(cfg, config.DriverLlamaCPP, "text")["gemma"]; got != "/m/gemma.gguf" {
		t.Errorf("chat lookup = %q", got)
	}
	if got := RegisteredModelsFor(cfg, config.DriverWhisperCPP, "speech")["turbo"]; got != "/m/turbo.bin" {
		t.Errorf("whisper lookup = %q", got)
	}
	// Wrong driver for the endpoint returns nil.
	if got := RegisteredModelsFor(cfg, config.DriverWhisperCPP, "text"); got != nil {
		t.Errorf("cross-driver lookup should be nil, got %v", got)
	}
	// The whisper-only wrapper still works.
	if got := RegisteredModels(cfg, "speech")["turbo"]; got != "/m/turbo.bin" {
		t.Errorf("RegisteredModels wrapper = %q", got)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/models/ -run TestRegisteredModelsFor -v`
Expected: FAIL — `RegisteredModelsFor` undefined.

- [ ] **Step 3: Implement.** In `internal/models/models.go`, replace `RegisteredModels`:

```go
// RegisteredModels returns the models map for a whisper_cpp endpoint, or nil.
func RegisteredModels(cfg *config.Config, endpointID string) map[string]string {
	return RegisteredModelsFor(cfg, config.DriverWhisperCPP, endpointID)
}

// RegisteredModelsFor returns the models map for an endpoint under the given
// driver, or nil when no such endpoint exists.
func RegisteredModelsFor(cfg *config.Config, driver, endpointID string) map[string]string {
	for _, d := range cfg.Drivers {
		if d.Driver != driver {
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
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/models/ -run TestRegisteredModelsFor -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/models/models.go internal/models/models_test.go
git commit -m "feat: RegisteredModelsFor for driver-aware installed-model lookup"
```

---

### Task 4: Rewrite the pure hotkey layer for independent stages

Replace the recipe model (one of four pipelines) with independent stage flags. This is the pure `HotkeyAnswers`/`BuildHotkey`/`AnswersFrom` logic; the huh wizard comes in Task 7.

**Files:**
- Rewrite: `internal/setup/recipe.go` → rename responsibilities; keep the filename.
- Rewrite tests: `internal/setup/recipe_test.go`
- Delete: the recipe-menu constants and `RecipeOf`.

**Interfaces:**
- Consumes: `config` types.
- Produces (package `setup`):
  - `type HotkeyAnswers struct { Combo, Mode, TranscribeRef, ChatRef, TargetLang, Instructions string; Enhance, Translate, Compose bool }`
  - `func BuildHotkey(a HotkeyAnswers) config.Hotkey`
  - `func AnswersFrom(combo string, hk config.Hotkey) HotkeyAnswers`
  - `func (a HotkeyAnswers) NeedsChatModel() bool` — true when any of Enhance/Translate/Compose is set.

- [ ] **Step 1: Write the failing tests.** Replace `internal/setup/recipe_test.go`:

```go
package setup

import (
	"testing"

	"github.com/dmtrkzntsv/gosaid/internal/config"
)

func TestNeedsChatModel(t *testing.T) {
	if (HotkeyAnswers{}).NeedsChatModel() {
		t.Error("transcribe-only needs no chat model")
	}
	if !(HotkeyAnswers{Enhance: true}).NeedsChatModel() {
		t.Error("enhance needs a chat model")
	}
	if !(HotkeyAnswers{Translate: true}).NeedsChatModel() {
		t.Error("translate needs a chat model")
	}
	if !(HotkeyAnswers{Compose: true}).NeedsChatModel() {
		t.Error("compose needs a chat model")
	}
}

func TestBuildHotkeyStages(t *testing.T) {
	a := HotkeyAnswers{
		Combo: "option+space", Mode: "hold",
		TranscribeRef: "speech:turbo", ChatRef: "text:gemma-4-e2b",
		Enhance: true, Translate: true, TargetLang: "en",
	}
	hk := BuildHotkey(a)
	if hk.Transcribe.Model != "speech:turbo" {
		t.Errorf("transcribe = %q", hk.Transcribe.Model)
	}
	if !hk.Enhance.IsEnabled() || hk.Enhance.Model != "text:gemma-4-e2b" {
		t.Errorf("enhance = %+v", hk.Enhance)
	}
	if !hk.Translate.IsEnabled() || hk.Translate.OutputLanguage != "en" || hk.Translate.Model != "text:gemma-4-e2b" {
		t.Errorf("translate = %+v", hk.Translate)
	}
	if hk.Compose != nil {
		t.Error("compose must be absent when not enabled")
	}
}

func TestBuildHotkeyIndependentStages(t *testing.T) {
	// Compose without enhance — proves stages are independent, not a recipe
	// ladder (the old model forced enhance whenever translate was on).
	hk := BuildHotkey(HotkeyAnswers{
		Combo: "option+up", Mode: "toggle",
		TranscribeRef: "speech:small", ChatRef: "text:qwen3.5-0.8b",
		Compose: true, Instructions: "Formal register.",
	})
	if hk.Enhance != nil || hk.Translate != nil {
		t.Error("only compose should be set")
	}
	if !hk.Compose.IsEnabled() || hk.Compose.Instructions != "Formal register." {
		t.Errorf("compose = %+v", hk.Compose)
	}
	if hk.Mode != config.ModeToggle {
		t.Errorf("mode = %q", hk.Mode)
	}
}

func TestAnswersFromRoundTrip(t *testing.T) {
	orig := config.Hotkey{
		Mode:       config.ModeHold,
		Transcribe: config.TranscribeStage{Model: "speech:turbo"},
		Enhance:    &config.EnhanceStage{Model: "text:gemma-4-e2b"},
		Compose:    &config.ComposeStage{Model: "text:gemma-4-e2b", Instructions: "x"},
	}
	a := AnswersFrom("option+right", orig)
	if a.Combo != "option+right" || a.Mode != "hold" {
		t.Errorf("combo/mode = %q/%q", a.Combo, a.Mode)
	}
	if !a.Enhance || a.Translate || !a.Compose {
		t.Errorf("stage flags = %v/%v/%v", a.Enhance, a.Translate, a.Compose)
	}
	if a.ChatRef != "text:gemma-4-e2b" || a.Instructions != "x" {
		t.Errorf("chat/instructions = %q/%q", a.ChatRef, a.Instructions)
	}
	if a.TranscribeRef != "speech:turbo" {
		t.Errorf("transcribe = %q", a.TranscribeRef)
	}
	// An empty-mode hotkey round-trips as hold.
	if got := AnswersFrom("x", config.Hotkey{}).Mode; got != "hold" {
		t.Errorf("default mode = %q, want hold", got)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/setup/ -run 'NeedsChatModel|BuildHotkey|AnswersFrom' 2>&1 | head`
Expected: FAIL — new field/method names undefined (or the package won't compile if the old symbols still exist; that's fine, fix in Step 3).

- [ ] **Step 3: Rewrite `internal/setup/recipe.go`:**

```go
package setup

import (
	"github.com/dmtrkzntsv/gosaid/internal/config"
)

// HotkeyAnswers is everything the wizard collects for one hotkey. The three
// stage flags are independent — unlike the old recipe model, enabling
// translate does not force enhance. One ChatRef backs every enabled stage.
type HotkeyAnswers struct {
	Combo         string
	Mode          string // "hold" or "toggle"
	TranscribeRef string // "speech:model"
	ChatRef       string // "text:model"; empty when no stage is enabled
	Enhance       bool
	Translate     bool
	Compose       bool
	TargetLang    string // Translate only
	Instructions  string // Compose only
}

// NeedsChatModel reports whether any stage that runs a chat model is enabled,
// so the wizard knows whether to ask for one.
func (a HotkeyAnswers) NeedsChatModel() bool {
	return a.Enhance || a.Translate || a.Compose
}

// BuildHotkey materializes wizard answers into a hotkey. Each enabled stage is
// set independently and shares the single ChatRef.
func BuildHotkey(a HotkeyAnswers) config.Hotkey {
	hk := config.Hotkey{
		Mode:       config.HotkeyMode(a.Mode),
		Transcribe: config.TranscribeStage{Model: a.TranscribeRef},
	}
	if a.Enhance {
		hk.Enhance = &config.EnhanceStage{Model: a.ChatRef}
	}
	if a.Translate {
		hk.Translate = &config.TranslateStage{OutputLanguage: a.TargetLang, Model: a.ChatRef}
	}
	if a.Compose {
		hk.Compose = &config.ComposeStage{Model: a.ChatRef, Instructions: a.Instructions}
	}
	return hk
}

// AnswersFrom pre-fills the wizard from an existing hotkey (the edit path).
// The chat ref is taken from whichever enabled stage has one — they share it.
func AnswersFrom(combo string, hk config.Hotkey) HotkeyAnswers {
	mode := hk.Mode
	if mode == "" {
		mode = config.ModeHold
	}
	a := HotkeyAnswers{
		Combo:         combo,
		Mode:          string(mode),
		TranscribeRef: hk.Transcribe.Model,
		Enhance:       hk.Enhance.IsEnabled(),
		Translate:     hk.Translate.IsEnabled(),
		Compose:       hk.Compose.IsEnabled(),
	}
	switch {
	case a.Enhance:
		a.ChatRef = hk.Enhance.Model
	case a.Translate:
		a.ChatRef = hk.Translate.Model
	case a.Compose:
		a.ChatRef = hk.Compose.Model
	}
	if a.Translate {
		a.TargetLang = hk.Translate.OutputLanguage
	}
	if a.Compose {
		a.Instructions = hk.Compose.Instructions
	}
	return a
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/setup/ -run 'NeedsChatModel|BuildHotkey|AnswersFrom' -v 2>&1 | tail -15`
Expected: PASS. (Other setup files won't compile yet — they reference deleted symbols. That's expected; Tasks 5–8 remove those. If the package fails to build, run just this file's logic mentally — the four tests above are what matters. To keep the suite green between tasks, this task and Tasks 5–8 land as one reviewable unit if needed; but each still commits.)

Note to implementer: because deleting the recipe API breaks `flow_hotkey.go`/`options.go` which still reference it, the package will not build until Task 8. Run the targeted `go build ./internal/models/` and `go vet ./internal/models/` for model-package sanity, and rely on Task 8's full build. Commit this task's files regardless — the plan's later tasks restore a buildable tree.

- [ ] **Step 5: Commit**

```bash
git add internal/setup/recipe.go internal/setup/recipe_test.go
git commit -m "feat: independent hotkey stage flags replace the recipe model"
```

---

### Task 5: Delete the provider manager, presets, and hub

Remove the cloud-facing surface. After this task the package still won't build (flows reference deleted code); Task 8 makes it whole. Land Tasks 5–8 in sequence.

**Files:**
- Delete: `internal/setup/flow_provider.go`
- Delete: `internal/setup/presets.go`, `internal/setup/presets_test.go`
- Delete: `internal/setup/hub.go`
- Delete: `internal/setup/options.go`, `internal/setup/options_test.go` (the preset-derived model-option builders; the wizard uses the catalog instead)

**Interfaces:** removes `runProviderFlow`, `runAddProvider`, `runProviderActions`, `runEditCloudProvider`, `runDeleteProvider`, `endpointDriver`, `localModelFiles`, `ProviderPresets`, `ProviderPreset`, `PresetForAPIBase`, `runHub`, `runFirstRun`, `TranscribeModelOptions`, `ChatModelOptions`, `OpenAIEndpointIDs`, `ModelOption`, `SuggestedCombos` (moves — see below).

- [ ] **Step 1: Preserve `SuggestedCombos`.** It lives in `options.go` but the hotkey wizard still needs it. Before deleting `options.go`, move the `SuggestedCombos` var into `internal/setup/flow_hotkey.go` (Task 7 rewrites that file; for now, add it to a new small file to keep it): create `internal/setup/combos.go`:

```go
package setup

// SuggestedCombos is the curated key-combo list for the hotkey wizard. All
// entries parse with the hotkey package on every platform ("option" and
// "alt" are aliases everywhere).
var SuggestedCombos = []string{
	"option+left", "option+right", "option+up", "option+down",
	"ctrl+alt+space", "ctrl+alt+enter",
	"option+f8", "option+f9", "option+f10",
}
```

- [ ] **Step 2: Delete the files.**

```bash
git rm internal/setup/flow_provider.go internal/setup/presets.go internal/setup/presets_test.go internal/setup/hub.go internal/setup/options.go internal/setup/options_test.go
```

- [ ] **Step 3: Commit** (build is intentionally broken until Task 8)

```bash
git add internal/setup/combos.go
git commit -m "refactor: remove provider manager, presets, and hub from setup"
```

---

### Task 6: Trim apply.go to what the local wizard needs

**Files:**
- Modify: `internal/setup/apply.go` (delete cloud/provider mutators)
- Modify: `internal/setup/apply_test.go` (delete their tests)

**Interfaces:**
- Keeps: `UpsertHotkey(cfg, combo, hk)`.
- Adds: `func ResetConfig(cfg *config.Config)` — clears `Drivers` and `Hotkeys` for "start from scratch", keeping global settings.
- Removes: `DeleteHotkey`, `EndpointIDInUse`, `validateEndpointID`, `AddOpenAIEndpoint`, `UpdateOpenAIEndpoint`, `DeleteEndpoint`, `stageRefs`, `HotkeysUsingEndpoint`, `ReassignEndpoint`, `SetDefaultMicrophone`, `FirstRun`, `ResetForFirstRun`, `DeleteHotkeyBlocked`, `DeleteEndpointBlocked` (and their tests). Note `SetDefaultMicrophone` is inlined into the mic flow in Task 7.

- [ ] **Step 1: Write the failing test.** Replace `internal/setup/apply_test.go` entirely with just what survives plus the new reset test:

```go
package setup

import (
	"testing"

	"github.com/dmtrkzntsv/gosaid/internal/config"
)

func TestUpsertHotkey(t *testing.T) {
	cfg := &config.Config{}
	UpsertHotkey(cfg, "option+space", config.Hotkey{Mode: config.ModeHold})
	if _, ok := cfg.Hotkeys["option+space"]; !ok {
		t.Fatal("hotkey not added")
	}
	// Upsert replaces.
	UpsertHotkey(cfg, "option+space", config.Hotkey{Mode: config.ModeToggle})
	if cfg.Hotkeys["option+space"].Mode != config.ModeToggle {
		t.Error("upsert did not replace")
	}
}

func TestResetConfig(t *testing.T) {
	cfg := config.Default()
	cfg.SoundFeedback = true
	cfg.Microphone = "MacBook"
	ResetConfig(cfg)
	if len(cfg.Drivers) != 0 || len(cfg.Hotkeys) != 0 {
		t.Fatalf("drivers/hotkeys must be cleared: %+v", cfg)
	}
	// Global settings survive — reset is "start the pipeline over", not
	// "forget the user's preferences".
	if !cfg.SoundFeedback || cfg.Microphone != "MacBook" {
		t.Error("global settings must survive reset")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/setup/ -run 'TestResetConfig' 2>&1 | head`
Expected: FAIL — `ResetConfig` undefined.

- [ ] **Step 3: Rewrite `internal/setup/apply.go`:**

```go
package setup

import (
	"github.com/dmtrkzntsv/gosaid/internal/config"
)

// UpsertHotkey adds or replaces a hotkey binding.
func UpsertHotkey(cfg *config.Config, combo string, hk config.Hotkey) {
	if cfg.Hotkeys == nil {
		cfg.Hotkeys = map[string]config.Hotkey{}
	}
	cfg.Hotkeys[combo] = hk
}

// ResetConfig clears drivers and hotkeys for a "start from scratch" run,
// keeping global settings (microphone, toggle window, sounds, injection mode,
// log level, user context). Downloaded model files on disk are untouched, so
// re-installing a model reuses its file instead of downloading again.
func ResetConfig(cfg *config.Config) {
	cfg.Drivers = nil
	cfg.Hotkeys = map[string]config.Hotkey{}
}
```

- [ ] **Step 4: Run tests to verify they pass** (targeted; full package still broken until Task 8)

Run: `go test ./internal/setup/ -run 'TestUpsertHotkey|TestResetConfig' 2>&1 | tail`
Expected: PASS (once Task 8 lands and the package builds; if the package doesn't yet build, this is deferred to Task 8's full run — commit anyway).

- [ ] **Step 5: Commit**

```bash
git add internal/setup/apply.go internal/setup/apply_test.go
git commit -m "refactor: trim apply.go to UpsertHotkey and ResetConfig"
```

---

### Task 7: Model install flows (whisper + chat)

**Files:**
- Rewrite: `internal/setup/flow_model.go`

**Interfaces:**
- Consumes: `models.Catalog`, `models.CatalogEntry`, `models.CatalogRepoFor`, `models.HuggingFaceBase`, `models.FetchModelFile`, `models.RegisterFor`, `models.RegisteredModelsFor`, `models.DefaultWhisperEndpoint`, `models.DefaultLlamaEndpoint`, `platform.ModelsDir`, `config.DriverWhisperCPP`, `config.DriverLlamaCPP`, `listHeight`, `cancelable`, `errCancelStep`, `pickBack`.
- Produces:
  - `const pickBack = "\x00back"` (moves here from the deleted flow_provider.go — it's the only remaining user).
  - `func installWhisperModel(s *Session) (ref string, err error)` — returns `speech:<name>`.
  - `func installChatModel(s *Session) (ref string, err error)` — returns `text:<name>`.
  - Each: if a model of that driver is already registered on its endpoint, reuse it (return its ref without prompting); otherwise show the catalog picker for that kind, `FetchModelFile` (which reuses an on-disk file), `RegisterFor` on `s.Cfg`, return the ref.

**Why `FetchModelFile` + `RegisterFor`, not `models.Download`:** `Download` loads and saves config from disk itself and *errors when the model file already exists* (`file already exists: … --force`). That breaks reuse and the "scratch keeps files on disk" path. `FetchModelFile` instead returns an existing file's path with no download, and `RegisterFor` mutates the in-memory `s.Cfg` — so no disk round-trip, no reload, and re-installing an already-downloaded model just re-registers it.

- [ ] **Step 1: Rewrite `internal/setup/flow_model.go`:**

```go
package setup

import (
	"fmt"

	"charm.land/huh/v2"

	"github.com/dmtrkzntsv/gosaid/internal/config"
	"github.com/dmtrkzntsv/gosaid/internal/models"
	"github.com/dmtrkzntsv/gosaid/internal/platform"
)

// pickBack is the sentinel value for a "← Back" option in a select. \x00
// cannot collide with a real model name or combo.
const pickBack = "\x00back"

// installWhisperModel ensures a local transcription model exists and returns
// its "speech:<name>" ref. If one is already installed it is reused without
// prompting; otherwise the user picks from the whisper catalog and it downloads.
func installWhisperModel(s *Session) (string, error) {
	return installModel(s, "whisper", config.DriverWhisperCPP, models.DefaultWhisperEndpoint,
		"Transcription model", "Runs on-device — no API key, no network once downloaded.")
}

// installChatModel ensures a local chat model exists and returns its
// "text:<name>" ref. Reused if already installed, else picked and downloaded.
func installChatModel(s *Session) (string, error) {
	return installModel(s, "chat", config.DriverLlamaCPP, models.DefaultLlamaEndpoint,
		"Chat model", "Backs enhance, translate and compose. Runs on-device.")
}

// installModel is the shared install path for one model kind.
func installModel(s *Session, kind, driver, endpointID, title, desc string) (string, error) {
	// Reuse an already-installed model of this kind — the wizard installs one
	// per endpoint, so the first registered name is the one to use.
	for name := range models.RegisteredModelsFor(s.Cfg, driver, endpointID) {
		return endpointID + ":" + name, nil
	}

	var opts []huh.Option[string]
	for _, e := range models.Catalog {
		if e.Kind != kind {
			continue
		}
		opts = append(opts, huh.NewOption(
			fmt.Sprintf("%s · %s · %s", e.Name, e.Size, e.Note), e.Name,
		))
	}
	opts = append(opts, huh.NewOption("← Back", pickBack))

	choice := ""
	if err := huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().
			Title(title).
			Description(desc).
			Options(opts...).
			Height(listHeight(len(opts))).
			Value(&choice),
	)).Run(); err != nil {
		return "", cancelable(err)
	}
	if choice == pickBack {
		return "", errCancelStep
	}

	var entry models.CatalogEntry
	for _, e := range models.Catalog {
		if e.Name == choice {
			entry = e
		}
	}

	modelsDir, err := platform.ModelsDir()
	if err != nil {
		return "", err
	}
	fmt.Printf("Downloading %s…\n", entry.Name)
	// FetchModelFile reuses an on-disk file (no re-download); RegisterFor
	// mutates s.Cfg, which finish() saves once at the end.
	dest, _, err := models.FetchModelFile(models.HuggingFaceBase, models.CatalogRepoFor(entry), entry.File, modelsDir, false)
	if err != nil {
		return "", fmt.Errorf("download %s: %w", entry.Name, err)
	}
	models.RegisterFor(s.Cfg, driver, endpointID, entry.Name, dest)
	s.Dirty = true
	return endpointID + ":" + entry.Name, nil
}
```

`models.FetchModelFile` skips the download when the file already exists, and `RegisterFor` mutates the in-memory `s.Cfg` (persisted by `finish`), so no disk reload is needed and re-installs are cheap.

- [ ] **Step 2: Verify it compiles in isolation** (the package as a whole builds after Task 8)

This task's file references only symbols that exist (models.*, platform.*, config.*, listHeight/cancelable/errCancelStep defined in run.go). It will compile once Task 8 removes the last dangling references in the old flow_hotkey.go.

- [ ] **Step 3: Commit**

```bash
git add internal/setup/flow_model.go
git commit -m "feat: local whisper and chat model install flows"
```

---

### Task 8: The linear wizard, dispatch, and mic flow

Wire everything into one flow and make the package build again.

**Files:**
- Rewrite: `internal/setup/flow_hotkey.go` (the stage wizard)
- Rewrite: `internal/setup/run.go` (`Run` + the entry branch + `wizard`)
- Modify: `internal/setup/flow_mic.go` (inline `SetDefaultMicrophone`, return the mic value instead of mutating)
- Test: `internal/setup/cli_dispatch_test.go` via `internal/cli/cli_test.go` (existing)

**Interfaces:**
- Consumes: everything from Tasks 3–7.
- Produces:
  - `func Run(args []string) int` — rejects any topic arg; runs the entry branch then the wizard.
  - `func runWizard(s *Session, prefill *HotkeyAnswers) error` — the eight steps.
  - `func askMicrophone(s *Session) (string, error)` — returns the chosen device name (replaces `runMicFlow`).
  - `func askCombo(s *Session, current string) (string, error)`, `func askMode(current string) (string, error)`, the three stage questions, `askTargetLanguage`, `askInstructions`.

- [ ] **Step 1: Rewrite `internal/setup/flow_mic.go`** so it returns the chosen device rather than mutating the session:

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

// askMicrophone returns the chosen default input device name ("" = system
// default). current pre-selects the existing value.
func askMicrophone(current string) (string, error) {
	opts, err := micOptions()
	if err != nil {
		return "", err
	}
	opts = append(opts, huh.NewOption("← Back", pickBack))
	choice := current
	if err := huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().
			Title("Microphone").
			Description("Used by every hotkey unless the hotkey sets its own microphone.").
			Options(opts...).
			Height(listHeight(len(opts))).
			Value(&choice),
	)).Run(); err != nil {
		return "", cancelable(err)
	}
	if choice == pickBack {
		return "", errCancelStep
	}
	return choice, nil
}
```

- [ ] **Step 2: Rewrite `internal/setup/flow_hotkey.go`** as the stage wizard:

```go
package setup

import (
	"errors"
	"fmt"
	"strings"

	"charm.land/huh/v2"

	"github.com/dmtrkzntsv/gosaid/internal/config"
	"github.com/dmtrkzntsv/gosaid/internal/hotkey"
)

const pickTypeOwn = "\x00type-own"

// askCombo picks a key combo: curated list (minus already-bound combos, except
// the one being edited) or free text validated by the hotkey parser. current
// is the combo being edited ("" when adding).
func askCombo(s *Session, current string) (string, error) {
	var opts []huh.Option[string]
	for _, c := range SuggestedCombos {
		if _, bound := s.Cfg.Hotkeys[c]; !bound || c == current {
			opts = append(opts, huh.NewOption(c, c))
		}
	}
	opts = append(opts,
		huh.NewOption("Type your own…", pickTypeOwn),
		huh.NewOption("← Back", pickBack),
	)
	choice := current
	if err := huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().Title("Shortcut").Options(opts...).
			Height(listHeight(len(opts))).Value(&choice),
	)).Run(); err != nil {
		return "", cancelable(err)
	}
	if choice == pickBack {
		return "", errCancelStep
	}
	if choice != pickTypeOwn {
		return choice, nil
	}
	combo := current
	var lockingSeen *hotkey.LockingKeyError
	if err := huh.NewForm(huh.NewGroup(
		huh.NewInput().Title("Shortcut").
			Description("Esc goes back.").
			Placeholder("ctrl+alt+space").
			Validate(func(v string) error {
				v = strings.ToLower(strings.TrimSpace(v))
				if _, _, err := hotkey.Parse(v); err != nil {
					var locking *hotkey.LockingKeyError
					if errors.As(err, &locking) {
						lockingSeen = locking
						return errors.New(locking.Short())
					}
					return err
				}
				if _, bound := s.Cfg.Hotkeys[v]; bound && v != current {
					return fmt.Errorf("%q is already bound", v)
				}
				return nil
			}).
			Value(&combo),
	)).Run(); err != nil {
		if lockingSeen != nil {
			fmt.Println(lockingSeen.Error())
		}
		return "", cancelable(err)
	}
	return strings.ToLower(strings.TrimSpace(combo)), nil
}

// askMode picks hold vs toggle.
func askMode(current string) (string, error) {
	if current == "" {
		current = string(config.ModeHold)
	}
	choice := current
	if err := huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().Title("Mode").
			Description("Hold: record while pressed. Toggle: press to start, press to stop.").
			Options(
				huh.NewOption("Hold (push-to-talk)", string(config.ModeHold)),
				huh.NewOption("Toggle", string(config.ModeToggle)),
				huh.NewOption("← Back", pickBack),
			).
			Height(listHeight(3)).
			Value(&choice),
	)).Run(); err != nil {
		return "", cancelable(err)
	}
	if choice == pickBack {
		return "", errCancelStep
	}
	return choice, nil
}

// askYesNo asks a stage enable question. Returns the bool; Esc backs out.
func askYesNo(title, desc string, current bool) (bool, error) {
	v := current
	if err := huh.NewForm(huh.NewGroup(
		huh.NewConfirm().Title(title).Description(desc).Value(&v),
	)).Run(); err != nil {
		return false, cancelable(err)
	}
	return v, nil
}

// askTargetLanguage picks the translate output language.
func askTargetLanguage(current string) (string, error) {
	var opts []huh.Option[string]
	for _, code := range config.Languages() {
		opts = append(opts, huh.NewOption(
			fmt.Sprintf("%s (%s)", config.LanguageName(code), code), code))
	}
	opts = append(opts, huh.NewOption("← Back", pickBack))
	choice := current
	if err := huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().Title("Translate to").Options(opts...).
			Height(listHeight(len(opts))).Value(&choice),
	)).Run(); err != nil {
		return "", cancelable(err)
	}
	if choice == pickBack {
		return "", errCancelStep
	}
	return choice, nil
}

// askInstructions collects the compose instructions.
func askInstructions(current string) (string, error) {
	v := current
	if err := huh.NewForm(huh.NewGroup(
		huh.NewText().Title("Compose instructions").
			Description("e.g. \"Write in a formal, business-email register.\"").
			Value(&v),
	)).Run(); err != nil {
		return "", cancelable(err)
	}
	return v, nil
}
```

- [ ] **Step 3: Rewrite `internal/setup/run.go`.** Keep the helper functions (`errCancelStep`, `listHeight`, `cancelable`, `absorbCancel`, `uncancel`, `confirmSaveAfterError`, `confirmDiscardOnAbort`, `finish`) exactly as they are now; replace only `setupUsage` and `Run`, and add `chooseEntry` + `runWizard`:

```go
const setupUsage = "usage: gosaid setup   (local setup wizard; edit config.json for cloud providers)"

// Run is the `gosaid setup` entry point: a single local-only wizard. Any
// argument is rejected — there are no sub-topics.
func Run(args []string) int {
	if len(args) > 0 {
		fmt.Fprintf(os.Stderr, "gosaid setup takes no arguments\n%s\n", setupUsage)
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

	prefill, err := chooseEntry(s)
	if err != nil {
		if errors.Is(err, errCancelStep) || errors.Is(err, huh.ErrUserAborted) {
			return 0
		}
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}

	if err := uncancel(runWizard(s, prefill)); err != nil {
		if abort, ferr := confirmDiscardOnAbort(s, err); abort {
			if ferr != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", ferr)
				return 1
			}
			fmt.Println("Changes discarded.")
			return 0
		} else if !errors.Is(err, huh.ErrUserAborted) {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			if !s.Dirty || !confirmSaveAfterError() {
				return 1
			}
		}
	}
	if err := finish(s); err != nil {
		fmt.Fprintf(os.Stderr, "config not saved: %v\n", err)
		return 1
	}
	return 0
}

// chooseEntry runs the fresh-vs-existing branch, returning the answers to
// pre-fill the wizard with (nil = fresh). It may reset the config in place.
func chooseEntry(s *Session) (*HotkeyAnswers, error) {
	if len(s.Cfg.Hotkeys) == 0 {
		return nil, nil // fresh
	}
	scratch := false
	if err := huh.NewForm(huh.NewGroup(
		huh.NewConfirm().
			Title("A config already exists. Start from scratch?").
			Description("Yes clears it (including cloud providers). No lets you edit or add a hotkey.").
			Value(&scratch),
	)).Run(); err != nil {
		return nil, cancelable(err)
	}
	if scratch {
		ResetConfig(s.Cfg)
		s.Dirty = true
		return nil, nil
	}

	// "No" — pick a hotkey to edit, or add a new one.
	combos := make([]string, 0, len(s.Cfg.Hotkeys))
	for combo := range s.Cfg.Hotkeys {
		combos = append(combos, combo)
	}
	sort.Strings(combos)
	var opts []huh.Option[string]
	for _, combo := range combos {
		opts = append(opts, huh.NewOption(HotkeySummary(combo, s.Cfg.Hotkeys[combo]), combo))
	}
	const pickAddNew = "\x00add-new"
	opts = append(opts, huh.NewOption("+ Add a new hotkey", pickAddNew))
	choice := ""
	if err := huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().Title("Which hotkey?").Options(opts...).
			Height(listHeight(len(opts))).Value(&choice),
	)).Run(); err != nil {
		return nil, cancelable(err)
	}
	if choice == pickAddNew {
		return nil, nil
	}
	a := AnswersFrom(choice, s.Cfg.Hotkeys[choice])
	return &a, nil
}

// runWizard runs the eight steps. prefill != nil seeds them from an existing
// hotkey (edit path); nil starts blank (mic from the current global setting).
func runWizard(s *Session, prefill *HotkeyAnswers) error {
	var a HotkeyAnswers
	if prefill != nil {
		a = *prefill
	} else {
		a.Mode = string(config.ModeHold)
	}

	// 1. Microphone (global).
	mic, err := askMicrophone(s.Cfg.Microphone)
	if err != nil {
		return err
	}
	if mic != s.Cfg.Microphone {
		s.Cfg.Microphone = mic
		s.Dirty = true
	}

	// 2. Transcription model → a.TranscribeRef. installWhisperModel registers
	// on s.Cfg in place, so the just-installed model is visible below.
	ref, err := installWhisperModel(s)
	if err != nil {
		return err
	}
	a.TranscribeRef = ref

	// 3. Shortcut.
	combo, err := askCombo(s, a.Combo)
	if err != nil {
		return err
	}
	a.Combo = combo

	// 4. Mode.
	mode, err := askMode(a.Mode)
	if err != nil {
		return err
	}
	a.Mode = mode

	// 5-7. Stage toggles.
	if a.Enhance, err = askYesNo("Enable enhance (clean up speech)?", "", a.Enhance); err != nil {
		return err
	}
	if a.Translate, err = askYesNo("Enable translate?", "", a.Translate); err != nil {
		return err
	}
	if a.Translate {
		if a.TargetLang, err = askTargetLanguage(a.TargetLang); err != nil {
			return err
		}
	}
	if a.Compose, err = askYesNo("Enable compose (rewrite to order)?", "", a.Compose); err != nil {
		return err
	}
	if a.Compose {
		if a.Instructions, err = askInstructions(a.Instructions); err != nil {
			return err
		}
	}

	// 8. Chat model — only when a stage needs it.
	if a.NeedsChatModel() {
		chatRef, err := installChatModel(s)
		if err != nil {
			return err
		}
		a.ChatRef = chatRef
	}

	UpsertHotkey(s.Cfg, a.Combo, BuildHotkey(a))
	s.Dirty = true
	fmt.Printf("Hotkey ready: %s\n", HotkeySummary(a.Combo, s.Cfg.Hotkeys[a.Combo]))
	return nil
}
```

- [ ] **Step 4: Update the CLI usage + dispatch.** In `internal/cli/cli.go`, the `setup` case already calls `setup.Run(args[1:])` — unchanged. Update the `Usage()` block:

```go
func Usage() {
	fmt.Println(`gosaid - headless push-to-talk voice dictation daemon

usage:
  gosaid           run the daemon
  gosaid setup     local setup wizard — transcription, a hotkey, optional local
                   chat stages. Edit config.json directly for cloud providers.
  gosaid config    open the config file in $EDITOR
  gosaid model download <hf-repo> <file>
                   download a model from Hugging Face and register it
  gosaid version   print version
  gosaid help      print this message`)
}
```

- [ ] **Step 5: Update the CLI dispatch test.** In `internal/cli/cli_test.go`, `TestDispatchSetupUnknownTopic` expects `setup bogus` → exit 2; that still holds (any arg is rejected now). `TestDispatchSetupNonTTY` expects `setup mic` → exit 1 in a non-TTY. But `setup mic` now hits the "takes no arguments" branch (exit 2) *before* the TTY check. Update it:

```go
func TestDispatchSetupRejectsArgs(t *testing.T) {
	// setup takes no arguments now — any arg is exit 2, before the TTY check.
	if code := Dispatch("test", []string{"setup", "mic"}); code != 2 {
		t.Fatalf("setup with an arg must exit 2, got %d", code)
	}
	if code := Dispatch("test", []string{"setup", "bogus"}); code != 2 {
		t.Fatalf("setup with an arg must exit 2, got %d", code)
	}
}

func TestDispatchSetupNonTTY(t *testing.T) {
	// Bare `setup` in a non-TTY (go test) must exit 1.
	if code := Dispatch("test", []string{"setup"}); code != 1 {
		t.Fatalf("bare setup without a TTY must exit 1, got %d", code)
	}
}
```

Delete the old `TestDispatchSetupUnknownTopic` if present (superseded by `TestDispatchSetupRejectsArgs`).

- [ ] **Step 6: Build, vet, full tests**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: PASS (the package builds whole again; `internal/setup` and `internal/cli` green).

- [ ] **Step 7: Manual smoke test**

Back up your real config first:
```bash
cp "$HOME/Library/Application Support/gosaid/config.json"{,.bak} 2>/dev/null || true
```
Run: `go build -o /tmp/gosaid ./cmd/gosaid && /tmp/gosaid setup`
Expected: with an existing config, "Start from scratch?" then either the hotkey list or a fresh chain; mic → transcription model (reused if installed, else download) → shortcut → mode → three stage questions → chat model only if a stage was enabled → save + restart offer. Restore: `mv "$HOME/Library/Application Support/gosaid/config.json"{.bak,} 2>/dev/null || true`.

- [ ] **Step 8: Commit**

```bash
git add internal/setup/ internal/cli/cli.go internal/cli/cli_test.go
git commit -m "feat: linear local-only setup wizard with fresh/edit entry branch"
```

---

### Task 9: Documentation

**Files:**
- Modify: `README.md`

**Interfaces:** none.

- [ ] **Step 1: Update the README.** Find setup/model references with `grep -n "gosaid setup\|local-llm\|local:" README.md` and update:
  1. In the setup section, describe `gosaid setup` as the local wizard: "walks you through a transcription model, a hotkey, and optional local chat stages (enhance/translate/compose). For cloud providers, edit config.json."
  2. Any `local:`/`local-llm:` model-ref examples become `speech:`/`text:`.
  3. If a model-download example names an endpoint, note the new defaults (`speech` for `.bin`, `text` for `.gguf`).

- [ ] **Step 2: Verify no stale references**

Run: `grep -rn "setup hotkey\|setup provider\|setup model\|setup mic\|local-llm" README.md`
Expected: no output.

- [ ] **Step 3: Final check**

Run: `go test ./... && go vet ./... && gofmt -l internal/ cmd/`
Expected: tests pass, vet clean, no unformatted files.

- [ ] **Step 4: Commit**

```bash
git add README.md
git commit -m "docs: document the local-only gosaid setup"
```

---

## Plan Self-Review (completed)

- **Spec coverage:** entry branch (Task 8 `chooseEntry`), eight steps (Task 8 `runWizard`), catalog with chat models + Repo (Task 2), speech/text ids (Task 1), driver-aware install (Task 7), independent stage flags (Task 4), one-chat-model rule (Task 4 `NeedsChatModel` + Task 8 step 8), reuse-not-redownload (Task 7 early-return + models.Download's existing skip), scratch resets incl. cloud (Task 6 `ResetConfig`), edit pre-fill (Task 4 `AnswersFrom` + Task 8), deletions (Tasks 5, 6), no-arg dispatch (Task 8), docs (Task 9).
- **Type consistency:** `HotkeyAnswers` fields identical across recipe.go and run.go; endpoint refs always `speech:<n>` / `text:<n>`; `installWhisperModel`/`installChatModel` return `(string, error)`; `pickBack`/`pickTypeOwn` sentinels defined once (flow_model.go / flow_hotkey.go respectively).
- **Known judgment calls encoded:** the package intentionally does not build between Tasks 4–7 (recipe API deleted before its consumers are rewritten); Task 8 restores a green tree, and Tasks 5–8 should land as a sequence. `models.Download` persists to disk itself, so `runWizard` reloads `s.Cfg` after each install (Task 8 step 4). "Reuse if installed" picks the first registered model of that kind — fine because the wizard only ever installs one per endpoint.
