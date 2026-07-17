# Local Whisper Models — Design

Date: 2026-07-16
Status: Approved

## Goal

Let GoSaid transcribe speech locally, with no cloud API, using whisper.cpp
embedded in the binary via cgo. Users download a model from Hugging Face with a
CLI command that also registers the model in `config.json`. Local support
covers the **transcribe** stage only; enhance/compose/translate keep using
OpenAI-compatible endpoints (cloud or user-run local servers).

Ships on all five release targets from day one: darwin/linux/windows ×
amd64/arm64 (windows amd64 only, matching the current matrix).

## Non-goals

- Local LLM inference for chat stages (enhance/compose/translate).
- Managing external inference server processes.
- Checksum verification or HF revision pinning for downloads (v1 downloads
  from the `main` branch over TLS).
- Idle eviction of loaded models from memory.
- Curated model catalog / `gosaid model list` command.

## Config schema & routing

New driver type `whisper_cpp` in the existing `drivers` array. Its endpoint
config holds a map of model names to GGML model file paths instead of
`api_base`/`api_key`:

```json
"drivers": [
  { "driver": "openai_compatible", "endpoints": [ ... ] },
  {
    "driver": "whisper_cpp",
    "endpoints": [
      {
        "id": "local",
        "config": {
          "models": {
            "base": "~/Library/Application Support/gosaid/models/ggml-base.bin"
          }
        }
      }
    ]
  }
]
```

Hotkeys reference local models with the existing `<endpoint_id>:<model>`
syntax: `"transcribe": { "model": "local:base" }`. `routing.ParseModelRef`
and registry lookup are unchanged.

`config.EndpointConfig` gains an optional `models` field
(`map[string]string`). Validation branches on driver type:

- `openai_compatible`: `api_base` and `api_key` required (unchanged).
- `whisper_cpp`: non-empty `models` map required; each path (after `~`
  expansion) must exist on disk — fail fast at startup with the endpoint and
  model name in the error.
- `enhance`/`compose`/`translate` model refs must not point at a
  `whisper_cpp` endpoint (whisper cannot chat) — rejected at validation time.
- `transcribe.output_language: "en"` works locally via whisper's native
  translate-to-English task.

## CLI: `gosaid model download`

```
gosaid model download <hf-repo> <file> [--name <name>] [--endpoint <id>] [--force]
```

Example: `gosaid model download ggerganov/whisper.cpp ggml-base.bin`

- Downloads `https://huggingface.co/<repo>/resolve/main/<file>` to a `models/`
  directory next to the config file (e.g.
  `~/Library/Application Support/gosaid/models/` on macOS). Streams to a
  `.part` temp file with a progress indicator; renames atomically on success,
  so failures leave no partial files.
- `--name` sets the model name used in config and hotkey refs. Default:
  filename with `ggml-` prefix and extension stripped (`ggml-base.bin` →
  `base`).
- On success, edits `config.json` via the existing config store: creates the
  `whisper_cpp` driver block and endpoint (default id `local`, `--endpoint`
  overrides) if missing, then sets `models[<name>] = <absolute path>`. Config
  is written only after the file is fully in place.
- Refuses to overwrite an already-registered model name on that endpoint;
  `--force` overwrites both file and config entry.
- Uses a tolerant config load (parse without full validation) so the command
  works on a half-written config.
- Prints a ready-to-paste hotkey snippet on success:
  `"transcribe": { "model": "local:base" }`.

## Driver & cgo wrapper

Two new pieces; cgo is isolated to one package.

### `internal/whisper` (cgo)

Thin wrapper over vendored whisper.cpp:

```go
type Model struct { ... }                      // wraps *C.whisper_context
func Load(path string) (*Model, error)
func (m *Model) Close()
func (m *Model) Transcribe(samples []float32, opts Options) (Result, error)
// Options: Language ("" = auto-detect), Translate bool, InitialPrompt string
// Result: Text, DetectedLanguage
```

- Converts input audio to whisper's required 16 kHz mono, reusing the
  resampler in `internal/audio`.
- Runs `whisper_full` with greedy sampling, threads = min(4, NumCPU),
  concatenates output segments.
- A `sync.Mutex` per model serializes calls (whisper contexts are not
  thread-safe; GoSaid processes one utterance at a time anyway).

### `internal/drivers/whisper_cpp.go` (no cgo)

Implements the existing `Driver` interface:

- `Transcribe` / `TranslateSpeech`: look up the named model, delegate to
  `internal/whisper` (`TranslateSpeech` sets whisper's native translate task).
- `Chat`: returns "whisper_cpp endpoints do not support chat stages" as a
  backstop; validation already prevents this.
- Lazy loading with caching: a model loads into memory on first use and stays
  resident (first press pays ~0.1–2 s load; later presses are instant). Load
  failures are returned per-request — the daemon stays up, the model is not
  cached, and a retry re-attempts the load.

`drivers.BuildRegistry` gains a `whisper_cpp` branch constructing one driver
instance per endpoint from its `models` map.

## Vendoring, build & CI

- whisper.cpp sources pinned to a stable release tag under
  `third_party/whisper.cpp/` — only what compilation needs: `whisper.cpp/.h`,
  ggml C/C++ sources, Metal shader. Committed directly (no submodule) so
  `git clone && go build` works with no extra steps.
- `third_party/whisper.cpp/VERSION` records the pinned tag; a
  `make vendor-whisper` target re-vendors from upstream.
- cgo flags live in `internal/whisper` behind per-platform build constraints:
  - **darwin**: Metal + Accelerate (`-framework Metal -framework Foundation
    -framework Accelerate`, `GGML_USE_METAL`, embedded shader library) —
    GPU-accelerated on all Macs.
  - **linux**: CPU build (`-lm -lstdc++ -pthread`); no new apt packages in CI.
  - **windows**: CPU build under mingw-w64 (preinstalled on `windows-latest`).
- CPU builds rely on ggml's runtime SIMD dispatch — no `-march=native`;
  release binaries stay portable.
- Release matrix unchanged (all targets already build natively with
  `CGO_ENABLED=1`). Expect a few minutes more C++ compile time per target and
  ~5–10 MB binary growth. macOS sign/notarize flow untouched (static link, no
  new dylibs).
- Contingency: if mingw cannot compile some ggml source on Windows, a
  `nowhisper` build tag can disable the feature for that leg — the plan
  targets all five legs, this is a documented fallback only.

## Error handling

- Startup (validation): missing/unreadable model file, empty `models` map, or
  chat stage referencing a `whisper_cpp` endpoint → error naming the
  endpoint/model, consistent with existing validation messages.
- Runtime: model load or inference failure → the press fails through the same
  error path as cloud drivers (logged, error cue); daemon stays up.
- Download: network/HTTP failure leaves no partial files and no config edits.

## Testing

- Unit tests (no cgo required): validation rules for the new driver type,
  registry construction, chat-stage rejection, model-name derivation, and the
  download command against an `httptest` server with a temp config dir.
- cgo wrapper: integration test gated behind `GOSAID_WHISPER_MODEL` (path to a
  real model file; runs a canned WAV through `Transcribe`). Skipped when the
  variable is unset, keeping `go test ./...` fast and hermetic. Run manually
  on macOS.
- Cross-platform compile coverage comes from the existing five-target release
  workflow on every push to main.

## README

Add a "Local transcription" section: the download command, the resulting
config block, the hotkey ref syntax, RAM expectations per model size, and the
note that chat stages still need an OpenAI-compatible endpoint.
