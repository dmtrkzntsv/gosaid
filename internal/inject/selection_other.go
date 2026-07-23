//go:build !darwin

package inject

import "context"

// Compile-time check: PasteInjector provides selection capture.
var _ SelectionReader = (*PasteInjector)(nil)

// GetSelection is disabled off macOS: synthesizing Ctrl+C would interrupt
// the foreground process when a terminal has focus (SIGINT on Linux,
// CTRL_C_EVENT on Windows). Until capture on these platforms is designed
// and tested, report "no selection" without touching the clipboard, so
// compose hotkeys keep their pre-capture behavior.
func (p *PasteInjector) GetSelection(_ context.Context) SelectionResult {
	return SelectionResult{}
}
