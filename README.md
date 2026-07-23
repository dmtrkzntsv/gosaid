# GoSaid

**Dictate in your native language, get polished text in another one.** Press a hotkey, speak, and GoSaid transcribes your speech, cleans it up, translates it — and types the result straight into the app under your cursor.

- **Local or cloud** — run transcription and chat stages fully on-device via embedded whisper.cpp and llama.cpp (no API key, no network), or use any OpenAI-compatible API (OpenAI, Groq, OpenRouter, DeepSeek, Together, …).
- **Lightweight** — a single static binary. No UI, no bundled runtimes, just one small background process.

> **Platform status:** actively used and tested on macOS. Linux and Windows builds are produced but **not yet tested** — expect rough edges and please report issues.

## Installation

### macOS & Linux (Homebrew)

```
brew install dmtrkzntsv/tap/gosaid
gosaid setup                   # guided setup: transcription model, hotkey, optional chat stages
brew services start gosaid     # runs in background, auto-starts at login
```

Upgrade with `brew upgrade gosaid`, stop with `brew services stop gosaid`.

- **macOS:** grant **Accessibility** (prompted on first hotkey press) and **Microphone** (first recording).
- **Linux:** install a keystroke-injection tool: `wtype` (Wayland), `xdotool` (X11), or `ydotool` (either, needs its daemon running).

### Windows

