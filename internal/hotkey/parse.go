package hotkey

import (
	"fmt"
	"strings"

	xh "golang.design/x/hotkey"
)

// lockingKeys are keys the OS resolves into a persistent state before any
// application sees them. They report a state rather than a press and a
// release, so they can't drive push-to-talk and the OS hotkey APIs won't
// deliver them — worth naming explicitly, since "unknown modifier capslock"
// reads like a typo rather than a limitation.
var lockingKeys = map[string]string{
	"capslock":   "Caps Lock",
	"caps":       "Caps Lock",
	"numlock":    "Num Lock",
	"scrolllock": "Scroll Lock",
	"fn":         "Fn",
}

// LockingKeyError reports a combo rejected for containing a locking key.
// Callers rendering into tight space (a form field) can use Short(); logs and
// config validation get the full explanation from Error().
type LockingKeyError struct {
	Key   string // display name, e.g. "Caps Lock"
	Combo string
}

func (e *LockingKeyError) Short() string {
	return fmt.Sprintf("%s can't be used in a hotkey — the OS handles it as a lock state", e.Key)
}

func (e *LockingKeyError) Error() string {
	return fmt.Sprintf("%s can't be used in a hotkey (%q): the OS turns it into a "+
		"lock state before apps see it, so there's no key press to react to. "+
		"Remap it to a real modifier such as Control (%s) and bind that instead",
		e.Key, e.Combo, remapHint)
}

// Parse converts "ctrl+alt+space" into the modifier bitmap + key recognised
// by golang.design/x/hotkey. The combo must have at least one modifier and
// exactly one non-modifier key (the last segment).
func Parse(combo string) ([]xh.Modifier, xh.Key, error) {
	parts := strings.Split(strings.ToLower(strings.TrimSpace(combo)), "+")
	for _, p := range parts {
		if name, locking := lockingKeys[p]; locking {
			return nil, 0, &LockingKeyError{Key: name, Combo: combo}
		}
	}
	if len(parts) < 2 {
		return nil, 0, fmt.Errorf("combo %q must include at least one modifier and a key", combo)
	}
	var mods []xh.Modifier
	for _, p := range parts[:len(parts)-1] {
		m, ok := modifierMap[p]
		if !ok {
			return nil, 0, fmt.Errorf("unknown modifier %q in combo %q", p, combo)
		}
		mods = append(mods, m)
	}
	key, ok := keyMap[parts[len(parts)-1]]
	if !ok {
		return nil, 0, fmt.Errorf("unknown key %q in combo %q", parts[len(parts)-1], combo)
	}
	return mods, key, nil
}
