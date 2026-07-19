package cli

import "testing"

func TestDispatchMicRemoved(t *testing.T) {
	if code := Dispatch("test", []string{"mic", "list"}); code != 2 {
		t.Fatalf("gosaid mic must be an unknown command now, exit = %d", code)
	}
}

func TestDispatchSetupUnknownTopic(t *testing.T) {
	if code := Dispatch("test", []string{"setup", "bogus"}); code != 2 {
		t.Fatalf("unknown setup topic must exit 2, got %d", code)
	}
}

func TestDispatchSetupNonTTY(t *testing.T) {
	// go test runs with a non-TTY stdin, so any valid topic must refuse.
	if code := Dispatch("test", []string{"setup", "mic"}); code != 1 {
		t.Fatalf("setup without a TTY must exit 1, got %d", code)
	}
}
