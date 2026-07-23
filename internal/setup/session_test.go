package setup

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dmtrkzntsv/gosaid/internal/config"
)

func TestSessionSaveValidates(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	s := &Session{Path: path, Cfg: &config.Config{}, Dirty: true}
	if err := s.Save(); err == nil {
		t.Fatal("saving an invalid (empty) config must fail validation")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("invalid config must not be written to disk")
	}
}
