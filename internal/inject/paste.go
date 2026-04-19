package inject

import (
	"context"
	"fmt"
	"time"

	"golang.design/x/clipboard"
)

// PasteInjector saves the current clipboard, writes the text, synthesizes a
// paste keystroke, and restores the previous clipboard. If synthesis fails
// the text is left in the clipboard and InjectionFailedError signals that
// the user can recover with a manual Cmd/Ctrl+V.
//
// Non-text clipboard contents (images, files) are not preserved: Read with
// FmtText returns nil for them, and restoring nil would clobber the original
// data, so we skip the write in that case and leave the transcript behind
// rather than destroy the image.
type PasteInjector struct {
	inited bool
}

// NewPasteInjector returns a platform-aware injector. Returns an error if
// the clipboard backend fails to initialize.
func NewPasteInjector() (*PasteInjector, error) {
	if err := clipboard.Init(); err != nil {
		return nil, fmt.Errorf("clipboard init: %w", err)
	}
	return &PasteInjector{inited: true}, nil
}

// Inject temporarily replaces the clipboard with text to drive a synthesized
// paste, then restores the previous text contents.
func (p *PasteInjector) Inject(ctx context.Context, text string) error {
	if text == "" {
		return nil
	}
	prev := clipboard.Read(clipboard.FmtText)
	clipboard.Write(clipboard.FmtText, []byte(text))

	if err := synthesizePaste(); err != nil {
		return &InjectionFailedError{TextInClipboard: true, Underlying: err}
	}
	// Give the target app time to consume the paste before we restore the
	// clipboard — otherwise slow/async apps paste the restored value.
	select {
	case <-time.After(100 * time.Millisecond):
	case <-ctx.Done():
	}
	if prev != nil {
		clipboard.Write(clipboard.FmtText, prev)
	}
	return nil
}
