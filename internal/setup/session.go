package setup

import (
	"github.com/dmtrkzntsv/gosaid/internal/config"
)

// Session is one interactive setup run: the loaded config, its path, and
// whether anything changed. All flows mutate Cfg in memory; nothing is
// written until finish() calls Save.
type Session struct {
	Path  string
	Cfg   *config.Config
	Dirty bool
}

// LoadSession resolves the config path and loads (or creates) the config.
func LoadSession() (*Session, error) {
	path, err := config.Path()
	if err != nil {
		return nil, err
	}
	cfg, err := config.Load(path)
	if err != nil {
		return nil, err
	}
	return &Session{Path: path, Cfg: cfg}, nil
}

// Save validates and writes the config atomically. An invalid config is
// never written.
func (s *Session) Save() error {
	if err := config.Validate(s.Cfg); err != nil {
		return err
	}
	return config.Save(s.Path, s.Cfg)
}
