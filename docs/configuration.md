# Manual configuration

`gosaid setup` covers the common local-only case. Use `gosaid driver` to list,
add, or configure hosted model providers. Edit `config.json` directly when you
want several hotkeys sharing different models, per-hotkey microphones, or
mixed local/hosted pipelines.

```bash
gosaid config
```

opens the file in `$EDITOR`. A complete annotated sample lives at [`../internal/config/config.example.json`](../internal/config/config.example.json).

| Platform | Path |
|---|---|
| macOS | `~/Library/Application Support/gosaid/config.json` |
| Linux | `~/.config/gosaid/config.json` |
| Windows | `%AppData%\gosaid\config.json` |

Restart GoSaid after editing (`brew services restart gosaid`, or Ctrl+C and relaunch) — the config is read at startup.

## `drivers` — where models come from

A list of drivers, each with named endpoints. Models are referenced everywhere else as `<endpoint_id>:<model>`.

- **`whisper_cpp`** — local transcription. Endpoint config is `models`, a map of name → GGML file path. `gosaid setup` writes this as the `speech` endpoint.
- **`llama_cpp`** — local text stages. Same shape, GGUF file paths. `gosaid setup` writes this as the `text` endpoint.
- **`openai_compatible`** — any hosted OpenAI-compatible API. Needs `api_base` and `api_key`. Works with OpenAI, Groq, OpenRouter, DeepSeek, Together, and others — swap the base URL. Add several endpoints to mix providers.

Local endpoints also take `unload_after_seconds`: an idle model is freed after that many seconds and reloads on the next dictation. Omit it (or use `0`) to keep models resident once loaded.

### Managing hosted drivers

The single driver command opens an interactive manager:

```bash
gosaid driver
```

It shows all configured drivers. Select a hosted driver to update its API base
URL or key, delete any configured driver, or choose **Add a new driver** and
pick OpenAI, OpenRouter, or OpenAI-compatible. Deletion asks for confirmation
and warns when a hotkey still references the driver; downloaded local model
files are not removed. The presets fill in the standard endpoint ID and API
base; the cursor goes directly to their API-key field. The next screen asks
for the remote model names, prefilled with provider defaults. A compatible
custom API asks for its endpoint ID, base URL, and transcription/chat model
names directly. At least one model is required.

Adding the driver makes a small live request to each named model: a short
silent audio transcription for STT and a minimal completion for chat. This
checks the API key, model name, endpoint path, and actual capability before the
driver is saved. Configuring an existing remote driver reruns the same checks.
These requests may appear in the provider's usage accounting.

During `gosaid setup`, a single compatible STT or LLM endpoint is selected
automatically, whether it is local or hosted. If more than one endpoint can run
that kind of model, setup shows a model picker. One selected LLM model backs
enhance, translate, and compose for the new hotkey.

The list shows local `whisper_cpp` and `llama_cpp` endpoints too. Their model
registrations continue to be managed by `gosaid setup` and
`gosaid model download`.

```json
"drivers": [
  {
    "driver": "whisper_cpp",
    "endpoints": [{
      "id": "speech",
      "config": {
        "models": { "turbo": "~/Library/Application Support/gosaid/models/ggml-large-v3-turbo-q5_0.bin" },
        "unload_after_seconds": 300
      }
    }]
  },
  {
    "driver": "llama_cpp",
    "endpoints": [{
      "id": "text",
      "config": {
        "models": { "gemma-4-e2b": "~/Library/Application Support/gosaid/models/gemma-4-E2B-it-Q4_0.gguf" }
      }
    }]
  },
  {
    "driver": "openai_compatible",
    "endpoints": [{
      "id": "openai",
      "config": {
        "api_base": "https://api.openai.com/v1",
        "api_key": "sk-...",
        "transcribe_model": "whisper-1",
        "chat_model": "gpt-5.4-nano"
      }
    }]
  }
]
```

Local chat models must ship an embedded chat template (instruct builds do); base models are rejected at load time.

### Downloading models by hand

```bash
gosaid model download ggerganov/whisper.cpp ggml-large-v3-turbo-q5_0.bin --name turbo
gosaid model download ggml-org/gemma-4-E2B-it-GGUF gemma-4-E2B-it-Q4_0.gguf --name gemma-4-e2b
```

Downloaded models are registered in your config automatically, under the `speech` (whisper) or `text` (llama) endpoint.

## `hotkeys` — what each shortcut does

A map of combo → binding. Each combo needs at least one modifier plus one key.

- **Modifiers:** `ctrl`, `shift`, `alt`/`option`, `cmd`/`win`
- **Keys:** `a`–`z`, `0`–`9`, `f1`–`f12`, arrows, `space`, `tab`, `enter`, `esc`

Per binding:

| Field | What it does |
|---|---|
| `mode` | `hold` (record while held) or `toggle` (press to start, press again to stop) |
| `microphone` | Optional per-hotkey input device, matched as a case-insensitive substring of the device name. Overrides the global `microphone`. If the device isn't connected when recording starts, GoSaid falls back to the system default and logs a warning rather than failing the dictation |
| `transcribe` / `enhance` / `compose` / `translate` | The pipeline stages — see below |

Run `gosaid mic` to choose the global default from an interactive list. The current setting is preselected and the system default device is labeled.

