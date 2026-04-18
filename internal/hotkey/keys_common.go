package hotkey

import xh "golang.design/x/hotkey"

// commonKeys returns the name→Key map shared by all platforms. Values of
// the xh.Key constants differ per OS but the identifiers are the same.
func commonKeys() map[string]xh.Key {
	return map[string]xh.Key{
		"space": xh.KeySpace,
		"tab":   xh.KeyTab,
		"enter": xh.KeyReturn,
		"esc":   xh.KeyEscape,

		"a": xh.KeyA, "b": xh.KeyB, "c": xh.KeyC, "d": xh.KeyD,
		"e": xh.KeyE, "f": xh.KeyF, "g": xh.KeyG, "h": xh.KeyH,
		"i": xh.KeyI, "j": xh.KeyJ, "k": xh.KeyK, "l": xh.KeyL,
		"m": xh.KeyM, "n": xh.KeyN, "o": xh.KeyO, "p": xh.KeyP,
		"q": xh.KeyQ, "r": xh.KeyR, "s": xh.KeyS, "t": xh.KeyT,
		"u": xh.KeyU, "v": xh.KeyV, "w": xh.KeyW, "x": xh.KeyX,
		"y": xh.KeyY, "z": xh.KeyZ,

		"0": xh.Key0, "1": xh.Key1, "2": xh.Key2, "3": xh.Key3, "4": xh.Key4,
		"5": xh.Key5, "6": xh.Key6, "7": xh.Key7, "8": xh.Key8, "9": xh.Key9,

		"f1": xh.KeyF1, "f2": xh.KeyF2, "f3": xh.KeyF3, "f4": xh.KeyF4,
		"f5": xh.KeyF5, "f6": xh.KeyF6, "f7": xh.KeyF7, "f8": xh.KeyF8,
		"f9": xh.KeyF9, "f10": xh.KeyF10, "f11": xh.KeyF11, "f12": xh.KeyF12,

		"left": xh.KeyLeft, "right": xh.KeyRight, "up": xh.KeyUp, "down": xh.KeyDown,
	}
}
