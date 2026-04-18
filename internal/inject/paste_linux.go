package inject

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
)

// synthesizePaste tries, in order, the most likely tool present on the system.
// Wayland: wtype (native), then ydotool (requires daemon+uinput).
// X11:     xdotool, ydotool.
func synthesizePaste() error {
	isWayland := os.Getenv("WAYLAND_DISPLAY") != ""
	candidates := xdotoolCandidates()
	if isWayland {
		candidates = waylandCandidates()
	}
	var lastErr error
	for _, c := range candidates {
		if _, err := exec.LookPath(c.bin); err != nil {
			lastErr = errors.Join(lastErr, err)
			continue
		}
		if err := exec.Command(c.bin, c.args...).Run(); err != nil {
			lastErr = errors.Join(lastErr, fmt.Errorf("%s: %w", c.bin, err))
			continue
		}
		return nil
	}
	if lastErr == nil {
		lastErr = errors.New("no keystroke synthesis tool available")
	}
	return fmt.Errorf("paste synthesis failed: %w — install one of: wtype, xdotool, ydotool", lastErr)
}

type injectCmd struct {
	bin  string
	args []string
}

func waylandCandidates() []injectCmd {
	return []injectCmd{
		{"wtype", []string{"-M", "ctrl", "v", "-m", "ctrl"}},
		{"ydotool", []string{"key", "29:1", "47:1", "47:0", "29:0"}}, // ctrl+v
	}
}

func xdotoolCandidates() []injectCmd {
	return []injectCmd{
		{"xdotool", []string{"key", "--clearmodifiers", "ctrl+v"}},
		{"ydotool", []string{"key", "29:1", "47:1", "47:0", "29:0"}},
	}
}