### Pipeline stages

A hotkey runs up to three stages in order: `transcribe` → (`enhance` or `compose`) → `translate`. Only `transcribe` is required. Each stage names its own `model`, so different stages can run on different endpoints.

| Stage | What it does |
|---|---|
| `transcribe` | Speech to text. Optional `input_language` (ISO 639-1 hint) and `output_language` |
| `enhance` | Strips "um"s, false starts, and repeats without changing meaning or style |
| `compose` | You describe what you want written and get the finished artifact — a message, note, commit message, snippet, summary, whatever the brief calls for. Audience and register are inferred from the instruction, and the output language matches the language you spoke in. With text selected in the focused app, the brief is applied to that selection instead and the rewrite replaces it, keeping the selection's language (macOS only — see below). Optional `instructions` adds a per-hotkey style directive on top of the defaults |
| `translate` | Renders the result in another language (`output_language`) |

Any optional stage takes `"enable": false` to skip it without deleting the block.

**Editing a selection.** A `compose` hotkey pressed while text is selected reads that selection and treats your speech as an instruction for changing it, replacing the selection with the result. Detection is automatic — no config, no separate hotkey — and falls back to writing from scratch whenever nothing is selected. Reading the selection borrows the clipboard for a moment (the previous contents, text or image, are restored), and needs the same **Accessibility** permission as text injection. Currently macOS-only: on Linux and Windows the synthesized copy keystroke would interrupt a foreground terminal process, so compose there always writes from scratch.

A fully local pipeline — dictate in any language, get clean English typed out:

```json
"hotkeys": {
  "cmd+shift+r": {
    "mode": "hold",
    "transcribe": { "model": "speech:turbo" },
    "enhance":    { "model": "text:gemma-4-e2b" },
    "translate":  { "model": "text:gemma-4-e2b", "output_language": "en" }
  }
}
```

The same pipeline with hosted text stages — local transcription, cloud cleanup:

```json
"hotkeys": {
  "cmd+shift+r": {
    "mode": "hold",
    "transcribe": { "model": "speech:turbo" },
    "enhance":    { "model": "openai:gpt-5.4-nano" },
    "translate":  { "model": "openai:gpt-5.4-nano", "output_language": "en" }
  }
}
```

A compose hotkey — speak the brief, get the finished text — with a style directive applied to everything it writes:

```json
"cmd+shift+e": {
  "mode": "hold",
  "transcribe": { "model": "speech:turbo" },
  "compose": {
    "model": "text:gemma-4-e2b",
    "instructions": "Write in a formal, business-email register."
  }
}
```

Mixing local and hosted stages within one hotkey is fine, and different hotkeys can use entirely different providers.

## Global settings

| Field | What it does |
|---|---|
| `microphone` | Default input device for every hotkey (substring match). Empty = system default |
| `user_context` | Free-form personal context (name, role, tone, email signature) fed to the `compose` stage. Any language |
| `toggle_max_seconds` | Safety cap on a `toggle` recording. Default `60` |
| `injection_mode` | How text reaches the app under your cursor. `paste` is currently the only supported value |
| `sound_feedback` | Play start/stop cues when recording |
| `log_level` | `debug`, `info`, `warn`, or `error`. Default `info`; unrecognized values fall back to it |
| `version` | Config schema version — currently `2` |

## Personal dictionary — custom vocabulary

Names, product names, and jargon that transcription keeps mangling can be added to a personal dictionary. The words are fed to Whisper as a transcription hint and injected into every text stage's prompt (`enhance`, `compose`, `translate`, and selection rewrites), so custom terms are spelled the way you want.

```bash
gosaid vocab Kubernetes        # add a word
gosaid vocab "New York"        # multi-word terms are fine (quote them)
gosaid vocab Kubernetes --delete   # remove a word
gosaid vocab                   # list the current words
```

Words are stored in `vocabulary.json` next to `config.json` — a simple list you can also edit by hand:

```json
{
  "words": ["Kubernetes", "PostHog", "gosaid"]
}
```

| Platform | Path |
|---|---|
| macOS | `~/Library/Application Support/gosaid/vocabulary.json` |
| Linux | `~/.config/gosaid/vocabulary.json` |
| Windows | `%AppData%\gosaid\vocabulary.json` |

Matching is case-insensitive for add/remove, so a term is only stored once. Restart GoSaid after editing the file by hand — like `config.json`, the vocabulary is read at startup. (`gosaid vocab` writes the file directly; the change still takes effect on the daemon's next start.)

## Memory and performance

On macOS, local inference runs on the GPU via Metal.

At startup, GoSaid loads the local transcription models and local LLMs referenced by enabled stages in configured hotkeys. Shared model references are loaded once, while unused models registered on an endpoint remain unloaded. This avoids model-loading latency on the first dictation.

By default (`unload_after_seconds` omitted or `0`), loaded models stay resident at their full weight size. Budget RAM for every active model — e.g. whisper `turbo` (~550 MB) plus `gemma-4-e2b` (~2.8 GB) ≈ 3.4 GB. Mixing a small enhance model with a larger compose model is fine, but each loads separately.

Set `unload_after_seconds` on a local endpoint to trade a few seconds of reload latency for that memory.