1. Download `gosaid-<version>-windows-amd64.zip` from [releases](https://github.com/dmtrkzntsv/gosaid/releases/latest), extract, and put `gosaid.exe` on your `PATH`.
2. SmartScreen will warn on first run (the binary is unsigned in v1) — click **More info → Run anyway**.
3. Run `gosaid setup`, then `gosaid`.

> Prefer a raw binary or building from source? See [Manual installation](#manual-installation).

## Configuration

The easiest way to configure GoSaid is the interactive setup:

```
gosaid setup             # guided wizard: transcription model, hotkey, optional chat stages
```

On a fresh install `gosaid setup` walks you through picking a microphone, a transcription model, a hotkey, and optional local chat stages (enhance, translate, compose). For cloud providers, edit `config.json` directly. Prefer raw JSON?
`gosaid config` still opens the file in `$EDITOR`.

Config is a single JSON file — run `gosaid config` to open it in `$EDITOR`. A complete annotated sample lives at [`internal/config/config.example.json`](internal/config/config.example.json).

| Platform | Path |
|---|---|
| macOS | `~/Library/Application Support/gosaid/config.json` |
| Linux | `~/.config/gosaid/config.json` |
| Windows | `%AppData%\gosaid\config.json` |

A minimal working config — one provider, one push-to-talk hotkey:

```json
{
  "drivers": [
    {
      "driver": "openai_compatible",
      "endpoints": [
        {
          "id": "openai",
          "config": {
            "api_base": "https://api.openai.com/v1",
            "api_key": "sk-..."
          }
        }
      ]
    }
  ],
  "hotkeys": {
    "ctrl+alt+space": {
      "mode": "hold",
      "transcribe": { "model": "openai:whisper-1" }
    }
  }
}
```

Any OpenAI-compatible API works — swap `api_base`/`api_key` for Groq, OpenRouter, DeepSeek, etc. Add more endpoints to mix providers; models are referenced as `<endpoint_id>:<model>`.

### Local models (no cloud)

Transcription can run fully locally via embedded whisper.cpp — no API key, no network. Download a GGML model and it's registered in your config automatically as the `speech` endpoint:

```
gosaid model download ggerganov/whisper.cpp ggml-large-v3-turbo-q5_0.bin --name turbo
```

Then use it in a hotkey: `"transcribe": { "model": "speech:turbo" }`.

| Model | Size | When to use |
|---|---|---|
| `ggml-large-v3-turbo-q5_0.bin` | ~550 MB | Best accuracy/latency balance — the default choice, especially for non-English speech |
| `ggml-small.bin` | ~460 MB | Balanced multilingual fallback, fast on plain CPU |
| `ggml-base.bin` | ~140 MB | Near-instant; fine for quick English notes |

On macOS inference runs on the GPU (Metal).

Loaded models stay resident in memory for instant dictation (the model's full weight size, e.g. ~1.7 GB for un-quantized `large-v3-turbo`). To trade a few seconds of reload latency for that memory, set `unload_after_seconds` in the endpoint's config — an idle model is freed after that long and reloads on the next dictation:

```json
"config": {
  "models": { "turbo": "/path/to/models/ggml-large-v3-turbo-q5_0.bin" },
  "unload_after_seconds": 300
}
```

#### Local chat models (enhance / compose / translate)

The text stages can also run fully locally via embedded llama.cpp. Download
any instruct-tuned GGUF model — it registers as the `text` endpoint:

```
gosaid model download ggml-org/gemma-3-4b-it-GGUF gemma-3-4b-it-Q4_K_M.gguf --name gemma
```

Then use it in a hotkey's chat stages:

```json
"cmd+shift+r": {
  "mode": "hold",
  "transcribe": { "model": "speech:turbo" },
  "enhance":    { "model": "text:gemma" },
  "translate":  { "model": "text:gemma", "output_language": "en" }
}
```

| Model | Size | When to use |
|---|---|---|
| `gemma-3-4b-it-Q4_K_M.gguf` (repo `ggml-org/gemma-3-4b-it-GGUF`) | ~2.5 GB | Recommended default — strong cleanup and translation quality |
| `Qwen3-1.7B-Q4_K_M.gguf` (repo `ggml-org/Qwen3-1.7B-GGUF`) | ~1.1 GB | Budget option: less RAM, faster loads, weaker compose quality |

Chat models follow the same residency rules as whisper models: loaded
lazily on first use, kept in memory for instant dictation, and — with
`unload_after_seconds` set on the endpoint — freed after idling. Budget
RAM for every model that can be resident at once (e.g. whisper turbo
~550 MB + a 4B chat model ~2.5 GB); mixing a small enhance model with a
larger compose model is fine, but each loads separately.

Models must ship an embedded chat template (instruct builds do); base
models are rejected at load time.

### Hotkeys

Each hotkey binds a combo (at least one modifier + one key, e.g. `ctrl+alt+space`, `cmd+shift+r`) to a recording mode:

- `hold` — record while held, release to stop.
- `toggle` — press to start, press again to stop.

Modifiers: `ctrl`, `shift`, `alt`/`option`, `cmd`/`win`. Keys: `a`–`z`, `0`–`9`, `f1`–`f12`, arrows, `space`, `tab`, `enter`, `esc`.

#### Picking a microphone

`gosaid setup` asks for the global default microphone on a fresh setup (or when you start from scratch), writing the `microphone` field at the top level of `config.json` — it's used by every hotkey unless overridden. Adding a shortcut later reuses that device rather than asking again; change it by editing `config.json`. By default recording uses the system default input. To override the default for a specific hotkey, edit the hotkey's `microphone` field — use a case-insensitive substring of the device name:

```json
"ctrl+alt+space": {
  "mode": "hold",
  "microphone": "usb pnp",
  "transcribe": { "model": "speech:turbo" }
}
```

If the device isn't connected when recording starts, GoSaid falls back to the system default (and logs a warning) rather than failing the dictation.

### Pipeline stages

A hotkey runs up to three stages in order: `transcribe` → (`enhance` or `compose`) → `translate`. Only `transcribe` is required.

| Stage | What it does |
|---|---|
| `transcribe` | Speech to text. Optional `input_language` (ISO 639-1 hint) and `output_language` |
| `enhance` | Strips "um"s, false starts, and repeats without changing meaning or style |
| `compose` | Treats your speech as an instruction and writes the artifact — *"write a polite email to Alice asking to reschedule to Thursday"*. Optional `instructions` field adds a per-hotkey style directive |
| `translate` | Renders the result in another language (`output_language`) |

A full pipeline — dictate in any language, get clean English typed out:

```json
"cmd+shift+r": {
  "mode": "hold",
  "transcribe": { "model": "speech:turbo" },
  "enhance":    { "model": "openai:gpt-5.4-nano" },
  "translate":  { "model": "openai:gpt-5.4-nano", "output_language": "en" }
}
```

Handy extras: any optional stage takes `"enable": false` to skip it without deleting the block, and the top-level `user_context` field gives `compose` personal context (name, role, tone, email signature).

## Manual installation

Prebuilt binaries for all platforms are on the [releases page](https://github.com/dmtrkzntsv/gosaid/releases/latest).

### macOS / Linux

```
tar -xzf gosaid-<version>-<os>-<arch>.tar.gz
sudo mv gosaid-<version>-<os>-<arch>/gosaid /usr/local/bin/
gosaid config
gosaid                         # foreground; Ctrl+C to stop
```

The macOS binary is signed and notarized — no Gatekeeper warning. If you run an unsigned build from source, grant Microphone and Accessibility to your **terminal app** (Terminal/iTerm/Ghostty) in System Settings → Privacy & Security.

### From source (Go 1.25+)

```
git clone https://github.com/dmtrkzntsv/gosaid
cd gosaid
make build
./gosaid version
```

## License

MIT (see [LICENSE](LICENSE)).
