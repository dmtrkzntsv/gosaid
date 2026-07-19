package inject

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
)

func synthesizePaste() error { return synthesizeCombo("v", "47") } // KEY_V = 47
func synthesizeCopy() error  { return synthesizeCombo("c", "46") } // KEY_C = 46

// synthesizeCombo emits Ctrl+<key>, trying the most likely tool present.
// Wayland: wtype (native), then ydotool (requires daemon+uinput).
// X11:     xdotool, ydotool.
// xkey is the X keysym name; ycode the Linux input event code for ydotool.
func synthesizeCombo(xkey, ycode string) error {
	isWayland := os.Getenv("WAYLAND_DISPLAY") != ""
	candidates := []injectCmd{
		{"xdotool", []string{"key", "--clearmodifiers", "ctrl+" + xkey}},
		{"ydotool", []string{"key", "29:1", ycode + ":1", ycode + ":0", "29:0"}}, // 29 = KEY_LEFTCTRL
	}
	if isWayland {
		candidates = []injectCmd{
			{"wtype", []string{"-M", "ctrl", xkey, "-m", "ctrl"}},
			{"ydotool", []string{"key", "29:1", ycode + ":1", ycode + ":0", "29:0"}},
		}
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
	return fmt.Errorf("keystroke synthesis failed: %w — install one of: wtype, xdotool, ydotool", lastErr)
}

type injectCmd struct {
	bin  string
	args []string
}
