# GoSaid

**Dictate in your native language, get polished text in another one.** Press a hotkey, speak, and GoSaid transcribes your speech, cleans it up, translates it — and types the result straight into the app under your cursor.

- **Local or cloud** — run Whisper fully on-device via embedded whisper.cpp (no API key, no network), or use any OpenAI-compatible API (OpenAI, Groq, OpenRouter, DeepSeek, Together, …).
- **Lightweight** — a single static binary. No UI, no bundled runtimes, just one small background process.

> **Platform status:** actively used and tested on macOS. Linux and Windows builds are produced but **not yet tested** — expect rough edges and please report issues.

## Installation

### macOS & Linux (Homebrew)

```
brew install dmtrkzntsv/tap/gosaid
gosaid config                  # paste your API key, save
brew services start gosaid     # runs in background, auto-starts at login
```

Upgrade with `brew upgrade gosaid`, stop with `brew services stop gosaid`.

- **macOS:** grant **Accessibility** (prompted on first hotkey press) and **Microphone** (first recording).
- **Linux:** install a keystroke-injection tool: `wtype` (Wayland), `xdotool` (X11), or `ydotool` (either, needs its daemon running).

### Windows

1. Download `gosaid-<version>-windows-amd64.zip` from [releases](https://github.com/dmtrkzntsv/gosaid/releases/latest), extract, and put `gosaid.exe` on your `PATH`.
2. SmartScreen will warn on first run (the binary is unsigned in v1) — click **More info → Run anyway**.
3. Run `gosaid config`, then `gosaid`.

> Prefer a raw binary or building from source? See [Manual installation](#manual-installation).

## Configuration

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

### Local transcription (no cloud)

Transcription can run fully locally via embedded whisper.cpp — no API key, no network. Download a GGML model and it's registered in your config automatically as the `local` endpoint:

```
gosaid model download ggerganov/whisper.cpp ggml-large-v3-turbo-q5_0.bin --name turbo
```

Then use it in a hotkey: `"transcribe": { "model": "local:turbo" }`.

| Model | Size | When to use |
|---|---|---|
| `ggml-large-v3-turbo-q5_0.bin` | ~550 MB | Best accuracy/latency balance — the default choice, especially for non-English speech |
| `ggml-small.bin` | ~460 MB | Balanced multilingual fallback, fast on plain CPU |
| `ggml-base.bin` | ~140 MB | Near-instant; fine for quick English notes |

On macOS inference runs on the GPU (Metal). Local models cover **transcription only** — `enhance`, `compose`, and `translate` need an OpenAI-compatible endpoint (cloud, or a local server like Ollama).

### Hotkeys

Each hotkey binds a combo (at least one modifier + one key, e.g. `ctrl+alt+space`, `cmd+shift+r`) to a recording mode:

- `hold` — record while held, release to stop.
- `toggle` — press to start, press again to stop.

Modifiers: `ctrl`, `shift`, `alt`/`option`, `cmd`/`win`. Keys: `a`–`z`, `0`–`9`, `f1`–`f12`, arrows, `space`, `tab`, `enter`, `esc`.

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
  "transcribe": { "model": "local:turbo" },
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
