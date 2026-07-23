//go:build windows

package setup

import "os"

// pidAlive on Windows: FindProcess only succeeds for live processes.
func pidAlive(pid int) bool {
	_, err := os.FindProcess(pid)
	return err == nil
}
