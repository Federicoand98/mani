package app

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/Federicoand98/mani/config"
)

// SetupLogging imposta lo slog di default al livello `level`. Con toStderr=true scrive su
// stderr (comandi headless run/serve: i log si vedono nel terminale, l'output vero resta su
// stdout). Con toStderr=false scrive su ~/.config/mani/mani.log (TUI: non sporca lo schermo;
// seguilo con `tail -f`). Se il file non è apribile, ricade su stderr.
func SetupLogging(level string, toStderr bool) {
	var w io.Writer = os.Stderr
	if !toStderr {
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
