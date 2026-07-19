# Interactive Setup CLI — Design

**Date:** 2026-07-18
**Status:** Approved

## Goal

Give users an interactive, guided way to configure GoSaid — hotkeys, providers,
local Whisper models, and the default microphone — without hand-editing
config.json. Optimize for the first-run experience: `brew install → gosaid
setup → dictating`.

## CLI surface

```
gosaid setup             interactive hub: Providers / Hotkeys / Microphone / Exit
gosaid setup hotkey      jump straight to the hotkey manager
gosaid setup provider    jump straight to the provider manager
gosaid setup model       jump straight to the local model manager
gosaid setup mic         jump straight to default-microphone selection
gosaid config            unchanged — opens config.json in $EDITOR (escape hatch)
gosaid model download    unchanged — scriptable, non-interactive model download
```

Removed: `gosaid mic` (its only subcommand, `list`, is superseded by `setup
mic`). Update the README and the per-hotkey `Microphone` doc comment in
`internal/config/config.go`, both of which reference `gosaid mic list`.

`gosaid setup` opens a menu that routes into the three managers and returns to
the menu after each. Local models are not a top-level entry — they are one
provider's detail, reached through Providers → Local Whisper (or directly via
`gosaid setup model`). Choosing *Exit* (or Esc/Ctrl+C) proceeds to save. When
invoked as `gosaid setup <topic>`, finishing that topic goes straight to save —
no hub menu.

Every screen below the hub offers a `← Back` entry that returns one level up,
and Esc does the same: backing out of a step never ends the session. Only the
hub's own menu treats Esc as leaving setup, where it matches *Exit*.

**First run:** if config.json is missing or has no endpoints, `setup` skips the
menu and chains: add provider → add first hotkey → pick mic → save.

## UI toolkit

`charmbracelet/huh` forms (select lists, multi-select, masked inputs, inline
validation). No raw bubbletea app — the flows are linear wizards.

## Architecture

New package `internal/setup` owns all interactive flows. Each flow is split
into two layers:

- **Pure logic** — build option lists from `config.Config` + system state
  (mic enumeration, registered models); apply validated answers as mutations
  on a `*config.Config`. Unit-testable without a terminal.
- **Form wiring** — thin `huh` forms that collect answers and call the pure
  layer.

`internal/cli` gains only dispatch for `setup [hotkey|provider|model|mic]`.
The daemon changes only for the global-microphone resolution rule (below).

## Managers

### Hotkey manager

Select list of existing hotkeys, one summary line each
(`option+right · hold · transcribe → enhance → translate(en)`), plus
`+ Add new hotkey` and `← Back`. Picking a hotkey opens *Edit / Delete / Back*;
Delete confirms first; Edit re-runs the wizard pre-filled.

**Add wizard (recipe-first):**

1. **Key combo** — curated select (option/ctrl + arrows, F-keys, …) with
   already-bound combos marked, plus "Type your own" free text validated
   against the hotkey parser.
2. **Recipe** — *Just transcribe / Transcribe + clean up / Translate /
   Compose*. Chat-dependent recipes (clean up, translate, compose) are shown
   disabled with a hint when no chat-capable endpoint exists (whisper_cpp is
   transcription-only).
3. **Recipe-specific** — target language for translate (from
   `config/languages.go`); instructions text for compose.
4. **Models** — auto-selected from configured endpoints using the preset
   model suggestions; ask only when more than one endpoint can serve a stage.
5. **Mode** — hold (default) / toggle.
6. **Advanced** (optional confirm, default no) — per-hotkey microphone
   override.

### Provider manager

Select list of existing endpoints (`openai · api.openai.com`,
`local · whisper_cpp (2 models)`), plus `+ Add new provider` and `← Back`.
Edit updates API key/base (cloud) or routes to the model manager (local).
Delete warns when hotkeys reference the endpoint: list them, and require
reassignment to another compatible endpoint when one exists; otherwise offer
to delete the referencing hotkeys instead. Deletions that would leave the
config with no provider or no hotkey are blocked (the daemon requires at
least one of each).

**Add (preset-driven).** Preset list, in order: **Local Whisper (on-device)**,
OpenAI, Groq, OpenRouter, Custom (OpenAI-compatible). Providers without a
preset are reached through Custom, which asks for the api_base and model ids
directly.

