package cli

import "testing"

func TestDispatchMic(t *testing.T) {
	if code := Dispatch("test", []string{"mic", "--help"}); code != 0 {
		t.Fatalf("gosaid mic help exit = %d, want 0", code)
	}
}

func TestDispatchDriverHelp(t *testing.T) {
	if code := Dispatch("test", []string{"driver", "--help"}); code != 0 {
		t.Fatalf("gosaid driver help exit = %d, want 0", code)
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
