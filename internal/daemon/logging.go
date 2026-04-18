package daemon

import (
	"log/slog"
	"os"
	"strings"
)

// InitLogger returns a slog.Logger honoring the level from config.
// Output goes to stderr in text form — there is no log file in v1; the
// user runs gosaid from a service manager that already captures stdio.
func InitLogger(level string) *slog.Logger {
	lvl := slog.LevelInfo
	switch strings.ToLower(level) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn", "warning":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	}
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: lvl}))
}
