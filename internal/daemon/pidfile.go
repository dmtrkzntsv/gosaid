package daemon

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ErrAlreadyRunning is returned when another gosaid instance holds the PID file.
var ErrAlreadyRunning = errors.New("another gosaid instance is already running")

// Acquire writes the current PID to path. If the file already exists and the
// recorded PID is alive, returns ErrAlreadyRunning; otherwise, overwrites.
func Acquire(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if data, err := os.ReadFile(path); err == nil {
		pid, convErr := strconv.Atoi(strings.TrimSpace(string(data)))
		if convErr == nil && pid > 0 && isAlive(pid) {
			return fmt.Errorf("%w (pid %d)", ErrAlreadyRunning, pid)
		}
	}
	return os.WriteFile(path, []byte(strconv.Itoa(os.Getpid())), 0o644)
}

// Release removes the PID file if it belongs to this process.
func Release(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	pid, _ := strconv.Atoi(strings.TrimSpace(string(data)))
	if pid != os.Getpid() {
		return nil
	}
	return os.Remove(path)
}
