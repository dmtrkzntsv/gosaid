package daemon

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestPIDFileAcquireAndRelease(t *testing.T) {
	p := filepath.Join(t.TempDir(), "gosaid.pid")

	if err := Acquire(p); err != nil {
		t.Fatalf("acquire: %v", err)
	}
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read pid: %v", err)
	}
	if pid, _ := strconv.Atoi(string(data)); pid != os.Getpid() {
		t.Fatalf("pid file = %q, want %d", data, os.Getpid())
	}
	if err := Release(p); err != nil {
		t.Fatalf("release: %v", err)
	}
	if _, err := os.Stat(p); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("pid file still present after release: %v", err)
	}
}

func TestPIDFileStaleOverwrite(t *testing.T) {
	p := filepath.Join(t.TempDir(), "gosaid.pid")
	// Unused, impossible PID — should be treated as stale and overwritten.
	if err := os.WriteFile(p, []byte("999999"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Acquire(p); err != nil {
		t.Fatalf("acquire over stale: %v", err)
	}
	data, _ := os.ReadFile(p)
	if pid, _ := strconv.Atoi(string(data)); pid != os.Getpid() {
		t.Fatalf("stale pid not replaced; got %q", data)
	}
}

func TestPIDFileAlreadyRunning(t *testing.T) {
	p := filepath.Join(t.TempDir(), "gosaid.pid")
	// Our own PID is obviously alive.
	if err := os.WriteFile(p, []byte(strconv.Itoa(os.Getpid())), 0o644); err != nil {
		t.Fatal(err)
	}
	err := Acquire(p)
	if !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("expected ErrAlreadyRunning, got %v", err)
	}
}
