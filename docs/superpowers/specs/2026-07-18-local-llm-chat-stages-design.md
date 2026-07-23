# Local LLM for Chat Stages — Design

Date: 2026-07-18
Status: Approved

## Goal

Let the chat stages (`enhance`, `compose`, `translate`) run fully locally by
embedding llama.cpp in the binary via cgo, the same way whisper.cpp is
embedded for `transcribe`. Users download a GGUF model (e.g. a Gemma-class
instruct model) from Hugging Face with the existing `gosaid model download`
command, which registers it in `config.json`; hotkeys reference it with the
existing `<endpoint_id>:<model>` syntax. No external runtime (Ollama,
llama-server) is required or managed — GoSaid stays a single static binary.

All three chat stages go through the one `Driver.Chat` method, so local
support covers enhance, compose, and translate at once. Mixed per-stage
models (small model for enhance, larger for compose) work through the
existing per-stage model refs.

Ships on all five release targets: darwin/linux/windows × amd64/arm64
(windows amd64 only), matching the whisper.cpp matrix.

## Non-goals

- A curated model registry or alias shortcuts (`gosaid model download gemma`)
  — HF repo + filename, same as whisper today.
- Local transcription changes — `whisper_cpp` behavior is untouched (its
  vendored sources move, but the driver and config surface are unchanged).
- Multi-turn chat, KV-cache reuse across dictations, or streaming output —
  each stage call is a stateless single-turn completion.
- Vendoring llama.cpp's `common/` library, server, or multimodal support —
  only the core library (`src/`, `include/llama.h`) is embedded.
- Custom/user-supplied chat templates. Models without an embedded template
  are rejected with a clear error.

## Vendored source layout & build

whisper.cpp and llama.cpp each bundle ggml, but one binary can link only one
copy of ggml's C symbols. ggml is therefore promoted to a shared,
singly-vendored library:

```
internal/ggml/cvendor/     ggml headers + src, ggml-cpu, ggml-metal
                           (moved out of internal/whisper/cvendor)
internal/whisper/cvendor/  whisper.h + whisper.cpp src only;
                           compiles against internal/ggml headers
internal/llama/cvendor/    llama.h + llama.cpp src (llama-*.cpp,
                           unicode.cpp, …); same ggml headers
```

The Go link shims (base, cpu, metal-on-darwin, per-arch CPU backends) move
from `internal/whisper` to `internal/ggml`; `internal/whisper` and
`internal/llama` both blank-import them.

Version pairing: both upstreams sync ggml from the same repository. A pinned
(whisper.cpp tag, llama.cpp tag) pair whose ggml versions match is recorded
in a single `VERSION` manifest; ggml is vendored **from the llama.cpp tag**
(its ggml sync is the most current). `scripts/vendor-whisper.sh` splits into
`vendor-ggml.sh`, `vendor-whisper.sh`, and `vendor-llama.sh`, with a
`make vendor` umbrella that runs all three so the trio cannot drift
independently. The Metal-shader embedding (`.incbin` asm stub, inlined
headers) moves into the ggml vendoring script unchanged.

Platform flags mirror whisper's: Metal + Accelerate on darwin, portable CPU
builds (runtime SIMD dispatch, no `-march=native`) on linux/windows. The
`nowhisper`-style contingency applies: if a platform leg cannot compile
llama.cpp, a build tag can disable local chat for that leg only.

Costs accepted: vendored source grows ~10–15 MB, the binary an estimated
5–10 MB, and clean-build time roughly doubles. Upgrading either library now
means re-verifying the pinned pair.

## `internal/llama` (cgo)

Thin wrapper over vendored llama.cpp, mirroring `internal/whisper`'s shape:

```go
type Model struct { ... }                    // wraps llama_model; mutex-serialized
func Load(path string) (*Model, error)      // reads GGUF; weights on Metal (darwin) / CPU
func (m *Model) Close()
func (m *Model) Chat(ctx context.Context, system, user string, opts Options) (string, error)
// Options: MaxTokens int (cap on generated tokens; default 1024)
```

Inside `Chat`:

1. **Prompt formatting** via `llama_chat_apply_template()` using the model's
   embedded chat template (GGUF files from Gemma, Qwen, Llama, etc. all ship
   one). System + user messages in, formatted prompt out. A model with no
   template fails with an error naming the model — wrong-template gibberish
   typed into the user's editor is worse than a clean failure.
2. **Fresh `llama_context` per call**, freed on return. Context creation is
   cheap relative to a generation, and stateless single-turn calls avoid all
   KV-cache management. Context size: the model's trained context, capped at
   8192 tokens.
3. **Sampling**: near-greedy (temperature ≈ 0.2 with min-p) via the
   `llama_sampler` chain API. Enhance/translate want faithfulness; compose
   still phrases naturally.
4. **Generation loop**: decode → sample → append until an EOG token or
   `MaxTokens`; detokenize incrementally and return the accumulated string.
   `ctx.Err()` is checked before starting and between decode steps, so a
   cancelled pipeline stops generation promptly. On `MaxTokens`, the
   truncated text is returned (not an error).

