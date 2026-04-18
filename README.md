# gosaid

Headless, cross-platform push-to-talk voice dictation daemon. Press a hotkey, speak, release — text appears at the cursor.

Cloud-only transcription via any OpenAI-compatible API: Groq, OpenAI, OpenRouter, Together, DeepSeek, LocalAI, Ollama.

## Features

- **Push-to-talk** hotkeys with hold and toggle modes.
- **Optional pipeline stages** per hotkey: transcribe → translate → enhance.
- **Per-language vocabulary** hints (preserved term list sent to Whisper and the LLMs).
- **Per-language find-and-replace** rules applied to final output (e.g. `"new line"` → `\n`).
- **Audio feedback** — distinct start/stop/error cues.
- **No UI, no tray, no IPC.** Edit JSON in `$EDITOR`, restart to apply.
- **Clipboard fallback** — if paste synthesis fails, text stays in the clipboard for manual `Cmd/Ctrl+V`.

## Install

### Recommended: Homebrew (macOS & Linux)

```
brew install dmtrkzntsv/tap/gosaid
gosaid config                  # paste your API key, save
brew services start gosaid     # runs in background, auto-starts at login
```

Upgrade later with `brew upgrade gosaid`. Stop with `brew services stop gosaid`.

**macOS:** the binary is signed and notarized — no Gatekeeper warnings. On first hotkey press macOS prompts for **Accessibility**; on first record it prompts for **Microphone**. Grant both in System Settings → Privacy & Security.

**Linux:** install a keystroke-injection tool (`wtype` for Wayland, `xdotool` for X11, `ydotool` for either): `sudo apt install wtype` or `sudo pacman -S wtype`.

### Manual download (any OS, any arch)

