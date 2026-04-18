//go:build windows

package daemon

import "os"

// On Windows, os.FindProcess always succeeds regardless of liveness. As a
// best-effort check, treat "process handle obtainable" as alive. False
// positives only delay the stale-file overwrite by one startup attempt.
func isAlive(pid int) bool {
	_, err := os.FindProcess(pid)
	return err == nil
}