A per-model `sync.Mutex` serializes inference, as with whisper.

## Driver, config & validation

### `internal/drivers/llama_cpp.go` (no cgo)

`LlamaCPP` implements the `Driver` interface: `Chat` acquires the named
model and delegates to `internal/llama`; `Transcribe`/`TranslateSpeech`
return "llama_cpp endpoints do not support transcription" as backstops
(validation prevents these refs).

The model cache in `WhisperCPP` — lazy load, in-flight counting, idle-unload
timer, failed-load-not-cached retry — is extracted into a small generic
`modelCache[M]` (acquire/release/maybeUnload, parameterized over the loaded
model type) used by both drivers, rather than duplicating ~80 lines of
concurrency-sensitive code.

### Config schema

New driver type `llama_cpp`, reusing the existing endpoint-config fields
`models` (name → GGUF path) and `unload_after_seconds`:

```json
{
  "driver": "llama_cpp",
  "endpoints": [
    {
      "id": "local-llm",
      "config": {
        "models": { "gemma": "~/Library/Application Support/gosaid/models/gemma-3-4b-it-Q4_K_M.gguf" },
        "unload_after_seconds": 300
      }
    }
  ]
}
```

Hotkey usage is unchanged syntax, mixed per-stage:

```json
"enhance":   { "model": "local-llm:gemma" },
"translate": { "model": "local-llm:gemma", "output_language": "en" }
```

### Validation

- `models` non-empty and each path (after `~` expansion) exists on disk —
  required for both local drivers; `unload_after_seconds` allowed for both
  (error message updated from "whisper_cpp only").
- Chat-stage refs must not point at `whisper_cpp` (existing rule);
  transcribe-stage refs must not point at `llama_cpp` (new mirror rule:
  "endpoint %q is llama_cpp, which supports chat stages only").
- `drivers.BuildRegistry` gains one `llama_cpp` switch case.

## CLI: `gosaid model download`

The existing command grows one branch: **file extension decides the driver**.
`.gguf` → register under a `llama_cpp` endpoint (default id `local-llm`);
anything else keeps today's whisper behavior (default id `local`).

```
gosaid model download ggml-org/gemma-3-4b-it-GGUF gemma-3-4b-it-Q4_K_M.gguf --name gemma
```

- `--name`, `--endpoint`, `--force` work identically. Name derivation for
  GGUF strips the extension and a trailing quantization suffix
  (`-Q4_K_M`, `-q4_0`, …): `gemma-3-4b-it-Q4_K_M.gguf` → `gemma-3-4b-it`.
- Download mechanics (streaming `.part` file, atomic rename, progress line,
  config edit only after the file is in place) are shared with whisper —
  unchanged.
- The endpoint-collision guard (refusing an id owned by a different driver)
  already scans all drivers and extends naturally.
- The success message prints a chat-stage snippet for GGUF:
  `"enhance": { "model": "local-llm:gemma" }`.

## Error handling

- **Startup (validation)**: missing/unreadable GGUF, empty `models` map, or a
  transcribe stage referencing a `llama_cpp` endpoint → error naming the
  endpoint/model, consistent with existing messages.
- **Runtime load failure** (corrupt file, out of memory): propagates through
  the same pipeline error path as whisper — logged, error cue, daemon stays
  up. Failed loads are not cached; the next press retries.
- **Generation failure / missing chat template**: the stage fails with a
  named-model error; the dictation fails cleanly rather than typing degraded
  output.
- **Cancellation**: checked between decode steps — better than whisper's
  uninterruptible cgo call.
- **Download failure**: no partial files, no config edits (existing
  behavior).

## Testing

- `internal/llama`: integration test gated behind `GOSAID_LLAMA_MODEL`
  (path to any small GGUF): a trivial chat round-trips coherently; the
  no-template error path fires. Skipped when unset; run manually on macOS.
- `internal/drivers`: `LlamaCPP` unit tests with a fake loader hook (as
  `whisper_cpp_test.go` does). The extracted `modelCache` tests — lazy load,
  concurrent-acquire race, idle unload, retry after failed load — run once
  and are exercised through both drivers.
- `internal/config`: table tests for the new driver type, shared
  `unload_after_seconds` handling, and transcribe-ref rejection.
- `internal/cli`: extend the httptest-based download suite with a `.gguf`
  case asserting the `llama_cpp` registration shape and the name-derivation
  rules.
- Build verification: `make build` on macOS (Metal) gates the shared-ggml
  link; the five-target release workflow covers Linux/Windows CPU-only legs.

## README

"Local transcription" becomes "Local models," documenting both kinds: the
GGUF download command, the config block, chat-stage hotkey refs, a
recommended-models table (~4B Gemma-class default, ~1B budget option), RAM
budgeting when whisper + chat models are resident together, and
`unload_after_seconds` as the mitigation. The "chat stages need an
OpenAI-compatible endpoint" caveat is removed.
