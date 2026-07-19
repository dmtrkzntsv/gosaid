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

// Compile-time check: PasteInjector provides selection capture.
var _ SelectionReader = (*PasteInjector)(nil)

// GetSelection captures the currently selected text via a clipboard
// round-trip: synthesized Cmd/Ctrl+C with save/restore of the previous
// clipboard contents (text or image). OK=false means no selection.
func (p *PasteInjector) GetSelection(ctx context.Context) SelectionResult {
	return captureSelection(ctx, osClipboard{}, synthesizeCopy, selectionTimeout, selectionInterval)
}
