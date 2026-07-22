package cli

import "testing"

func TestDispatchMicRemoved(t *testing.T) {
	if code := Dispatch("test", []string{"mic", "list"}); code != 2 {
		t.Fatalf("gosaid mic must be an unknown command now, exit = %d", code)
	}
}

func TestDispatchSetupRejectsArgs(t *testing.T) {
	// setup takes no arguments now — any arg is exit 2, before the TTY check.
	if code := Dispatch("test", []string{"setup", "mic"}); code != 2 {
		t.Fatalf("setup with an arg must exit 2, got %d", code)
	}
	if code := Dispatch("test", []string{"setup", "bogus"}); code != 2 {
		t.Fatalf("setup with an arg must exit 2, got %d", code)
	}
}

func TestDispatchSetupNonTTY(t *testing.T) {
	// Bare `setup` in a non-TTY (go test) must exit 1.
	if code := Dispatch("test", []string{"setup"}); code != 1 {
		t.Fatalf("bare setup without a TTY must exit 1, got %d", code)
	}
}
