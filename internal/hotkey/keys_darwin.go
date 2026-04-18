package hotkey

import xh "golang.design/x/hotkey"

var modifierMap = map[string]xh.Modifier{
	"ctrl":    xh.ModCtrl,
	"control": xh.ModCtrl,
	"shift":   xh.ModShift,
	"alt":     xh.ModOption,
	"option":  xh.ModOption,
	"cmd":     xh.ModCmd,
	"command": xh.ModCmd,
	"super":   xh.ModCmd,
}

var keyMap = commonKeys()
