package inject

import (
	"bytes"
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

// CGEventPost and the equivalent platform helpers only enqueue the paste
// keystroke. Some applications do not read the clipboard until their event
// loop handles it, so restoring too quickly can make them paste the previous
// clipboard value instead of the generated text.
const pasteRestoreDelay = 750 * time.Millisecond

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
	return injectPaste(ctx, osClipboard{}, synthesizePaste, text, pasteRestoreDelay)
}

func injectPaste(ctx context.Context, clip clipboardAPI, synth func() error, text string, restoreDelay time.Duration) error {
	prev := clip.ReadText()
	injected := []byte(text)
	clip.WriteText(injected)

	if err := synth(); err != nil {
		return &InjectionFailedError{TextInClipboard: true, Underlying: err}
	}

	timer := time.NewTimer(restoreDelay)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-ctx.Done():
		// The paste event may still be queued. Leave the generated text in the
		// clipboard so a late consumer cannot receive the previous contents.
		return nil
	}

	// Do not overwrite a copy made by the user (or another application) while
	// waiting for the target to consume the paste.
	if prev != nil && bytes.Equal(clip.ReadText(), injected) {
		clip.WriteText(prev)
	}
	return nil
}
