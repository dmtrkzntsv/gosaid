package setup

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestDaemonRunningAt(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "state.json")
	if daemonRunningAt(missing) {
		t.Fatal("missing state file = not running")
	}

	garbage := filepath.Join(dir, "garbage.json")
	os.WriteFile(garbage, []byte("{not json"), 0o644)
	if daemonRunningAt(garbage) {
		t.Fatal("unparseable state file = not running")
	}

	// Our own test process pid is definitely alive.
	alive := filepath.Join(dir, "alive.json")
	os.WriteFile(alive, []byte(fmt.Sprintf(`{"state":"idle","pid":%d}`, os.Getpid())), 0o644)
	if !daemonRunningAt(alive) {
		t.Fatal("state file with a live pid = running")
	}

	// A stale file with an almost-certainly-dead pid.
	dead := filepath.Join(dir, "dead.json")
	os.WriteFile(dead, []byte(`{"state":"idle","pid":99999999}`), 0o644)
	if daemonRunningAt(dead) {
		t.Fatal("dead pid = not running (crash left a stale file)")
	}
}