Grab a prebuilt binary from [releases](https://github.com/dmtrkzntsv/gosaid/releases/latest).

#### macOS (arm64 / amd64)
```
tar -xzf gosaid-<version>-darwin-arm64.tar.gz   # or -amd64
sudo mv gosaid-<version>-darwin-arm64/gosaid /usr/local/bin/
gosaid version                 # signed + notarized → no Gatekeeper warning
gosaid config
gosaid                         # foreground; Ctrl+C to stop
```

To run in the background and auto-start at login:
```
mkdir -p ~/Library/LaunchAgents
curl -fsSL https://raw.githubusercontent.com/dmtrkzntsv/gosaid/main/examples/service/dev.gosaid.plist \
  -o ~/Library/LaunchAgents/dev.gosaid.plist
launchctl load -w ~/Library/LaunchAgents/dev.gosaid.plist
```
Stop: `launchctl unload ~/Library/LaunchAgents/dev.gosaid.plist`. Logs: `tail -f /tmp/gosaid.err`.

#### Linux (amd64 / arm64)
```
tar -xzf gosaid-<version>-linux-amd64.tar.gz
sudo mv gosaid-<version>-linux-amd64/gosaid /usr/local/bin/
sudo apt install wtype         # or xdotool / ydotool
gosaid config
gosaid                         # foreground
```

Background + autostart (systemd user unit):
```
mkdir -p ~/.config/systemd/user
curl -fsSL https://raw.githubusercontent.com/dmtrkzntsv/gosaid/main/examples/service/gosaid.service \
  -o ~/.config/systemd/user/gosaid.service
systemctl --user daemon-reload
systemctl --user enable --now gosaid
loginctl enable-linger "$USER"   # optional: keep running when logged out
```
Status: `systemctl --user status gosaid`. Logs: `journalctl --user -u gosaid -f`.

#### Windows (amd64)
1. Extract `gosaid-<version>-windows-amd64.zip`.
2. Move `gosaid.exe` to a folder on your `PATH` (e.g. `C:\Users\<you>\bin\`, then add it via System Properties → Environment Variables).
3. SmartScreen will warn "Windows protected your PC" on first run — click **More info → Run anyway**. (The Windows binary is unsigned in v1.)
4. Configure + foreground: `gosaid config` then `gosaid`.

Background + autostart:
1. Press `Win+R`, type `shell:startup`, press Enter.
2. Right-click → **New → Shortcut**.
3. Location: `cmd.exe /c start "" /min "C:\Users\<you>\bin\gosaid.exe"`.
4. Name it **gosaid**. It will start every login. Delete the shortcut to disable.

To stop: `Get-Process gosaid | Stop-Process`.

### Verify the download (optional)
```
shasum -a 256 gosaid-<version>-*.tar.gz                          # macOS/Linux
certutil -hashfile gosaid-<version>-windows-amd64.zip SHA256     # Windows
```
Compare to `checksums.txt` in the release.

### From source (Go 1.25+)
```
git clone https://github.com/dmtrkzntsv/gosaid
cd gosaid
make build
./gosaid version
```

## First run

1. `./gosaid config` — opens the config file in `$EDITOR` (falls back to `$VISUAL`, then the OS default).
2. Replace `"api_key": ""` with a real key from [Groq](https://console.groq.com/) (free, fast) or OpenAI.
3. `./gosaid` — starts the daemon. You'll see `registered ctrl+alt+space (hold)` and `gosaid running`.
4. Open any text editor, hold Ctrl+Alt+Space, say "hello world", release. Text appears.

## Commands

- `gosaid` — run the daemon (foreground).
- `gosaid config` — edit config in `$EDITOR`.
- `gosaid version` — print version.
- `gosaid help` — usage.
- `gosaid --debug <cmd>` — dev helpers (record-test, play-test, transcribe, translate-speech, chat).

## Config

Platform paths (resolved via `os.UserConfigDir()`):

| Platform | Path |
|---|---|
| macOS | `~/Library/Application Support/gosaid/config.json` |
| Linux | `$XDG_CONFIG_HOME/gosaid/config.json` or `~/.config/gosaid/config.json` |
| Windows | `%AppData%\gosaid\config.json` |

See [`internal/config/config.example.json`](internal/config/config.example.json) for a complete annotated sample.

### Field reference

- `drivers[].driver` — `"openai_compatible"` (only supported type).
- `drivers[].endpoints[].id` — unique, referenced by model strings.
- `drivers[].endpoints[].config.api_base` — base URL, no trailing `/chat/completions` etc.
- `drivers[].endpoints[].config.api_key` — **stored literally**. Protect the config file permissions.
- `vocabulary.<lang>` — list of terms fed to Whisper via `initial_prompt` and injected into translate/enhance templates.
- `replacements.<lang>` — case-insensitive whole-word find/replace applied to final output.
- `hotkeys.<combo>.mode` — `"hold"` (default) or `"toggle"`.
- `hotkeys.<combo>.transcribe.model` — required, `<endpoint_id>:<model>`.
- `hotkeys.<combo>.transcribe.input_language` — optional ISO 639-1 code; if set, passed to Whisper as a language hint for better accuracy. Omit for auto-detect.
- `hotkeys.<combo>.transcribe.output_language` — `"en"` triggers Whisper's native English-translate fast path.
- `hotkeys.<combo>.translate.output_language` — any ISO 639-1 code.
- `hotkeys.<combo>.translate.prompt` — optional user instruction.
- `hotkeys.<combo>.enhance.prompt` — required user instruction (e.g. "format as email").
- `toggle_max_seconds` — hard cap on toggle-mode recordings.
- `injection_mode` — `"paste"` (only option in v1).
- `sound_feedback` — enable/disable audio cues.
- `log_level` — `debug`, `info`, `warn`, `error`.

### Hotkey combos

Modifiers: `ctrl`, `alt`, `shift`, `cmd` (also `super`, `win` on Windows).
Keys: `a`-`z`, `0`-`9`, `space`, `tab`, `enter`, `esc`, `f1`-`f12`, `left`, `right`, `up`, `down`.

Examples: `ctrl+alt+space`, `cmd+shift+r`, `ctrl+alt+f1`.

## Platform notes

### macOS
On first hotkey press, macOS will prompt for **Accessibility** permission (needed for global hotkeys and paste synthesis). If paste fails silently, go to **System Settings → Privacy & Security → Accessibility** and enable `gosaid` (or the terminal running it).

Microphone permission is prompted separately on first record.

### Linux
Wayland compositors don't let apps synthesize keystrokes without help. Install one of:

- `wtype` (Wayland, native — preferred)
- `ydotool` (any, requires a running daemon + uinput group)
- `xdotool` (X11 only)

gosaid tries them in that order. Global hotkeys currently use X11 via the `golang.design/x/hotkey` library — Wayland hotkey support depends on your compositor.

### Windows
Works out of the box — SendInput needs no extra tools.

## Security

API keys are stored in plain JSON. The config file is created with standard user permissions (`0644`). If you're on a shared machine, chmod it:
```
chmod 600 "$HOME/Library/Application Support/gosaid/config.json"
```

## Running as a service

See [`examples/service/`](examples/service/) for a launchd plist (macOS) and systemd user unit (Linux).

## Troubleshooting

**No text appears, no error.** Check microphone permissions and `log_level: "debug"`. Make sure the API key is valid.

**"paste failed" warning.** Grant Accessibility (macOS) or install a keystroke tool (Linux). Text is still in your clipboard — paste manually.

**"another gosaid instance is already running"** — remove the stale PID file or kill the existing process:
```
cat "$HOME/Library/Caches/gosaid/gosaid.pid"    # macOS/Linux
```

**Transcription sounds wrong.** Add domain terms to `vocabulary.<lang>`; set `languages.transcription` to a single language; try `whisper-large-v3` over smaller models.

## License

MIT (see LICENSE).
