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
