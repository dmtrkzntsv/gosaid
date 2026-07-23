# Local-Only Setup — Design

**Date:** 2026-07-19
**Status:** Approved
**Supersedes:** the interactive `gosaid setup` design in
`2026-07-18-interactive-setup-design.md` (provider manager, presets, hub menu,
topic subcommands). The pieces reused from it are noted inline.

## Goal

Make `gosaid setup` a short, linear wizard that configures a **fully local**
GoSaid: a Whisper transcription model, one hotkey, and optionally local chat
stages (enhance / translate / compose) backed by a llama.cpp model. Cloud
providers are out of scope for setup — users who want them edit config.json.

## Rationale

Cloud setup means API keys, endpoint ids, base URLs, and per-provider model
names — the bulk of the previous design's complexity, for a path many users
never take. Local setup needs none of that: models are downloaded from a
curated list, and there is exactly one local transcription endpoint (`speech`,
whisper_cpp) and one local chat endpoint (`text`, llama_cpp). Dropping
cloud from setup removes the provider manager, presets, the hub menu, and the
topic subcommands — a large net reduction — while covering the common case in
one guided pass.

## CLI surface

```
gosaid setup     run the local setup wizard (start to finish)
gosaid config    unchanged — open config.json in $EDITOR (the escape hatch for
                 cloud providers, per-stage models, toggle mode, etc.)
gosaid model download <repo> <file>   unchanged — scriptable model install
```

Removed: the `setup hotkey|provider|model|mic` topic subcommands. `gosaid
setup` is the only entry point; any topic argument is rejected with usage.

## Entry: fresh config vs. existing

The first thing `gosaid setup` does depends on whether a usable config already
exists (any hotkey configured):

- **No config yet** → run the wizard fresh (steps 1–8).
- **Config exists** → ask **"Start from scratch?"** (yes/no):
  - **Yes** → reset the config to empty — drop all hotkeys *and* endpoints,
    including any cloud ones — then run the wizard fresh. Model *files* on disk
    are kept, so the model steps re-register without re-downloading; only a
    genuinely missing model downloads.
  - **No** → ask **"Which hotkey?"**: a list of the existing hotkeys (each with
    its `HotkeySummary`) plus **"+ Add a new hotkey"**.
    - Picking an existing hotkey runs the wizard **pre-filled from it**
      (microphone from the global setting, transcription model, mode, each
      stage's enabled/disabled state, translate language, compose
      instructions), editing that hotkey in place.
    - "+ Add a new hotkey" runs the wizard fresh, except the microphone step is
      pre-filled from the current global setting.

This is the only branch in the flow; every path below is the same eight-step
wizard, differing only in whether the steps start blank or pre-filled.

## The wizard

One linear chain, run top to bottom. Steps default to blank on a fresh run and
to the chosen hotkey's current values when editing. Re-running `gosaid setup`
and choosing "+ Add a new hotkey" adds another binding; already-installed
models are reused, not re-downloaded.

1. **Microphone** — select from the audio device enumeration (system default
   marked) or "System default". Sets the global `microphone`.
2. **Transcription model** — pick `small` (~488 MB) or `turbo` (~550 MB);
   download and register under the `speech` whisper_cpp endpoint. Skipped when
   a whisper model is already installed (the wizard uses the existing one).
3. **Shortcut** — curated combo list (option/ctrl + arrows, F-keys, …) minus
   already-bound combos, plus "Type your own". Free text is validated by
   `hotkey.Parse`, which already rejects locking keys (Caps Lock, Fn, …) with
   a clear message.
4. **Mode** — hold (push-to-talk, default) or toggle.
5. **Enhance (clean up speech)?** — yes/no.
6. **Translate?** — yes/no; if yes, pick a target language from
   `config/languages.go`.
7. **Compose (rewrite to order)?** — yes/no; if yes, enter free-text
   instructions.
8. **Chat model** — shown only when at least one of 5–7 is enabled: pick
   `qwen3.5-0.8b` (~563 MB) or `gemma-4-e2b` (~2.8 GB); download and register
   under the `text` llama_cpp endpoint. The one chosen model backs every
   enabled stage. Skipped when a llama_cpp model is already installed (that
   one is reused). Per-stage models remain possible by editing config.json.

