package inject

/*
#cgo LDFLAGS: -framework ApplicationServices -framework CoreFoundation
#include <ApplicationServices/ApplicationServices.h>

// kVK_ANSI_V = 0x09
// Synthesizes Cmd+V via Quartz Event Services. Returns 0 on success.
static int synth_paste(void) {
    CGEventSourceRef src = CGEventSourceCreate(kCGEventSourceStateHIDSystemState);
    if (!src) return 1;

    CGEventRef down = CGEventCreateKeyboardEvent(src, (CGKeyCode)0x09, true);
    CGEventRef up   = CGEventCreateKeyboardEvent(src, (CGKeyCode)0x09, false);
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
