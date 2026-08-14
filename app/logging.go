package app

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/Federicoand98/mani/config"
)

// SetupLogging imposta lo slog di default al livello `level`. `dest` decide dove finiscono i log:
//
//	"stderr"  → terminale (serve, o run --verbose)
//	"discard" → nessun log (run silenzioso di default: l'output vero resta su stdout)
//	"file"    → ~/.config/mani/mani.log (TUI: non sporca lo schermo; seguilo con `tail -f`)
func SetupLogging(level, dest string) {
	var w io.Writer
	switch dest {
	case "stderr":
		w = os.Stderr
	case "discard":
		w = io.Discard
	default: // "file"
		w = os.Stderr
		path := filepath.Join(config.ConfigDir(), "mani.log")
		if f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644); err == nil {
			w = f
		}
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
