package cli

import "testing"

func TestRunModelRejectsInterleavedFlagAfterName(t *testing.T) {
	// "model download repo --name x file": the two-phase flag.Parse must not
	// silently treat "--name" as the file positional.
	code := RunModel([]string{"download", "repo", "--name", "x", "file"})
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
}

func TestRunModelRejectsFlagAsFilePositional(t *testing.T) {
	// "model download repo --force": must not treat "--force" as the file
	// positional.
	code := RunModel([]string{"download", "repo", "--force"})
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
}
