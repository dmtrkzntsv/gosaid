package daemon

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// redirectStateFile forces the package-scoped statePath into a test dir
// so tests don't touch the real ~/Library/Application Support/gosaid.
// Returns the file path.
func redirectStateFile(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	stateMu.Lock()
	statePath = path
	stateCur = StateFile{}
	stateMu.Unlock()
	t.Cleanup(func() {
		stateMu.Lock()
		statePath = ""
		stateCur = StateFile{}
		stateMu.Unlock()
	})
	return path
}

func readStateFile(t *testing.T, path string) StateFile {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read state file: %v", err)
	}
	var s StateFile
	if err := json.Unmarshal(data, &s); err != nil {
		t.Fatalf("decode state file: %v", err)
	}
	return s
}

func TestStateFileUpdateAndInjection(t *testing.T) {
	path := redirectStateFile(t)

	// Seed with the Idle+pid payload that InitStateFile would normally
	// produce; call writeStateFileLocked directly to avoid re-resolving
	// the real path via platform.ConfigDir.
	stateMu.Lock()
	stateCur = StateFile{State: StateIdle.String(), PID: 4242, DaemonBinary: "/tmp/gosaid"}
	if err := writeStateFileLocked(); err != nil {
		stateMu.Unlock()
		t.Fatalf("seed write: %v", err)
	}
	stateMu.Unlock()

	if err := UpdateState(StateRecording); err != nil {
		t.Fatalf("UpdateState: %v", err)
	}
	s := readStateFile(t, path)
	if s.State != "Recording" {
		t.Fatalf("state: got %q, want Recording", s.State)
	}
	if s.PID != 4242 {
		t.Fatalf("pid: got %d, want 4242", s.PID)
	}
	if s.DaemonBinary != "/tmp/gosaid" {
		t.Fatalf("daemon_binary: got %q, want /tmp/gosaid", s.DaemonBinary)
	}

	// Record an injection and make sure it rides along with the latest state.
	if err := RecordInjection("hello world", false, "paste failed"); err != nil {
		t.Fatalf("RecordInjection: %v", err)
	}
	if err := UpdateState(StateIdle); err != nil {
		t.Fatalf("UpdateState idle: %v", err)
	}
	s = readStateFile(t, path)
	if s.State != "Idle" {
		t.Fatalf("state: got %q, want Idle", s.State)
	}
	if s.LastInjection == nil {
		t.Fatal("last_injection: got nil, want populated record")
	}
	if s.LastInjection.Text != "hello world" {
		t.Fatalf("last_injection.text: got %q, want hello world", s.LastInjection.Text)
	}
	if s.LastInjection.OK {
		t.Fatal("last_injection.ok: got true, want false")
	}
	if s.LastInjection.Error != "paste failed" {
		t.Fatalf("last_injection.error: got %q, want paste failed", s.LastInjection.Error)
	}
}

// Hammer UpdateState + RecordInjection concurrently; mutex should prevent
// torn writes or undefined interleavings.
func TestStateFileConcurrentWrites(t *testing.T) {
	path := redirectStateFile(t)
	stateMu.Lock()
	stateCur = StateFile{State: StateIdle.String(), PID: 1}
	if err := writeStateFileLocked(); err != nil {
		stateMu.Unlock()
		t.Fatalf("seed: %v", err)
	}
	stateMu.Unlock()

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); _ = UpdateState(StateRecording) }()
		go func() { defer wg.Done(); _ = RecordInjection("x", true, "") }()
	}
	wg.Wait()

	// Final read should decode cleanly (no half-written files, no junk).
	_ = readStateFile(t, path)
}

func TestStateFileClear(t *testing.T) {
	path := redirectStateFile(t)
	stateMu.Lock()
	stateCur = StateFile{State: StateIdle.String(), PID: 1}
	if err := writeStateFileLocked(); err != nil {
		stateMu.Unlock()
		t.Fatalf("seed: %v", err)
	}
	stateMu.Unlock()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected file to exist: %v", err)
	}
	if err := ClearStateFile(); err != nil {
		t.Fatalf("ClearStateFile: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected file to be gone, got err=%v", err)
	}
	// Calling Clear again is a no-op.
	if err := ClearStateFile(); err != nil {
		t.Fatalf("ClearStateFile second call: %v", err)
	}
}
