package hotkey

import xh "golang.design/x/hotkey"

var modifierMap = map[string]xh.Modifier{
	"ctrl":    xh.ModCtrl,
	"control": xh.ModCtrl,
	"shift":   xh.ModShift,
	"alt":     xh.Mod1, // Alt is typically Mod1 on X11
	"super":   xh.Mod4, // Super (Windows key) is Mod4
	"cmd":     xh.Mod4,
	"command": xh.Mod4,
}

var keyMap = commonKeys()

// remapHint names where to remap a locking key on this platform.
const remapHint = "e.g. `setxkbmap -option caps:ctrl_modifier`, or your desktop's keyboard settings"
