package hotkey

import (
	"fmt"
	"strings"

	xh "golang.design/x/hotkey"
)

// Parse converts "ctrl+alt+space" into the modifier bitmap + key recognised
// by golang.design/x/hotkey. The combo must have at least one modifier and
// exactly one non-modifier key (the last segment).
func Parse(combo string) ([]xh.Modifier, xh.Key, error) {
	parts := strings.Split(strings.ToLower(strings.TrimSpace(combo)), "+")
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
