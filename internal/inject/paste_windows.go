package inject

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	vkControl    = 0x11
	vkV          = 0x56
	inputKeyboard = 1
	keyEventKeyUp = 0x0002
)

type keyboardInput struct {
	wVk         uint16
	wScan       uint16
	dwFlags     uint32
	time        uint32
	dwExtraInfo uintptr
}

type input struct {
	inputType uint32
	_         uint32 // 4-byte pad to align the union on 64-bit
	ki        keyboardInput
	_         [8]byte // union tail padding
}

var (
	user32    = windows.NewLazySystemDLL("user32.dll")
	sendInput = user32.NewProc("SendInput")
)

func synthesizePaste() error {
	events := []input{
		{inputType: inputKeyboard, ki: keyboardInput{wVk: vkControl}},                    // ctrl down
		{inputType: inputKeyboard, ki: keyboardInput{wVk: vkV}},                          // v    down
		{inputType: inputKeyboard, ki: keyboardInput{wVk: vkV, dwFlags: keyEventKeyUp}},  // v    up
		{inputType: inputKeyboard, ki: keyboardInput{wVk: vkControl, dwFlags: keyEventKeyUp}}, // ctrl up
	}
	r, _, err := sendInput.Call(
		uintptr(len(events)),
		uintptr(unsafe.Pointer(&events[0])),
		unsafe.Sizeof(events[0]),
	)
	if int(r) != len(events) {
		return fmt.Errorf("SendInput sent %d of %d events: %v", r, len(events), err)
	}
	return nil
}
