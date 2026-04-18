package inject

import (
	"context"
	"fmt"
	"time"

	"golang.design/x/clipboard"
)

// PasteInjector writes the text to the system clipboard and synthesizes a
// paste keystroke on the platform. If the keystroke synthesis fails, the
// text is still in the clipboard and InjectionFailedError signals that
// the user can recover with a manual Cmd/Ctrl+V.
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

// Inject puts text on the clipboard and attempts to fire the platform paste
// keystroke. Does not restore the previous clipboard content.
func (p *PasteInjector) Inject(ctx context.Context, text string) error {
	if text == "" {
		return nil
	}
	clipboard.Write(clipboard.FmtText, []byte(text))

	if err := synthesizePaste(); err != nil {
		return &InjectionFailedError{TextInClipboard: true, Underlying: err}
	}
	// Give the target app time to consume the paste before we return and
	// potentially re-enter the state machine.
	select {
	case <-time.After(100 * time.Millisecond):
	case <-ctx.Done():
	}
	return nil
}
