package cli

import "testing"

func TestRunMicHelp(t *testing.T) {
	if code := RunMic([]string{"--help"}); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
}

func TestRunMicRejectsUnknownCommand(t *testing.T) {
	if code := RunMic([]string{"bogus"}); code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
}