- *Cloud presets* prefill `api_base`; ask endpoint id (defaulted, e.g.
  `openai`) and API key (masked, non-empty). Each preset carries suggested
  transcribe + chat model ids consumed by the hotkey wizard.
- *Custom* additionally asks for `api_base` and model ids to suggest.
- *Local Whisper* routes into the model manager.

Presets live in a small table in `internal/setup` — easy to extend.

### Local model manager

Multi-select checklist:

- **Curated models** from the official `ggerganov/whisper.cpp` HF repo — a
  deliberately short list covering the useful range, each with its approximate
  download size and a one-phrase note: `small` (~488 MB, fast on plain CPU),
  `turbo` (~550 MB, the quantized `large-v3-turbo-q5_0` the README already
  recommends as the default), `large-v3` (~3.1 GB, highest accuracy). A
  curated model's registered name need not match its filename — `turbo` is
  friendlier than `large-v3-turbo-q5_0`. Models already registered on the
  `local` endpoint are pre-checked; models registered outside the catalog
  appear pre-checked as "custom".
- **`+ Custom model`** asks for a Hugging Face link, which the user can paste
  straight from the browser: the `resolve`/`blob` URL shapes and the bare
  `owner/repo/file` form are all accepted, and the repo and file are parsed
  out of it.

On confirm, apply the diff:

- *Newly checked* → download via the existing progress-streaming fetch in the
  model-download internals (skip if the file already exists on disk), register
  on the `local` endpoint.
- *Unchecked* → unregister from config. If a hotkey references
  `local:<name>`, offer to delete those hotkeys too; declining keeps the
  model. Removals that would leave no provider are blocked (the daemon
  requires at least one). Then ask once whether to also delete the model
  files from disk (default yes).

Downloads execute immediately on confirm (disk side effects); the config
registration rides the final save like every other change. Model file
*deletions*, by contrast, are deferred: they are only applied to disk after
a successful save, so a discarded session never leaves config.json
referencing a file that's already gone.

### Microphone

Select list from the audio device enumeration (system default marked) plus a
"System default" entry that clears the setting. Writes the new global field.

## Config schema change

`Config` gains one additive field:

```go
// Microphone is the default input device by name — case-insensitive
// substring match. Empty uses the system default. A hotkey's own
// Microphone field overrides this.
Microphone string `json:"microphone,omitempty"`
```

Daemon resolution order: per-hotkey `microphone` → global `microphone` →
system default. No config version bump — old configs remain valid.

## Save & restart

- Flows mutate an in-memory `config.Config` copy; nothing is written until the
  user finishes (hub *Done* or end of a `setup <topic>` run).
- On finish: run `validate.go` checks → write via the existing atomic store.
  Validation failure (should be impossible — flows only offer valid choices)
  shows the error and re-enters the menu instead of writing.
- Ctrl+C / Esc abandons unsaved changes, with a "discard changes?" confirm
  when anything was modified.
- After a successful save, if state.json shows a running daemon: offer
  "Restart the daemon to apply changes?" — run `brew services restart gosaid`
  when brew manages it, otherwise print the platform-appropriate manual
  instruction. If no daemon is running, print how to start one.
- After a successful save, any pending model-file deletions (see Local model
  manager) are applied to disk.
- A flow error that isn't a Ctrl+C/Esc abort no longer discards unsaved work
  outright: if the session has unsaved changes, setup offers to save them
  before exiting.

## Error handling

- **No providers + hotkey manager** — chat-dependent recipes disabled with
  "add a provider first"; *Just transcribe* works if a whisper_cpp endpoint
  exists.
- **Provider delete with references** — blocked behind reassignment or
  explicit confirmation (see Provider manager).
- **Download failure** — shown inline; model stays unchecked; flow continues.
  The existing `.part` + atomic-rename pattern guarantees no torn files.
- **Non-TTY stdin** — `setup` exits with "requires an interactive terminal".

## Testing

- Table-driven unit tests for the pure layer: option-list builders, config
  mutators, endpoint-reference checks, preset table, mic/model resolution.
- Form wiring stays thin and is not directly tested (standard practice for
  `huh` apps).
- Existing model-download tests continue to cover the shared internals.

## Out of scope

- Config hot-reload in the daemon (future feature; restart offer covers it).
- Live hotkey capture ("press the combo now") — combos are picked from a list
  or typed.
- Windows service management for the restart offer (prints manual instructions
  there).
