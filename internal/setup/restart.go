package setup

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"

	"charm.land/huh/v2"

	"github.com/dmtrkzntsv/gosaid/internal/daemon"
)

// daemonRunningAt reports whether the state file at path describes a live
// daemon. A missing/garbled file or a dead PID (crash leftover) means no.
func daemonRunningAt(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var st daemon.StateFile
	if json.Unmarshal(data, &st) != nil {
		return false
	}
	return st.PID > 0 && pidAlive(st.PID)
}

func daemonRunning() bool {
	path, err := daemon.StateFilePath()
	if err != nil {
		return false
	}
	return daemonRunningAt(path)
}

func restartHint() string {
	switch runtime.GOOS {
	case "windows":
		return "stop the running gosaid.exe and start it again"
	default:
		return "brew services restart gosaid  (or restart your gosaid process)"
	}
}

func startHint() string {
	switch runtime.GOOS {
	case "windows":
		return "gosaid.exe"
	default:
		return "brew services start gosaid  (or run `gosaid` directly)"
	}
}

// offerRestart is called after a successful save. With a running daemon it
// offers an automatic restart (brew when available); otherwise it prints how
// to start one.
func offerRestart() {
	if !daemonRunning() {
		fmt.Println("The daemon is not running. Start it with:\n  " + startHint())
		return
	}
	restart := true
	err := huh.NewForm(huh.NewGroup(
		huh.NewConfirm().
			Title("Restart the daemon to apply changes?").
			Value(&restart),
	)).Run()
	if err != nil || !restart {
		fmt.Println("Restart the daemon later to apply changes:\n  " + restartHint())
		return
	}
	if brew, lerr := exec.LookPath("brew"); lerr == nil {
		cmd := exec.Command(brew, "services", "restart", "gosaid")
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if cmd.Run() == nil {
			fmt.Println("Daemon restarted.")
			return
		}
	}
	fmt.Println("Could not restart automatically. Restart manually:\n  " + restartHint())
}
