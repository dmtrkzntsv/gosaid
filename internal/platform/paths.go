package platform

import (
	"os"
	"path/filepath"
)

const appName = "gosaid"

// ConfigDir returns the platform-specific config directory for gosaid.
// macOS:  ~/Library/Application Support/gosaid
// Linux:  $XDG_CONFIG_HOME/gosaid or ~/.config/gosaid
// Win:    %AppData%\gosaid
func ConfigDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, appName), nil
}

// ConfigFile is the absolute path of the config.json file.
func ConfigFile() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

// LogDir returns the platform-specific directory for log files.
func LogDir() (string, error) {
	if v := os.Getenv("XDG_STATE_HOME"); v != "" {
		return filepath.Join(v, appName), nil
	}
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, appName), nil
}

// PIDFile returns the absolute path of the daemon PID file.
func PIDFile() (string, error) {
	dir, err := LogDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "gosaid.pid"), nil
}
