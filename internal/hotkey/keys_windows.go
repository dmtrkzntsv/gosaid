package hotkey

import xh "golang.design/x/hotkey"

var modifierMap = map[string]xh.Modifier{
	"ctrl":    xh.ModCtrl,
	"control": xh.ModCtrl,
	"shift":   xh.ModShift,
	"alt":     xh.ModAlt,
	"cmd":     xh.ModWin,
	"command": xh.ModWin,
	"super":   xh.ModWin,
	"win":     xh.ModWin,
}

var keyMap = commonKeys()

// remapHint names where to remap a locking key on this platform.
const remapHint = "e.g. PowerToys Keyboard Manager, or a Scancode Map registry entry"
