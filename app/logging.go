package app

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/Federicoand98/mani/config"
)

// SetupLogging imposta lo slog di default: livello da `level`, output su
// ~/.config/mani/mani.log (così non sporca la TUI). Seguilo con `tail -f` su quel file.
// Se il file non è apribile, ricade su stderr.
func SetupLogging(level string) {
	var w io.Writer = os.Stderr
	path := filepath.Join(config.ConfigDir(), "mani.log")
	if f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644); err == nil {
		w = f
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: parseLevel(level)})))
}

func parseLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
