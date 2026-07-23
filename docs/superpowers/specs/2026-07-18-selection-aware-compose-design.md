# Selection-Aware Compose

**Date:** 2026-07-18
**Status:** Approved

## Summary

The `compose` stage becomes selection-aware. When a compose-enabled hotkey
fires while text is selected in the focused app, GoSaid captures that
selection and treats the dictated speech as an instruction for rewriting it
("make this more formal", "shorten this", "translate this to German"). The
rewritten text replaces the selection. With no selection, compose behaves
exactly as today: the dictated speech is an instruction for generating new
text, injected at the cursor.

No config changes. Existing compose hotkeys gain the behavior automatically.

## User experience

1. Select text anywhere. Press and hold the compose hotkey.
2. Dictate what to change: "make it half as long", "спокойнее тоном".
3. Release. The selection is replaced in place by the rewritten text.

- The rewritten text stays in the **selection's language**, regardless of the
  language the instruction was spoken in — unless the instruction itself asks
  for a translation.
- With no selection, the same hotkey composes fresh text as before. No mode
  switch, no error: absence of a selection selects the compose path.

## Architecture

### Selection capture (`internal/inject`)

A new `GetSelection(ctx) (text string, ok bool, err error)` lives in the
`inject` package — it is the mirror image of paste and shares the clipboard
and keystroke-synthesis machinery.

Mechanism:

1. Save current clipboard contents — text (`clipboard.FmtText`) and, if text
   is absent, image (`clipboard.FmtImage`), so an image on the clipboard
   survives the round-trip.
2. Clear/write a sentinel value to the clipboard.
3. Synthesize Cmd+C — macOS only, via Quartz `CGEvent` (`kVK_ANSI_C` with the
   Command flag set explicitly, so the physically-held hotkey modifiers do
   not leak into the copy keystroke). On Linux/Windows, `GetSelection`
   reports "no selection" without touching the clipboard: a synthesized
   Ctrl+C would interrupt the foreground process when a terminal has focus
   (SIGINT on Linux, CTRL_C_EVENT on Windows).
4. Poll the clipboard for up to ~300 ms for a change. Changed and non-empty →
   selection captured (`ok=true`). Unchanged or empty → no selection
   (`ok=false`, not an error).
5. Restore the saved clipboard contents unconditionally.

Detection is a strong heuristic, not a guarantee: an app that ignores Cmd+C
with no selection is indistinguishable from a broken copy. Both resolve to
`ok=false` → compose path, which is the safe fallback.

### Pipeline integration (`internal/daemon`)

- **On hotkey press** (compose-enabled hotkeys only): start audio capture as
  today and run `GetSelection` **concurrently**. The detection window
  overlaps with the user starting to speak — no added latency.
- **On release:** transcribe as today. In the compose stage:
  - Selection captured → render the new **transform prompt** (see below) and
    call the compose model with the transcript as the user message.
  - No selection → render the existing compose prompt. Unchanged behavior.
- **Injection:** existing paste path, untouched. When a selection existed it
  is still highlighted in the target app, so the synthesized paste replaces
  it; otherwise it inserts at the cursor.
- **Translate stage:** unchanged — if configured, it runs after compose in
  both paths.
- The pipeline awaits the concurrent capture result before entering the
  compose stage; a release before capture completes waits those few hundred
  milliseconds.

Selection capture is exposed to the pipeline through a small interface
(pattern: `captureStopper`), so tests fake it without cgo.

### Prompt (`internal/daemon/prompts.go`)

New `transform` template alongside compose/enhance/translate, rendered via
`RenderTransform(TransformData{...})`. The system prompt carries:

- the selected text,
- `user_context` from config,
- the hotkey's compose `instructions`,
- rules: rewrite the selection according to the user's instruction; output
  **only** the rewritten text (no preamble, no quotes); keep the selection's
  language unless the instruction explicitly requests another.

The dictated transcript is the user message — same `drv.Chat(ctx, model,
system, input)` shape as every other stage.

## Config

None. Selection-awareness is inherent to `compose`. An opt-out flag can be
added later if a real need appears.

## Error handling

| Condition | Behavior |
|---|---|
| Copy synthesis fails (no Accessibility permission, missing Linux keystroke tool) | Hard error at press: error cue, recording stops, no run. If Cmd+C cannot be synthesized, the later paste would fail too; silently composing fresh text when the user meant "rewrite this" is worse. |
| No selection detected | Normal compose path (by design, not an error). |
| Non-text clipboard (image) | Saved and restored by `GetSelection` (`FmtImage`), so compose presses do not destroy images. |
| Empty transcript | Existing path: skip injection, return to idle. |
| LLM / transcription failure | Existing error paths unchanged. |
| Injection failure | Existing path: `InjectionFailedError`, text left in clipboard for manual paste, recorded to state buffer. |
| Any produced text | Recorded via `RecordInjection` for "Copy Last Text", as today. |

## Testing

**Unit (no cgo):**
- Pipeline with fake audio, stub injector, fake selection reader:
  - selection present → transform prompt path used;
  - selection absent → compose prompt path used;
  - capture error → run aborted with error transition.
- Table tests for `RenderTransform` (selection text, user_context,
  instructions, language rule present in output).
- Clipboard save/restore logic where separable from the OS clipboard.

**Manual (macOS first, per platform status):**
- Rewrite a selection in TextEdit, a browser textarea, and an Electron app.
- No-selection press → fresh compose, clipboard intact afterwards.
- Image on clipboard → survives a compose press.
- Instruction spoken in a different language than the selection → output
  stays in the selection's language.
- Hold released quickly (sub-300 ms) → run completes correctly.

## Out of scope

- Accessibility-API selection reading (macOS `AXSelectedText`) — possible
  later optimization layered under the same interface; does not change
  config or UX.
- A strict transform-only stage that errors on missing selection.
- Opt-out config flag for selection capture.
- Linux/Windows selection capture (requires a non-interrupting copy strategy,
  e.g. Ctrl+Insert, and real platform testing).
