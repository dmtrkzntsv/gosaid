package inject

import "context"

// Compile-time check: PasteInjector provides selection capture.
var _ SelectionReader = (*PasteInjector)(nil)

// GetSelection captures the currently selected text via a clipboard
// round-trip: synthesized Cmd+C with save/restore of the previous
// clipboard contents (text or image). OK=false means no selection.
func (p *PasteInjector) GetSelection(ctx context.Context) SelectionResult {
	return captureSelection(ctx, osClipboard{}, synthesizeCopy, selectionTimeout, selectionInterval)
}