Then: save (validated), and offer the daemon restart (reused from the prior
design's `offerRestart`).

### Esc / back

Esc backs out one step, reusing the existing `errCancelStep` mechanism. Esc
on step 1 leaves setup. A step reached by Esc re-shows the previous prompt.
Because the chain is required end-to-end, backing out of a step the config
depends on (transcription model, shortcut) and confirming discard exits
without saving, as today.

## Model catalog

Two curated lists, both from `ggml-org` on Hugging Face (the llama.cpp
maintainers' org, so quantizations track format changes). Whisper models
download from `CatalogRepo` (`ggerganov/whisper.cpp`); chat models each carry
their own repo.

**Transcription (whisper_cpp, endpoint `speech`)** — unchanged from the current
catalog:

| name | file | size |
|---|---|---|
| `small` | `ggml-small.bin` | ~488 MB |
| `turbo` | `ggml-large-v3-turbo-q5_0.bin` | ~550 MB |

**Chat (llama_cpp, endpoint `text`)** — new:

| name | repo | file | size |
|---|---|---|---|
| `qwen3.5-0.8b` | `ggml-org/Qwen3.5-0.8B-GGUF` | `Qwen3.5-0.8B-Q4_0.gguf` | ~563 MB |
| `gemma-4-e2b` | `ggml-org/gemma-4-E2B-it-GGUF` | `gemma-4-E2B-it-Q4_0.gguf` | ~2.8 GB |

The chat entries need their own repo per model (whisper entries share one), so
`CatalogEntry` gains a `Repo` field, empty for whisper entries (they fall back
to `CatalogRepo`). The registered name is always the catalog's explicit
`Name`, not `DeriveName(File)` — verified, `DeriveName` would otherwise give
`Qwen3.5-0.8B` / `gemma-4-E2B-it` for the chat files and `large-v3-turbo-q5_0`
for turbo, so the friendly lowercase names in this table come from `Name`
alone.

Anything outside these lists is installed with `gosaid model download`.

## Config produced

A hotkey the wizard writes, with all stages enabled:

```json
{
  "option+space": {
    "mode": "hold",
    "transcribe": { "model": "speech:turbo" },
    "enhance":    { "model": "text:gemma-4-e2b" },
    "translate":  { "output_language": "en", "model": "text:gemma-4-e2b" },
    "compose":    { "model": "text:gemma-4-e2b", "instructions": "…" }
  }
}
```

The global `microphone` field (from the prior design) carries the step-1
choice. No config version bump — every field already exists.

## Architecture

`internal/setup` shrinks to the wizard plus the pieces it still needs.

**Kept (from the prior design):**
- `session.go` — `Session`, `LoadSession`, `Save` (validate-then-write),
  `PendingDeletes`.
- `run.go` — `Run`, `finish`, the `errCancelStep`/`cancelable`/`absorbCancel`
  cancel helpers, `listHeight`, the restart offer wiring.
- `restart.go` (+ platform files) — `offerRestart`, `daemonRunningAt`.
- `flow_mic.go` — the microphone picker (now step 1).
- `summary.go`'s `HotkeySummary` — for the already-bound-combo display and the
  saved-hotkey confirmation.
- `SuggestedCombos`, `config.Languages()`, `hotkey.Parse`.
- `apply.go`'s `UpsertHotkey`, and `recipe.go`'s `HotkeyAnswers`,
  `BuildHotkey`, and `AnswersFrom` — reworked to the local-only shape (below).
  The `RecipeOf` recipe classifier and the recipe-name constants go, since the
  wizard no longer has a recipe menu; stages are now independent yes/no flags.

**Removed:**
- `flow_provider.go` (provider manager, presets add/edit/delete, reassignment).
- `presets.go` + `presets_test.go` (the cloud preset table).
- `hub.go` (the menu and first-run branch — the wizard is always the same
  chain now).
- The provider/model/reassignment mutators in `apply.go`:
  `AddOpenAIEndpoint`, `UpdateOpenAIEndpoint`, `DeleteEndpoint`,
  `HotkeysUsingEndpoint`, `ReassignEndpoint`, `DeleteEndpointBlocked`,
  `DeleteHotkeyBlocked`, `EndpointIDInUse`, `validateEndpointID`,
  `FirstRun`, `ResetForFirstRun`, `SetDefaultMicrophone` (folded into the mic
  flow), plus their tests. `PresetForAPIBase`, `ChatModelOptions`,
  `TranscribeModelOptions`, `OpenAIEndpointIDs` go too — the wizard's model
  choices come from the catalog, not from configured endpoints.
- The `setup hotkey|provider|model|mic` dispatch in `run.go` and the topic
  routing.

**New / reworked:**
- `flow_model.go` becomes `installWhisperModel(s) (name string, err error)`
  and `installChatModel(s) (ref string, err error)` — each shows its catalog
  list (skipping the picker when a model of that driver is already
  registered), downloads the choice via `models.Download` with the right
  driver, and returns the model name / `endpoint:model` ref. No provider
  naming prompt: the endpoint ids are fixed (`speech`, `text`).
- The endpoint-id constants in `internal/models` change:
  `DefaultWhisperEndpoint` `"local"` → `"speech"`, `DefaultLlamaEndpoint`
  `"local-llm"` → `"text"`. This also changes the default endpoint
  `gosaid model download` registers under, so its `--endpoint` help text and
  any README mention update to match. Existing `local:`/`local-llm:` configs
  keep working — nothing rewrites them; only new configs use the new ids.
- `flow_hotkey.go` becomes the linear stage wizard: combo → mode → the three
  yes/no stage questions → returns a `HotkeyAnswers`. On the edit path it is
  seeded from the chosen hotkey via a pre-fill helper (`AnswersFrom`, kept
  from the prior design and trimmed to the local-only fields). It no longer
  offers a recipe menu or per-stage model pickers; the transcribe model is the
  installed whisper model, and every enabled chat stage uses the single chat
  ref from step 8.
- `run.go`'s `Run` runs the eight steps in sequence instead of dispatching a
  topic, then calls `finish`.

`models.Download` and `DownloadDefaults` (from the llama.cpp merge) already
register `.gguf` under llama_cpp and `.bin` under whisper_cpp, so the wizard
passes the catalog entry's file and lets the driver follow from the
extension. The chat entries are `.gguf`, so they land under `text`
automatically.

`models.RegisteredModels` is whisper-only (it hardcodes `DriverWhisperCPP`),
so the "is a chat model already installed?" check needs a driver-aware lookup.
Add `models.RegisteredModelsFor(cfg, driver, endpointID)` — the existing
`RegisteredModels` becomes a thin wrapper over it with `DriverWhisperCPP`,
mirroring how `Register`/`RegisterFor` were split in the merge.

## Existing configs

The wizard never edits or deletes existing endpoints or hotkeys — it only
adds. A user with a cloud `openai` endpoint keeps it; running setup adds a
new local hotkey alongside their existing ones, pointing at local models. Their
cloud hotkeys are untouched. Setup simply doesn't surface cloud endpoints as
choices.

## Error handling

- **Download failure** — shown inline; the wizard stops before writing the
  hotkey (a hotkey referencing an un-downloaded model wouldn't validate), so
  nothing is saved. Re-running retries.
- **Non-TTY stdin** — `setup` exits with "requires an interactive terminal"
  (unchanged).
- **Save validation failure** — should be unreachable (the wizard only
  produces valid hotkeys), but if it happens the error is printed and nothing
  is written.

## Testing

- Pure layer: catalog shape (unique names, colon-free, chat entries have a
  repo, `.gguf` files), `HotkeyAnswers` → `config.Hotkey` construction for
  each combination of enabled stages, the "chat model asked only when a stage
  is enabled" predicate.
- The download/registration path is already covered in `internal/models`
  (including the GGUF-under-llama_cpp case).
- Flow wiring (the huh forms) stays untested, as before.

## Out of scope

- Cloud provider setup (config.json).
- Per-stage chat models, toggle-vs-hold beyond the one mode question,
  per-hotkey microphone override (config.json).
- Removing or editing existing models/hotkeys from setup (config.json, or
  `gosaid model download` for adding more).
