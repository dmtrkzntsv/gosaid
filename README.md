# GoSaid

Headless, cross-platform push-to-talk voice dictation daemon. Press a hotkey, speak, release — text appears at the cursor.

**Dictate in your native language, get text in another one.** Set up a hotkey that transcribes your speech and translates it on the fly — so you can speak your language and insert English (or any other) straight into the app under your cursor.

Designed to stay **lightweight with as few dependencies as possible**: transcription and translation run against any OpenAI-compatible cloud API (OpenAI, Groq, OpenRouter, Together, DeepSeek, and similar), so the daemon itself is a small static binary that ships as a single file, starts instantly, and idles with a negligible footprint — no bundled models, no Electron, no background services beyond the one process.

## Install

### macOS & Linux (Homebrew)

```
brew install dmtrkzntsv/tap/gosaid
gosaid config                  # paste your API key, save
brew services start gosaid     # runs in background, auto-starts at login
```

Upgrade with `brew upgrade gosaid`. Stop with `brew services stop gosaid`.

### Windows

1. Download and extract `gosaid-<version>-windows-amd64.zip` from [releases](https://github.com/dmtrkzntsv/gosaid/releases/latest).
2. Move `gosaid.exe` to a folder on your `PATH` (e.g. `C:\Users\<you>\bin\`, then add it via System Properties → Environment Variables).
3. SmartScreen will warn "Windows protected your PC" on first run — click **More info → Run anyway**. (The Windows binary is unsigned in v1.)
4. Configure and run: `gosaid config` then `gosaid`.

> Prefer a raw binary on macOS/Linux, or want to build from source? See [Manual installation](#manual-installation) at the bottom.

## Configuration

Config is a single JSON file. Run `gosaid config` to open it in `$EDITOR`, or edit directly:

| Platform | Path |
|---|---|
| macOS | `~/Library/Application Support/gosaid/config.json` |
| Linux | `$XDG_CONFIG_HOME/gosaid/config.json` (or `~/.config/gosaid/config.json`) |
| Windows | `%AppData%\gosaid\config.json` |

A complete annotated sample lives at [`internal/config/config.example.json`](internal/config/config.example.json).

### Provider

Declare one or more API endpoints. Each endpoint has an `id` you reference later in hotkey `model` strings as `<endpoint_id>:<model>`.

```json
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
]
```

Any OpenAI-compatible cloud API works — swap `api_base` and `api_key` for Groq, OpenRouter, DeepSeek, Together, etc. Add more endpoints to the same `endpoints` array to mix providers (e.g. Groq for transcription, OpenAI for enhancement).

### Hotkey

Bind a key combo to a recording mode and one or more pipeline stages:

```json
"hotkeys": {
  "ctrl+alt+space": {
    "mode": "hold",
    "transcribe": { "model": "openai:whisper-1" }
  }
}
```

**Modes:**
- `hold` — record while the combo is held; release to stop.
- `toggle` — press once to start, press again to stop. Capped by the top-level `toggle_max_seconds`.

**Combo syntax** (case-insensitive, joined by `+`, at least one modifier + one key):

- Modifiers: `ctrl` (alias `control`), `shift`, `alt` (macOS alias `option`), `cmd` (aliases `command`, `super`; Windows alias `win`).
- Keys: `a`–`z`, `0`–`9`, `f1`–`f12`, `left`, `right`, `up`, `down`, `space`, `tab`, `enter`, `esc`.
- Examples: `ctrl+alt+space`, `cmd+shift+r`, `ctrl+alt+f1`.

### Modes

A hotkey runs up to three stages in order. `transcribe` is required; `translate` and `enhance` are optional.

**Transcribe** — speech to text.

```json
"transcribe": {
  "model": "openai:whisper-1",
  "input_language": "en",
  "output_language": "en"
}
```

`input_language` is an optional ISO 639-1 hint for Whisper. `output_language: "en"` activates Whisper's native English fast-path; for other targets, chain a `translate` stage.

**Translate** — render the transcript in another language via an LLM. Prompt is optional; when omitted, translations stick to the source's exact wording and style.

```json
"translate": {
  "output_language": "fr",
  "model": "openai:gpt-5.4-nano",
  "prompt": "Use formal register."
}
```

**Enhance** — reshape the text with an LLM (format as email, clean up filler words, etc.). Prompt is optional; when omitted, the model only strips speech disfluencies ("um", "uh", "mm", false starts, repeats) without changing meaning or style.

```json
"enhance": {
  "prompt": "Format as a professional email with greeting and sign-off.",
  "model": "openai:gpt-5.4-nano"
}
```

## Manual installation

Prebuilt binaries for all platforms are on the [releases page](https://github.com/dmtrkzntsv/gosaid/releases/latest).

### macOS (arm64 / amd64)

```
tar -xzf gosaid-<version>-darwin-arm64.tar.gz   # or -amd64
sudo mv gosaid-<version>-darwin-arm64/gosaid /usr/local/bin/
gosaid config
gosaid                         # foreground; Ctrl+C to stop
```

The macOS binary is signed and notarized — no Gatekeeper warning. First hotkey press prompts for **Accessibility**; first record prompts for **Microphone**.

### Linux (amd64 / arm64)

```
tar -xzf gosaid-<version>-linux-amd64.tar.gz
sudo mv gosaid-<version>-linux-amd64/gosaid /usr/local/bin/
sudo apt install wtype         # or xdotool / ydotool
gosaid config
gosaid                         # foreground
```

A keystroke-injection tool is required: `wtype` (Wayland), `xdotool` (X11), or `ydotool` (either, needs a running daemon).

### Windows (amd64)

Same as the [Install → Windows](#windows) section above.

### From source (Go 1.25+)

```
git clone https://github.com/dmtrkzntsv/gosaid
cd gosaid
make build
./gosaid version
```

## License

MIT (see LICENSE).
