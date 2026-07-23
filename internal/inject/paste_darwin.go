package inject

/*
#cgo LDFLAGS: -framework ApplicationServices -framework CoreFoundation
#include <ApplicationServices/ApplicationServices.h>

// kVK_ANSI_V = 0x09, kVK_ANSI_C = 0x08
// Synthesizes Cmd+<key> via Quartz Event Services. Returns 0 on success.
// Flags are set explicitly on the event, so physically-held hotkey
// modifiers (e.g. Option) do not leak into the synthesized keystroke.
static int synth_combo(CGKeyCode key) {
    CGEventSourceRef src = CGEventSourceCreate(kCGEventSourceStateHIDSystemState);
    if (!src) return 1;

    CGEventRef down = CGEventCreateKeyboardEvent(src, key, true);
    CGEventRef up   = CGEventCreateKeyboardEvent(src, key, false);
    if (!down || !up) {
        if (down) CFRelease(down);
        if (up)   CFRelease(up);
        CFRelease(src);
        return 2;
    }
    CGEventSetFlags(down, kCGEventFlagMaskCommand);
    CGEventSetFlags(up,   kCGEventFlagMaskCommand);
    CGEventPost(kCGHIDEventTap, down);
    CGEventPost(kCGHIDEventTap, up);

    CFRelease(down);
    CFRelease(up);
    CFRelease(src);
    return 0;
}

static int synth_paste(void) { return synth_combo((CGKeyCode)0x09); }
static int synth_copy(void)  { return synth_combo((CGKeyCode)0x08); }
*/
import "C"

import "fmt"

func synthesizePaste() error {
	code := C.synth_paste()
	if code != 0 {
		return fmt.Errorf("CGEvent paste synthesis failed (code %d) — grant gosaid Accessibility permission in System Settings", code)
	}
	return nil
}

func synthesizeCopy() error {
	code := C.synth_copy()
	if code != 0 {
		return fmt.Errorf("CGEvent copy synthesis failed (code %d) — grant gosaid Accessibility permission in System Settings", code)
	}
	return nil
}
