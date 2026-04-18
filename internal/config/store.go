package config

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/dmtrkzntsv/gosaid/internal/platform"
)

// config.example.json is the default config shipped inside the binary and
// written to disk on first run.
//
//go:embed config.example.json
var exampleConfig []byte

// Path returns the resolved config file path.
func Path() (string, error) {
	return platform.ConfigFile()
}

// Load reads the config from disk. If the file does not exist, the embedded
// example config is written atomically and returned. The returned config is
// NOT validated — call Validate() explicitly.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		if err := writeExample(path); err != nil {
			return nil, fmt.Errorf("write default config: %w", err)
		}
		data = exampleConfig
	} else if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	return &cfg, nil
}

// Save writes the config to disk atomically (tmp + rename).
func Save(path string, cfg *Config) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(path, data)
}

func writeExample(path string) error {
	return writeAtomic(path, exampleConfig)
}

func writeAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "config-*.json.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
