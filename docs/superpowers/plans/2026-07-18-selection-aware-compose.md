# Selection-Aware Compose Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** When a compose-enabled hotkey fires while text is selected in the focused app, capture the selection via a clipboard round-trip and let the dictated speech act as a rewrite instruction for it; with no selection, compose behaves exactly as today.

**Architecture:** A new selection-capture unit in `internal/inject` mirrors the existing paste injector (save clipboard → sentinel → synthesized Cmd/Ctrl+C → poll for change → restore). The daemon starts capture concurrently with audio recording at hotkey press and hands the result to the pipeline via a channel; the compose stage picks a new `transform` prompt when a selection arrived, the existing compose prompt otherwise.

**Tech Stack:** Go, cgo (Quartz on macOS), `golang.design/x/clipboard`, `golang.org/x/sys/windows`, external keystroke tools on Linux (`wtype`/`xdotool`/`ydotool`).

**Spec:** `docs/superpowers/specs/2026-07-18-selection-aware-compose-design.md`

## Global Constraints

- No new dependencies — everything needed is already in `go.mod`.
- No config schema changes; selection-awareness is inherent to `compose`.
- All commands run from the repo root. Test with `go test ./...`, vet with `go vet ./...`, build with `make build`.
- macOS is the primary tested platform; Linux/Windows code compiles but manual verification is macOS-only (matches the project's platform status).
- Rewritten text must keep the selection's language unless the instruction explicitly requests a translation (prompt-enforced).
- The clipboard must be restored after every capture attempt, including text-absent (image) clipboards and error paths.
- Commit after every task with the message given in its final step.

## File map

| File | Change |
|---|---|
| `internal/inject/selection.go` | Create — `SelectionResult`, `SelectionReader`, `captureSelection`, `PasteInjector.GetSelection` |
| `internal/inject/selection_test.go` | Create — unit tests with a fake clipboard |
| `internal/inject/paste_darwin.go` | Modify — parametrize keycode, add `synthesizeCopy` |
| `internal/inject/paste_linux.go` | Modify — parametrize key, add `synthesizeCopy` |
| `internal/inject/paste_windows.go` | Modify — parametrize VK code, add `synthesizeCopy` |
| `internal/daemon/prompts/transform.tmpl` | Create — rewrite-selection system prompt |
| `internal/daemon/prompts.go` | Modify — embed + `TransformData` + `RenderTransform` |
| `internal/daemon/prompts_transform_test.go` | Create — template rendering tests |
| `internal/daemon/pipeline.go` | Modify — `Run` gains selection channel; new `transform()` |
| `internal/daemon/pipeline_test.go` | Modify — update `Run` callsites; new transform-path tests |
| `internal/daemon/daemon.go` | Modify — spawn capture at press, abort on capture error, pass channel |
| `README.md` | Modify — feature bullet |

---

### Task 1: Selection capture core (`captureSelection`)

Pure, platform-free capture logic: clipboard save → sentinel → synthesized copy (injected as a `func() error`) → poll → restore. Fully unit-testable with a fake clipboard.

**Files:**
- Create: `internal/inject/selection.go`
- Test: `internal/inject/selection_test.go`

**Interfaces:**
- Consumes: nothing new (uses `golang.design/x/clipboard` already in `go.mod`).
- Produces:
  - `type SelectionResult struct { Text string; OK bool; Err error }`
  - `type SelectionReader interface { GetSelection(ctx context.Context) SelectionResult }`
  - `func captureSelection(ctx context.Context, clip clipboardAPI, synth func() error, timeout, interval time.Duration) SelectionResult`
  - `type clipboardAPI interface { ReadText() []byte; ReadImage() []byte; WriteText([]byte); WriteImage([]byte) }`
  - Task 2 adds `PasteInjector.GetSelection`; Tasks 4–5 use `SelectionResult`/`SelectionReader`.

- [ ] **Step 1: Write the failing tests**

Create `internal/inject/selection_test.go`:

```go
package inject

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"
)

// fakeClipboard mimics an OS clipboard that holds either text or an image.
type fakeClipboard struct {
	text  []byte
	image []byte
}

func (f *fakeClipboard) ReadText() []byte    { return f.text }
func (f *fakeClipboard) ReadImage() []byte   { return f.image }
func (f *fakeClipboard) WriteText(b []byte)  { f.text = b; f.image = nil }
func (f *fakeClipboard) WriteImage(b []byte) { f.image = b; f.text = nil }

const (
	testTimeout  = 50 * time.Millisecond
	testInterval = 5 * time.Millisecond
)

func TestCaptureSelectionSuccess(t *testing.T) {
	clip := &fakeClipboard{text: []byte("before")}
	synth := func() error {
		clip.WriteText([]byte("hello world")) // app copies the selection
		return nil
	}
	res := captureSelection(context.Background(), clip, synth, testTimeout, testInterval)
	if res.Err != nil {
		t.Fatalf("unexpected err: %v", res.Err)
	}
	if !res.OK || res.Text != "hello world" {
		t.Fatalf("want OK with %q, got ok=%v text=%q", "hello world", res.OK, res.Text)
	}
	if !bytes.Equal(clip.text, []byte("before")) {
		t.Fatalf("clipboard not restored: %q", clip.text)
	}
}

func TestCaptureSelectionNoSelection(t *testing.T) {
	clip := &fakeClipboard{text: []byte("before")}
	synth := func() error { return nil } // app ignores the copy
	res := captureSelection(context.Background(), clip, synth, testTimeout, testInterval)
	if res.Err != nil || res.OK {
		t.Fatalf("want ok=false err=nil, got ok=%v err=%v", res.OK, res.Err)
	}
	if !bytes.Equal(clip.text, []byte("before")) {
		t.Fatalf("clipboard not restored: %q", clip.text)
	}
}

func TestCaptureSelectionSynthError(t *testing.T) {
	clip := &fakeClipboard{text: []byte("before")}
	boom := errors.New("no accessibility permission")
	res := captureSelection(context.Background(), clip, func() error { return boom }, testTimeout, testInterval)
	if !errors.Is(res.Err, boom) {
		t.Fatalf("want synth error, got %v", res.Err)
	}
	if res.OK {
		t.Fatal("OK must be false on error")
	}
	if !bytes.Equal(clip.text, []byte("before")) {
		t.Fatalf("clipboard not restored: %q", clip.text)
	}
}

func TestCaptureSelectionRestoresImage(t *testing.T) {
	clip := &fakeClipboard{image: []byte{0x89, 0x50, 0x4E, 0x47}} // png-ish bytes
	synth := func() error { return nil }
	res := captureSelection(context.Background(), clip, synth, testTimeout, testInterval)
	if res.OK || res.Err != nil {
		t.Fatalf("want ok=false err=nil, got ok=%v err=%v", res.OK, res.Err)
	}
	if !bytes.Equal(clip.image, []byte{0x89, 0x50, 0x4E, 0x47}) {
		t.Fatalf("image clipboard not restored: %v", clip.image)
	}
}

func TestCaptureSelectionEmptyClipboardRestoredEmpty(t *testing.T) {
	clip := &fakeClipboard{} // nothing on the clipboard
	synth := func() error { return nil }
	_ = captureSelection(context.Background(), clip, synth, testTimeout, testInterval)
	if len(clip.text) != 0 || len(clip.image) != 0 {
		t.Fatalf("sentinel leaked: text=%q image=%v", clip.text, clip.image)
	}
}

func TestCaptureSelectionContextCancel(t *testing.T) {
	clip := &fakeClipboard{text: []byte("before")}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	res := captureSelection(ctx, clip, func() error { return nil }, 10*time.Second, testInterval)
	if res.OK || res.Err != nil {
		t.Fatalf("want ok=false err=nil, got ok=%v err=%v", res.OK, res.Err)
	}
	if time.Since(start) > time.Second {
		t.Fatal("cancelled capture did not return promptly")
	}
	if !bytes.Equal(clip.text, []byte("before")) {
		t.Fatalf("clipboard not restored: %q", clip.text)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/inject/ -run TestCaptureSelection -v`
Expected: compile error — `undefined: captureSelection` (and `clipboardAPI`).

- [ ] **Step 3: Write the implementation**

Create `internal/inject/selection.go`:

```go
package inject

import (
	"bytes"
	"context"
	"time"

	"golang.design/x/clipboard"
)

// SelectionResult is the outcome of a selection-capture attempt.
// OK=false with Err=nil means "no selection" — a valid state, not a failure.
type SelectionResult struct {
	Text string
	OK   bool
	Err  error
}

// SelectionReader captures the currently selected text in the focused app.
// PasteInjector implements it; the daemon type-asserts for it so injectors
// without capture support (e.g. the test Stub) degrade to plain compose.
type SelectionReader interface {
	GetSelection(ctx context.Context) SelectionResult
}

// clipboardAPI abstracts the OS clipboard so captureSelection is testable
// without touching the real clipboard.
type clipboardAPI interface {
	ReadText() []byte
	ReadImage() []byte
	WriteText([]byte)
	WriteImage([]byte)
}

type osClipboard struct{}

func (osClipboard) ReadText() []byte    { return clipboard.Read(clipboard.FmtText) }
func (osClipboard) ReadImage() []byte   { return clipboard.Read(clipboard.FmtImage) }
func (osClipboard) WriteText(b []byte)  { clipboard.Write(clipboard.FmtText, b) }
func (osClipboard) WriteImage(b []byte) { clipboard.Write(clipboard.FmtImage, b) }

// selectionSentinel marks the clipboard so a copy that lands can be told
// apart from the pre-existing contents. Invisible-separator runes keep it
// harmless if an app ever renders it.
const selectionSentinel = "⁣gosaid-selection-probe⁣"

const (
	selectionTimeout  = 300 * time.Millisecond
	selectionInterval = 20 * time.Millisecond
)

// captureSelection copies the current selection through the clipboard:
// save contents → write sentinel → synthesize copy → poll for a change →
// restore contents. Detection is a heuristic: an app that ignores the copy
// keystroke is indistinguishable from "no selection"; both return OK=false.
func captureSelection(ctx context.Context, clip clipboardAPI, synth func() error, timeout, interval time.Duration) SelectionResult {
	sentinel := []byte(selectionSentinel)
	prevText := clip.ReadText()
	var prevImage []byte
	if prevText == nil {
		prevImage = clip.ReadImage()
	}
	restore := func() {
		switch {
		case prevText != nil:
			clip.WriteText(prevText)
		case prevImage != nil:
			clip.WriteImage(prevImage)
		default:
			clip.WriteText([]byte{}) // clipboard was empty — don't leak the sentinel
		}
	}

	clip.WriteText(sentinel)
	if err := synth(); err != nil {
		restore()
		return SelectionResult{Err: err}
	}

	deadline := time.After(timeout)
	for {
		if got := clip.ReadText(); len(got) > 0 && !bytes.Equal(got, sentinel) {
			restore()
			return SelectionResult{Text: string(got), OK: true}
		}
		select {
		case <-time.After(interval):
		case <-deadline:
			restore()
			return SelectionResult{}
		case <-ctx.Done():
			restore()
			return SelectionResult{}
		}
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/inject/ -run TestCaptureSelection -v`
Expected: all 6 tests PASS.

- [ ] **Step 5: Vet and commit**

```bash
go vet ./internal/inject/
git add internal/inject/selection.go internal/inject/selection_test.go
git commit -m "feat: clipboard-based selection capture core"
```

---

### Task 2: Platform copy synthesis + `PasteInjector.GetSelection`

Add `synthesizeCopy()` on each platform by parametrizing the existing paste synthesis, then wire `GetSelection` onto `PasteInjector`.

**Files:**
- Modify: `internal/inject/paste_darwin.go`
- Modify: `internal/inject/paste_linux.go`
- Modify: `internal/inject/paste_windows.go`
- Modify: `internal/inject/selection.go` (append `GetSelection`)

**Interfaces:**
- Consumes: `captureSelection`, `osClipboard`, `selectionTimeout`, `selectionInterval` from Task 1.
- Produces: `func (p *PasteInjector) GetSelection(ctx context.Context) SelectionResult` — making `*PasteInjector` satisfy `SelectionReader`. Task 5 relies on this via type assertion.

No new unit tests: this task is thin platform glue over OS keystroke synthesis (same test posture as the existing `synthesizePaste`). Correctness is covered by the compile-time interface check below, `go vet`, and the Task 6 manual matrix.

- [ ] **Step 1: Parametrize macOS synthesis**

Replace the C block and Go function in `internal/inject/paste_darwin.go` so the whole file reads:

```go
package inject

/*
#cgo LDFLAGS: -framework ApplicationServices -framework CoreFoundation
#include <ApplicationServices/ApplicationServices.h>

// kVK_ANSI_V = 0x09, kVK_ANSI_C = 0x08
// Synthesizes Cmd+<key> via Quartz Event Services. Returns 0 on success.
// Flags are set explicitly on the event, so physically-held hotkey
// modifiers (e.g. Option) do not leak into the synthesized keystroke.
static int synth_combo(CGKeyCode key) {
    CGEventSourceRef src = CGEventSourceCreate(kCGEventSourceStateHIDSystemState);
    if (!src) return 1;

    CGEventRef down = CGEventCreateKeyboardEvent(src, key, true);
    CGEventRef up   = CGEventCreateKeyboardEvent(src, key, false);
    if (!down || !up) {
        if (down) CFRelease(down);
        if (up)   CFRelease(up);
        CFRelease(src);
        return 2;
    }
    CGEventSetFlags(down, kCGEventFlagMaskCommand);
    CGEventSetFlags(up,   kCGEventFlagMaskCommand);
    CGEventPost(kCGHIDEventTap, down);
    CGEventPost(kCGHIDEventTap, up);

    CFRelease(down);
    CFRelease(up);
    CFRelease(src);
    return 0;
}

static int synth_paste(void) { return synth_combo((CGKeyCode)0x09); }
static int synth_copy(void)  { return synth_combo((CGKeyCode)0x08); }
*/
import "C"

import "fmt"

func synthesizePaste() error {
	code := C.synth_paste()
	if code != 0 {
		return fmt.Errorf("CGEvent paste synthesis failed (code %d) — grant gosaid Accessibility permission in System Settings", code)
	}
	return nil
}

func synthesizeCopy() error {
	code := C.synth_copy()
	if code != 0 {
		return fmt.Errorf("CGEvent copy synthesis failed (code %d) — grant gosaid Accessibility permission in System Settings", code)
	}
	return nil
}
```

- [ ] **Step 2: Parametrize Linux synthesis**

Replace `internal/inject/paste_linux.go` contents with:

```go
package inject

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
)

func synthesizePaste() error { return synthesizeCombo("v", "47") } // KEY_V = 47
func synthesizeCopy() error  { return synthesizeCombo("c", "46") } // KEY_C = 46

// synthesizeCombo emits Ctrl+<key>, trying the most likely tool present.
// Wayland: wtype (native), then ydotool (requires daemon+uinput).
// X11:     xdotool, ydotool.
// xkey is the X keysym name; ycode the Linux input event code for ydotool.
func synthesizeCombo(xkey, ycode string) error {
	isWayland := os.Getenv("WAYLAND_DISPLAY") != ""
	candidates := []injectCmd{
		{"xdotool", []string{"key", "--clearmodifiers", "ctrl+" + xkey}},
		{"ydotool", []string{"key", "29:1", ycode + ":1", ycode + ":0", "29:0"}}, // 29 = KEY_LEFTCTRL
	}
	if isWayland {
		candidates = []injectCmd{
			{"wtype", []string{"-M", "ctrl", xkey, "-m", "ctrl"}},
			{"ydotool", []string{"key", "29:1", ycode + ":1", ycode + ":0", "29:0"}},
		}
	}
	var lastErr error
	for _, c := range candidates {
		if _, err := exec.LookPath(c.bin); err != nil {
			lastErr = errors.Join(lastErr, err)
			continue
		}
		if err := exec.Command(c.bin, c.args...).Run(); err != nil {
			lastErr = errors.Join(lastErr, fmt.Errorf("%s: %w", c.bin, err))
			continue
		}
		return nil
	}
	if lastErr == nil {
		lastErr = errors.New("no keystroke synthesis tool available")
	}
	return fmt.Errorf("keystroke synthesis failed: %w — install one of: wtype, xdotool, ydotool", lastErr)
}

type injectCmd struct {
	bin  string
	args []string
}
```

(This removes `waylandCandidates`/`xdotoolCandidates`; nothing else references them.)

- [ ] **Step 3: Parametrize Windows synthesis**

In `internal/inject/paste_windows.go`:

Change the const block to:

```go
const (
	vkControl     = 0x11
	vkC           = 0x43
	vkV           = 0x56
	inputKeyboard = 1
	keyEventKeyUp = 0x0002
)
```

Replace `func synthesizePaste() error { ... }` with:

```go
func synthesizePaste() error { return sendCtrlCombo(vkV) }
func synthesizeCopy() error  { return sendCtrlCombo(vkC) }

func sendCtrlCombo(vk uint16) error {
	events := []input{
		{inputType: inputKeyboard, ki: keyboardInput{wVk: vkControl}},                         // ctrl down
		{inputType: inputKeyboard, ki: keyboardInput{wVk: vk}},                                // key  down
		{inputType: inputKeyboard, ki: keyboardInput{wVk: vk, dwFlags: keyEventKeyUp}},        // key  up
		{inputType: inputKeyboard, ki: keyboardInput{wVk: vkControl, dwFlags: keyEventKeyUp}}, // ctrl up
	}
	r, _, err := sendInput.Call(
		uintptr(len(events)),
		uintptr(unsafe.Pointer(&events[0])),
		unsafe.Sizeof(events[0]),
	)
	if int(r) != len(events) {
		return fmt.Errorf("SendInput sent %d of %d events: %v", r, len(events), err)
	}
	return nil
}
```

- [ ] **Step 4: Add `GetSelection` to `PasteInjector`**

Append to `internal/inject/selection.go`:

```go
// Compile-time check: PasteInjector provides selection capture.
var _ SelectionReader = (*PasteInjector)(nil)

// GetSelection captures the currently selected text via a clipboard
// round-trip: synthesized Cmd/Ctrl+C with save/restore of the previous
// clipboard contents (text or image). OK=false means no selection.
func (p *PasteInjector) GetSelection(ctx context.Context) SelectionResult {
	return captureSelection(ctx, osClipboard{}, synthesizeCopy, selectionTimeout, selectionInterval)
}
```

- [ ] **Step 5: Build, vet, test**

Run: `go build ./... && go vet ./... && go test ./internal/inject/ -v`
Expected: build and vet clean, all selection tests PASS. (Linux/Windows files are exercised by CI's cross-platform builds; they can't compile on macOS.)

- [ ] **Step 6: Commit**

```bash
git add internal/inject/paste_darwin.go internal/inject/paste_linux.go internal/inject/paste_windows.go internal/inject/selection.go
git commit -m "feat: per-platform copy synthesis and PasteInjector.GetSelection"
```

---

### Task 3: Transform prompt template

**Files:**
- Create: `internal/daemon/prompts/transform.tmpl`
- Modify: `internal/daemon/prompts.go`
- Test: `internal/daemon/prompts_transform_test.go`

**Interfaces:**
- Consumes: nothing from earlier tasks.
- Produces: `type TransformData struct { Selection, UserContext, Instructions string }` and `func RenderTransform(d TransformData) (string, error)`. Task 4 calls `RenderTransform`.

- [ ] **Step 1: Write the failing tests**

Create `internal/daemon/prompts_transform_test.go`:

```go
package daemon

import (
	"strings"
	"testing"
)

func TestRenderTransformIncludesSelection(t *testing.T) {
	out, err := RenderTransform(TransformData{Selection: "hey buddy, fix the thing"})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(out, "hey buddy, fix the thing") {
		t.Fatalf("selection missing from prompt:\n%s", out)
	}
	if !strings.Contains(out, "same language as the selected text") {
		t.Fatalf("language rule missing from prompt:\n%s", out)
	}
	if strings.Contains(out, "About the user") {
		t.Fatalf("user-context block must be absent when empty:\n%s", out)
	}
	if strings.Contains(out, "Additional instructions") {
		t.Fatalf("instructions block must be absent when empty:\n%s", out)
	}
}

func TestRenderTransformOptionalBlocks(t *testing.T) {
	out, err := RenderTransform(TransformData{
		Selection:    "text",
		UserContext:  "  Dmitry, staff engineer  ",
		Instructions: "  prefer plain words  ",
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(out, "Dmitry, staff engineer") {
		t.Fatalf("user context missing:\n%s", out)
	}
	if !strings.Contains(out, "prefer plain words") {
		t.Fatalf("instructions missing:\n%s", out)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/daemon/ -run TestRenderTransform -v`
Expected: compile error — `undefined: RenderTransform`.

- [ ] **Step 3: Create the template**

Create `internal/daemon/prompts/transform.tmpl`:

```
You are a text editor that rewrites an existing piece of text according to a spoken instruction.

The user selected the text below and dictated an instruction describing how to change it. Apply the instruction to the selected text and output the result.

Selected text:
<<<
{{.Selection}}
>>>
{{if .UserContext}}
About the user (use for personalization — name, role, tone, sign-offs — when relevant; ignore if not applicable to the current task):
{{.UserContext}}
{{end}}{{if .Instructions}}
Additional instructions for this hotkey (apply on top of the defaults above; do not override the output-format rules below):
{{.Instructions}}
{{end}}
Rules:
- Keep the output in the same language as the selected text, even if the instruction was spoken in a different language — unless the instruction explicitly asks to translate.
- Apply only the requested change; preserve everything else — meaning, tone, formatting, paragraph breaks, proper nouns, code identifiers, URLs — as much as the instruction allows.
- The instruction is a speech-to-text transcript and may contain disfluencies; act on its intent.

Output requirements:
- Output only the rewritten text, nothing else.
- No commentary, no preamble, no quotation marks or code fences wrapping the output.
- If the instruction is ambiguous, produce the most reasonable interpretation rather than asking questions.
```

- [ ] **Step 4: Wire it into `prompts.go`**

In `internal/daemon/prompts.go`:

Change the embed directive to:

```go
//go:embed prompts/translate.tmpl prompts/enhance.tmpl prompts/compose.tmpl prompts/transform.tmpl
var promptFS embed.FS
```

Add to the `var (...)` block:

```go
	transformTmpl = template.Must(template.ParseFS(promptFS, "prompts/transform.tmpl"))
```

Add after `ComposeData`:

```go
type TransformData struct {
	Selection    string
	UserContext  string
	Instructions string
}
```

Add after `RenderCompose`:

```go
func RenderTransform(d TransformData) (string, error) {
	d.UserContext = strings.TrimSpace(d.UserContext)
	d.Instructions = strings.TrimSpace(d.Instructions)
	var b strings.Builder
	if err := transformTmpl.Execute(&b, d); err != nil {
		return "", err
	}
	return b.String(), nil
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/daemon/ -run TestRenderTransform -v`
Expected: both tests PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/daemon/prompts/transform.tmpl internal/daemon/prompts.go internal/daemon/prompts_transform_test.go
git commit -m "feat: transform prompt for rewriting selected text"
```

---

### Task 4: Pipeline transform path

`Pipeline.Run` gains a selection-result channel. In the compose stage: selection captured → transform prompt; none → existing compose; capture error → error transition.

**Files:**
- Modify: `internal/daemon/pipeline.go`
- Test: `internal/daemon/pipeline_test.go`

**Interfaces:**
- Consumes: `inject.SelectionResult` (Task 1), `RenderTransform`/`TransformData` (Task 3).
- Produces: `func (p *Pipeline) Run(ctx context.Context, hk config.Hotkey, sel <-chan inject.SelectionResult) error` — a nil channel means "no capture was attempted" and preserves today's behavior. Task 5 passes the channel from the daemon.

- [ ] **Step 1: Update existing callsites for the new signature**

In `internal/daemon/pipeline_test.go`, every existing call of the form
`p.Run(context.Background(), cfg.Hotkeys["ctrl+alt+space"])` gains a trailing `nil` argument:

```go
p.Run(context.Background(), cfg.Hotkeys["ctrl+alt+space"], nil)
```

(11 callsites at the time of writing — update all; the compiler will find any stragglers.)

- [ ] **Step 2: Write the failing tests**

Append to `internal/daemon/pipeline_test.go` (follow the existing compose-test pattern in that file for building `cfg`; the hotkey needs `Transcribe: config.TranscribeStage{Model: "m:x"}` and `Compose: &config.ComposeStage{Model: "m:c"}`):

```go
func TestComposeWithSelectionUsesTransformPrompt(t *testing.T) {
	var gotSystem, gotUser string
	drv := &mockDriver{
		transcribe: func(model string, _ drivers.TranscribeOptions) (drivers.TranscribeResult, error) {
			return drivers.TranscribeResult{Text: "make it formal", DetectedLanguage: "en"}, nil
		},
		chat: func(_, system, user string) (string, error) {
			gotSystem, gotUser = system, user
			return "Formal text.", nil
		},
	}
	var sink strings.Builder
	cfg := &config.Config{
		Version: 2,
		Hotkeys: map[string]config.Hotkey{"ctrl+alt+space": {
			Transcribe: config.TranscribeStage{Model: "m:x"},
			Compose:    &config.ComposeStage{Model: "m:c"},
		}},
		ToggleMaxSeconds: 1,
		InjectionMode:    config.InjectionModePaste,
	}
	p := newPipeline(t, drv, cfg, &sink)

	sel := make(chan inject.SelectionResult, 1)
	sel <- inject.SelectionResult{Text: "hey buddy", OK: true}
	if err := p.Run(context.Background(), cfg.Hotkeys["ctrl+alt+space"], sel); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(gotSystem, "hey buddy") {
		t.Fatalf("system prompt must embed the selection, got:\n%s", gotSystem)
	}
	if !strings.Contains(gotSystem, "rewrites an existing piece of text") {
		t.Fatalf("expected transform prompt, got:\n%s", gotSystem)
	}
	if gotUser != "make it formal" {
		t.Fatalf("user message must be the transcript, got %q", gotUser)
	}
	if sink.String() != "Formal text." {
		t.Fatalf("injected %q", sink.String())
	}
}

func TestComposeWithoutSelectionFallsBack(t *testing.T) {
	var gotSystem string
	drv := &mockDriver{
		transcribe: func(model string, _ drivers.TranscribeOptions) (drivers.TranscribeResult, error) {
			return drivers.TranscribeResult{Text: "write a greeting", DetectedLanguage: "en"}, nil
		},
		chat: func(_, system, _ string) (string, error) {
			gotSystem = system
			return "Hello!", nil
		},
	}
	var sink strings.Builder
	cfg := &config.Config{
		Version: 2,
		Hotkeys: map[string]config.Hotkey{"ctrl+alt+space": {
			Transcribe: config.TranscribeStage{Model: "m:x"},
			Compose:    &config.ComposeStage{Model: "m:c"},
		}},
		ToggleMaxSeconds: 1,
		InjectionMode:    config.InjectionModePaste,
	}
	p := newPipeline(t, drv, cfg, &sink)

	sel := make(chan inject.SelectionResult, 1)
	sel <- inject.SelectionResult{} // capture ran, found nothing
	if err := p.Run(context.Background(), cfg.Hotkeys["ctrl+alt+space"], sel); err != nil {
		t.Fatalf("run: %v", err)
	}
	if strings.Contains(gotSystem, "rewrites an existing piece of text") {
		t.Fatalf("must use compose prompt when no selection, got:\n%s", gotSystem)
	}
	if sink.String() != "Hello!" {
		t.Fatalf("injected %q", sink.String())
	}
}

func TestComposeSelectionCaptureError(t *testing.T) {
	drv := &mockDriver{
		transcribe: func(model string, _ drivers.TranscribeOptions) (drivers.TranscribeResult, error) {
			return drivers.TranscribeResult{Text: "anything", DetectedLanguage: "en"}, nil
		},
		chat: func(_, _, _ string) (string, error) {
			t.Fatal("chat must not be called on capture error")
			return "", nil
		},
	}
	var sink strings.Builder
	cfg := &config.Config{
		Version: 2,
		Hotkeys: map[string]config.Hotkey{"ctrl+alt+space": {
			Transcribe: config.TranscribeStage{Model: "m:x"},
			Compose:    &config.ComposeStage{Model: "m:c"},
		}},
		ToggleMaxSeconds: 1,
		InjectionMode:    config.InjectionModePaste,
	}
	p := newPipeline(t, drv, cfg, &sink)

	sel := make(chan inject.SelectionResult, 1)
	sel <- inject.SelectionResult{Err: errors.New("copy synthesis failed")}
	err := p.Run(context.Background(), cfg.Hotkeys["ctrl+alt+space"], sel)
	if err == nil || !strings.Contains(err.Error(), "copy synthesis failed") {
		t.Fatalf("want capture error, got %v", err)
	}
	if sink.String() != "" {
		t.Fatalf("nothing must be injected, got %q", sink.String())
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./internal/daemon/ -run 'TestCompose(With|Selection)' -v`
Expected: compile error — `Run` does not yet take a third argument (Step 1 already added it at callsites).

- [ ] **Step 4: Implement the pipeline changes**

In `internal/daemon/pipeline.go`:

Change the `Run` signature and doc comment:

```go
// Run executes the full pipeline for one hotkey trigger. Called after the
// user releases the hotkey (or the toggle-mode cap fires). sel, when
// non-nil, delivers the result of the selection capture started at hotkey
// press; it is only set for compose-enabled hotkeys.
func (p *Pipeline) Run(ctx context.Context, hk config.Hotkey, sel <-chan inject.SelectionResult) error {
```

Replace the compose case in the `switch`:

```go
	case hk.Compose.IsEnabled():
		if hk.Enhance.IsEnabled() {
			p.Log.Debug("compose set: enhance stage skipped")
		}
		var selRes inject.SelectionResult
		if sel != nil {
			select {
			case selRes = <-sel:
			case <-ctx.Done():
				p.Core.Transition(StateError, ctx.Err())
				return ctx.Err()
			}
		}
		if selRes.Err != nil {
			p.Core.Transition(StateError, selRes.Err)
			return selRes.Err
		}
		if selRes.OK {
			p.Log.Debug("selection captured", "chars", len(selRes.Text))
			reshaped, err = p.transform(ctx, text1, selRes.Text, hk.Compose)
		} else {
			reshaped, err = p.compose(ctx, text1, hk.Compose)
		}
		// Compose/transform may produce output in a different language than
		// the transcript (e.g. Russian instruction about English text). Drop
		// the stale language hint so translate neither skips incorrectly nor
		// fills the prompt with a wrong source.
		translateLang = ""
```

Add after `compose()`:

```go
// transform rewrites captured selection text according to the dictated
// instruction, reusing the compose stage's model and instructions.
func (p *Pipeline) transform(ctx context.Context, instruction, selection string, stage *config.ComposeStage) (string, error) {
	drv, model, err := p.resolve(stage.Model)
	if err != nil {
		return "", err
	}
	system, err := RenderTransform(TransformData{
		Selection:    selection,
		UserContext:  p.Config.UserContext,
		Instructions: stage.Instructions,
	})
	if err != nil {
		return "", err
	}
	out, err := drv.Chat(ctx, model, system, instruction)
	if err != nil {
		return "", err
	}
	p.Log.Debug("transform", "text", out)
	return out, nil
}
```

Also update the one non-test caller: in `internal/daemon/daemon.go`, `pipe.Run(pctx, hk)` becomes `pipe.Run(pctx, hk, nil)` for now (Task 5 replaces the `nil`).

- [ ] **Step 5: Run the full test suite**

Run: `go test ./...`
Expected: all packages PASS (new tests and all pre-existing pipeline tests).

- [ ] **Step 6: Commit**

```bash
git add internal/daemon/pipeline.go internal/daemon/pipeline_test.go internal/daemon/daemon.go
git commit -m "feat: selection-aware compose path in pipeline"
```

---

### Task 5: Daemon wiring — capture at press

Start selection capture concurrently with audio recording when a compose-enabled hotkey is pressed; abort with an error cue if copy synthesis fails while still recording; hand the channel to the pipeline on release.

**Files:**
- Modify: `internal/daemon/daemon.go`

**Interfaces:**
- Consumes: `inject.SelectionReader`, `inject.SelectionResult` (Tasks 1–2), `Pipeline.Run(ctx, hk, sel)` (Task 4).
- Produces: runtime behavior only; no new exported symbols.

This is glue inside `daemon.Run` (a function that wires OS-level subsystems and has no unit-test harness); logic-level behavior is already covered by the Task 4 pipeline tests, and the press-time abort is verified in the Task 6 manual matrix.

- [ ] **Step 1: Resolve the selection reader once**

In `internal/daemon/daemon.go`, immediately before the `for combo, hk := range cfg.Hotkeys` loop, add:

```go
	// Selection capture is available only for injectors that support it
	// (PasteInjector does; the test Stub doesn't). Without it, compose
	// hotkeys behave as before: fresh composition, no clipboard touch.
	selReader, _ := injector.(inject.SelectionReader)
```

- [ ] **Step 2: Spawn capture at press, pass the channel at release**

Inside the hotkey loop, replace the `var captureLive atomic.Bool` declaration and the `handler := hotkey.Handler{...}` literal with:

```go
		// captureLive tracks whether the most recent OnTrigger actually
		// started capture. OnStop reads it to decide whether to run the
		// pipeline — guards against a Start() failure followed by error
		// auto-recovery (Error→Idle) racing the user's hotkey release.
		// The selection-capture goroutine also swaps it to abort the run
		// when copy synthesis fails while recording is still live.
		var captureLive atomic.Bool
		// selCh carries the selection-capture result from press to release.
		// OnTrigger and OnStop run on the hotkey manager's event goroutine,
		// so plain assignment is safe.
		var selCh chan inject.SelectionResult
		handler := hotkey.Handler{
			OnTrigger: func() {
				captureLive.Store(false)
				selCh = nil
				if !core.TryStartRecording() {
					log.Debug("hotkey press ignored — core busy", "combo", combo)
					return
				}
				opened, err := capturer.Start(hk.Microphone)
				if err != nil {
					core.Transition(StateError, err)
					return
				}
				if hk.Microphone != "" && !audio.MatchesDevice(opened, hk.Microphone) {
					log.Warn("configured microphone not found — using fallback",
						"combo", combo, "want", hk.Microphone, "using", opened)
				}
				captureLive.Store(true)
				if hk.Compose.IsEnabled() && selReader != nil {
					ch := make(chan inject.SelectionResult, 1)
					selCh = ch
					go func() {
						res := selReader.GetSelection(ctx)
						// Copy synthesis failure means the later paste would
						// fail too — abort now with the error cue rather than
						// composing text the user meant as a rewrite command.
						// Swap decides the winner if release races us: whoever
						// flips captureLive first owns stopping the capturer.
						if res.Err != nil && captureLive.Swap(false) {
							_, _ = capturer.Stop()
							core.Transition(StateError, res.Err)
						}
						ch <- res
					}()
				}
			},
			OnStop: func() {
				if !captureLive.Swap(false) {
					log.Debug("hotkey release ignored — capture never started", "combo", combo)
					return
				}
				sel := selCh
				go func() {
					pctx, pcancel := context.WithTimeout(ctx, 90*time.Second)
					defer pcancel()
					if err := pipe.Run(pctx, hk, sel); err != nil {
						log.Error("pipeline", "combo", combo, "err", err)
					}
				}()
			},
		}
```

(If the capture goroutine loses the race — release fired first — the pipeline consumes the buffered error from the channel and aborts via the Task 4 path instead.)

- [ ] **Step 3: Build and run the full suite**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: clean build, vet, all tests PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/daemon/daemon.go
git commit -m "feat: start selection capture at hotkey press for compose hotkeys"
```

---

### Task 6: README + manual verification

**Files:**
- Modify: `README.md`

**Interfaces:** none — documentation and end-to-end verification of Tasks 1–5.

- [ ] **Step 1: Add the feature bullet to README**

In `README.md`, in the bullet list under the opening paragraph (after the "**Local or cloud**" bullet), add:

```markdown
- **Edit by voice** — select text in any app, hold a compose hotkey, and say how to change it ("make it shorter", "more formal"). The selection is replaced in place; with nothing selected, the same hotkey composes fresh text.
```

- [ ] **Step 2: Build and start the daemon**

Run: `make build && ./gosaid`
Expected: `gosaid running — press configured hotkey to dictate, Ctrl+C to quit`. Config must have a compose hotkey (e.g. `option+up` from `internal/config/config.example.json`).

- [ ] **Step 3: Manual matrix (macOS)**

Verify each and note results:

1. **Rewrite in TextEdit:** type and select a sentence, hold the compose hotkey, say "make this more formal", release → selection replaced by a formal rewrite.
2. **Browser textarea and an Electron app** (e.g. a GitHub comment box, Slack/VS Code): same flow works.
3. **No selection:** click into empty space (no selection), compose hotkey + "write a short greeting" → fresh text at cursor; previously-copied clipboard text still pastes with Cmd+V afterwards.
4. **Image clipboard survives:** copy a screenshot region (Cmd+Ctrl+Shift+4), press compose hotkey with no selection, dictate anything, then Cmd+V into Preview → the image still pastes.
5. **Cross-language:** select English text, dictate the instruction in another language (e.g. Russian "сделай короче") → output stays English.
6. **Quick tap:** press and release the compose hotkey in under ~300 ms with a selection → no hang; run completes or exits cleanly.
7. **Error cue:** temporarily revoke Accessibility permission for the terminal running gosaid, press the compose hotkey → error cue sounds, nothing typed. Restore the permission afterwards.

Expected: all 7 pass. If any fail, stop and fix before committing.

- [ ] **Step 4: Commit**

```bash
git add README.md
git commit -m "docs: README bullet for selection-aware compose"
```
