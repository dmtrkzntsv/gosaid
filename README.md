# GoSaid

**Dictate in your native language, get polished text in another one.** Press a hotkey, speak, and GoSaid types the result straight into the app under your cursor — cleaned up, translated into another language, or written for you from a spoken brief — describe what you need and get the finished text. It's a headless background process with no UI, and it runs entirely on your machine for free: no API key, no account, nothing you say leaving your laptop. Hosted models work too, if you'd rather.

> **Platform status:** actively used and tested on macOS. Linux and Windows builds are produced but **not yet tested** — expect rough edges and please report issues.

## Install

### macOS & Linux (Homebrew)

```bash
brew install dmtrkzntsv/tap/gosaid
```

Then run the wizard and start it in the background:

```bash
gosaid setup && brew services start gosaid
```

- **macOS:** grant **Accessibility** (prompted on first hotkey press) and **Microphone** (first recording). The binary is signed and notarized.
- **Linux:** install a keystroke-injection tool — `wtype` (Wayland), `xdotool` (X11), or `ydotool` (either, needs its daemon running).

### Windows

1. Download `gosaid-<version>-windows-amd64.zip` from the [releases page](https://github.com/dmtrkzntsv/gosaid/releases/latest), extract it, and put `gosaid.exe` on your `PATH`.
2. SmartScreen warns on first run (the binary is unsigned in v1) — click **More info → Run anyway**.
3. Run `gosaid setup`, then `gosaid`.

## Quick start — `gosaid setup`

```bash
gosaid setup
```

### Stages

The wizard asks which stages a hotkey should run. Each is an independent yes/no, and they always run in the order below. Different hotkeys can enable different stages — that's how you end up with one shortcut for quick notes and another that writes your emails.

**`transcribe`** — speech to text. Always on; it's the only required stage. A hotkey with nothing else enabled gives you the raw transcript, which is fast and accurate but reads like speech: no capitals, stray "um"s, and whatever the recognizer made of your accent.

**`enhance`** — fixes exactly that. It removes filler words, false starts, and repeated words, keeps only the final version when you correct yourself mid-sentence, and adds the capitalization and punctuation that speech doesn't carry. It deliberately doesn't touch your phrasing, your language, or the English technical terms you drop into another language — the point is text you'd have typed, not text someone else wrote. Enable it on any hotkey you dictate real sentences into.

**`compose`** — for when you don't want to dictate the text at all, just say what it should be. *"A short Slack reply saying I'll miss standup"*, *"a commit message for a fix to the retry logic"*, *"three bullet points summarising this idea"* — what you speak is the brief, and what gets typed is the finished thing. Register and audience are inferred from how you phrase the request, and you can pin a house style per hotkey (*"always write in a formal register"*) so you don't have to repeat it every time.

A compose hotkey also edits text you already have: select something in any app, hold the hotkey, and say how it should change — *"make it shorter"*, *"more formal"*, *"turn this into a bullet list"* — and the rewritten version replaces your selection in place. It stays in the language of the selected text however you phrase the request, unless you ask for a translation. With nothing selected the same hotkey writes from scratch as above, so there's no separate mode to remember. Selection capture is macOS-only for now, and it works by briefly borrowing the clipboard to read your selection; whatever you had copied is put back afterwards.

**`translate`** — renders the result in a language you pick when you set the hotkey up. This is the stage that lets you think and speak in your native language and have polished text in another one appear in the app you're typing into, which is the thing GoSaid is really for. It translates closely rather than rewriting, so an enhanced transcript stays yours.

The one choice worth thinking about is `enhance` vs `compose`: enhance keeps what you said and cleans it up, compose throws away your wording and writes something new from it. Most hotkeys want enhance (plus translate); a "write this for me" hotkey wants compose.

## Manual configuration

The wizard is local-only and does one hotkey at a time. Everything else — cloud providers, several hotkeys with different models, per-hotkey microphones, mixed local/hosted pipelines, memory tuning — is a direct edit:

```bash
gosaid config
```

Custom vocabulary — names, products, jargon that transcription keeps getting wrong — goes in a personal dictionary that hints both Whisper and the text stages:

```bash
gosaid vocab Kubernetes          # add a word (--delete to remove, no args to list)
```

See **[docs/configuration.md](docs/configuration.md)** for the full reference.

## License

MIT (see [LICENSE](LICENSE)).
