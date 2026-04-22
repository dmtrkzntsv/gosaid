package daemon

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/dmtrkzntsv/gosaid/internal/platform"
)

// state.json is the daemon's live status artifact, read by external tools
// (primarily the desktop UI) to know what the daemon is doing right now
// and to locate it. It's the runtime counterpart to config.json: one is
// user-authored input, the other is daemon-authored output. No IPC, no
// sockets — files only, matching the project's UI/daemon contract.
//
// The file is written atomically (temp+rename) on every change so readers
// using fsnotify/FSEvents see single rename events rather than torn writes.
// On graceful shutdown the file is unlinked; a crash leaves it stale, which
// readers disambiguate via the PID field (dead pid → treat as idle).

// StateFileInjection mirrors the JSON shape for the last injection record.
// Text is stored in full (no size cap) so the UI can offer a "Copy Last
// Text" action — the dictation content sits in the same directory as the
// plaintext API keys in config.json, which is consistent with the v1
// security posture (documented in the desktop CLAUDE.md).
type StateFileInjection struct {
	OK    bool      `json:"ok"`
	TS    time.Time `json:"ts"`
	Text  string    `json:"text"`
	Error string    `json:"error,omitempty"`
}

// StateFile is the on-disk payload. Keep the JSON shape stable — the
// desktop UI parses it directly.
type StateFile struct {
	State         string              `json:"state"`
	StateTS       time.Time           `json:"state_ts"`
	PID           int                 `json:"pid"`
	DaemonBinary  string              `json:"daemon_binary,omitempty"`
	LastInjection *StateFileInjection `json:"last_injection,omitempty"`
}

// Package-scoped mutable state. The daemon has a single state file per
// process — a singleton matches that, and keeps the call sites in daemon.go
// and pipeline.go trivial (no plumbing through Pipeline/Core).
var (
	stateMu   sync.Mutex
	stateCur  StateFile
	statePath string
)

// StateFilePath resolves the on-disk location. Co-located with config.json
// so readers (the UI) can derive it from the same ConfigDir they already
// resolve for config.json. Not in LogDir/PIDFile territory, which is
// intentionally cache-like and cleanable.
func StateFilePath() (string, error) {
	dir, err := platform.ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "state.json"), nil
}

// InitStateFile sets up the singleton and writes the initial Idle snapshot
// with pid + daemonBinary. Must be called once, early in daemon startup,
// before any UpdateState/RecordInjection calls.
func InitStateFile(pid int, daemonBinary string) error {
	path, err := StateFilePath()
	if err != nil {
		return err
	}
	stateMu.Lock()
	defer stateMu.Unlock()
	statePath = path
	stateCur = StateFile{
		State:        StateIdle.String(),
		StateTS:      time.Now().UTC(),
		PID:          pid,
		DaemonBinary: daemonBinary,
	}
	return writeStateFileLocked()
}

// UpdateState persists a new daemon state. Safe to call from any goroutine;
// pairs with the existing state-bus subscriber in daemon.go.
func UpdateState(s State) error {
	stateMu.Lock()
	defer stateMu.Unlock()
	if statePath == "" {
		return nil // init not called — silently no-op; not fatal.
	}
	stateCur.State = s.String()
	stateCur.StateTS = time.Now().UTC()
	return writeStateFileLocked()
}

// RecordInjection captures the text and outcome of the most recent inject
// attempt. Call once before Inject (ok=false) and again after (ok=true on
// success, ok=false with errStr on failure). Text is always the final
// content we attempted to inject, so a Copy Last Text affordance works
// even when the paste itself failed.
func RecordInjection(text string, ok bool, errStr string) error {
	stateMu.Lock()
	defer stateMu.Unlock()
	if statePath == "" {
		return nil
	}
	stateCur.LastInjection = &StateFileInjection{
		OK:    ok,
		TS:    time.Now().UTC(),
		Text:  text,
		Error: errStr,
	}
	return writeStateFileLocked()
}

// ClearStateFile removes the file on graceful shutdown. A missing file is
// the agreed "daemon not running" signal to readers.
func ClearStateFile() error {
	stateMu.Lock()
	defer stateMu.Unlock()
	if statePath == "" {
		return nil
	}
	err := os.Remove(statePath)
	statePath = ""
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func writeStateFileLocked() error {
	data, err := json.MarshalIndent(stateCur, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(statePath), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(statePath), "state-*.json.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	// If anything below fails, drop the leftover temp file. Safe when
	// Rename succeeds too — Remove on the now-nonexistent path is a no-op.
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write state tmp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close state tmp: %w", err)
	}
	return os.Rename(tmpName, statePath)
}
