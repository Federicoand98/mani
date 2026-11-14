package workspace

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Native dialogs are tied to the HTTP request and close after two minutes.
const nativeFolderTimeout = 2 * time.Minute

var errFolderUnavailable = errors.New("native folder selection unavailable")

func (s *Server) handlePickFolder(w http.ResponseWriter, r *http.Request) {
	var in NativeFolderRequest
	if err := readJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "bad request body: "+err.Error())
		return
	}
	start := in.Path
	if !filepath.IsAbs(start) {
		start = filepath.Join(s.opt.Dir, start)
	}
	if err := validateNativeFolder(start); err != nil {
		writeError(w, http.StatusBadRequest, "starting folder: "+err.Error())
		return
	}
	if !s.folderMu.TryLock() {
		writeError(w, http.StatusConflict, "a native folder dialog is already open")
		return
	}
	defer s.folderMu.Unlock()

	ctx, cancel := context.WithTimeout(r.Context(), nativeFolderTimeout)
	defer cancel()
	stop := context.AfterFunc(s.chats.ctx, cancel)
	defer stop()
	path, cancelled, err := s.pickFolder(ctx, start)
	if ctx.Err() != nil {
		status := http.StatusRequestTimeout
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			status = http.StatusGatewayTimeout
		}
		writeError(w, status, "native folder dialog: "+ctx.Err().Error())
		return
	}
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, errFolderUnavailable) {
			status = http.StatusServiceUnavailable
		} else if errors.Is(err, context.DeadlineExceeded) {
			status = http.StatusGatewayTimeout
		}
		writeError(w, status, err.Error())
		return
	}
	if cancelled {
		writeJSON(w, http.StatusOK, NativeFolderResult{Cancelled: true})
		return
	}
	if err := validateNativeFolder(path); err != nil {
		writeError(w, http.StatusInternalServerError, "selected folder: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, NativeFolderResult{Path: filepath.Clean(path)})
}

func validateNativeFolder(path string) error {
	if !filepath.IsAbs(path) {
		return errors.New("path must be absolute")
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("cannot open directory: %w", err)
	}
	if !info.IsDir() {
		return errors.New("path is not a directory")
	}
	return nil
}

// Dependencies are local to the picker so tests never open a desktop dialog.
type nativeFolderPicker struct {
	goos     string
	getenv   func(string) string
	lookPath func(string) (string, error)
	output   func(*exec.Cmd) ([]byte, error)
}

func desktopFolderPicker() nativeFolderPicker {
	return nativeFolderPicker{
		goos: runtime.GOOS, getenv: os.Getenv, lookPath: exec.LookPath,
		output: func(cmd *exec.Cmd) ([]byte, error) { return cmd.Output() },
	}
}

func (p nativeFolderPicker) pick(ctx context.Context, start string) (string, bool, error) {
	cmd, err := p.command(ctx, start)
	if err != nil {
		return "", false, err
	}
	cmd.WaitDelay = time.Second
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := p.output(cmd)
	if ctx.Err() != nil {
		return "", false, ctx.Err()
	}
	if err != nil {
		detail := strings.TrimSpace(stderr.String())
		if errors.Is(err, exec.ErrNotFound) || errors.Is(err, os.ErrNotExist) {
			return "", false, fmt.Errorf("%w: %s: %v", errFolderUnavailable, filepath.Base(cmd.Path), err)
		}
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			// Linux pickers reserve exit 1 for Cancel. Diagnostics mean a failure,
			// not cancellation (e.g. the display disappeared before launch).
			if p.goos == "linux" && exit.ExitCode() == 1 && detail == "" && len(out) == 0 {
				return "", true, nil
			}
			if p.goos == "windows" && exit.ExitCode() == 3 {
				return "", false, fmt.Errorf("%w: no interactive Windows desktop", errFolderUnavailable)
			}
		}
		lower := strings.ToLower(detail)
		if (p.goos == "linux" && (strings.Contains(lower, "cannot open display") || strings.Contains(lower, "could not connect to display"))) ||
			(p.goos == "darwin" && (strings.Contains(detail, "(-600)") || strings.Contains(detail, "(-10810)"))) {
			return "", false, fmt.Errorf("%w: %s", errFolderUnavailable, detail)
		}
		return "", false, fmt.Errorf("native folder dialog (%s): %w: %s", filepath.Base(cmd.Path), err, detail)
	}
	// Remove only the transport newline: spaces are valid in directory names.
	path := strings.TrimSuffix(strings.TrimSuffix(string(out), "\n"), "\r")
	if path == "" {
		if p.goos == "darwin" || p.goos == "windows" {
			return "", true, nil
		}
		return "", false, errors.New("native folder dialog returned an empty path")
	}
	return path, false, nil
}

func (p nativeFolderPicker) command(ctx context.Context, start string) (*exec.Cmd, error) {
	makeCmd := func(name string, args ...string) (*exec.Cmd, error) {
		path, err := p.lookPath(name)
		if err != nil {
			return nil, fmt.Errorf("%w: %s is required", errFolderUnavailable, name)
		}
		return exec.CommandContext(ctx, path, args...), nil
	}
	switch p.goos {
	case "linux":
		if p.getenv("DISPLAY") == "" && p.getenv("WAYLAND_DISPLAY") == "" {
			return nil, fmt.Errorf("%w: no desktop display (DISPLAY or WAYLAND_DISPLAY)", errFolderUnavailable)
		}
		if _, err := p.lookPath("zenity"); err == nil {
			return makeCmd("zenity", "--file-selection", "--directory", "--title=Choose agent workspace", "--filename="+start+string(os.PathSeparator))
		}
		if _, err := p.lookPath("kdialog"); err == nil {
			return makeCmd("kdialog", "--getexistingdirectory", start, "--title", "Choose agent workspace")
		}
		return nil, fmt.Errorf("%w: install zenity or kdialog on the desktop running mani", errFolderUnavailable)
	case "darwin":
		return makeCmd("osascript", "-e", nativeFolderAppleScript, "--", start)
	case "windows":
		cmd, err := makeCmd("powershell.exe", "-NoProfile", "-NonInteractive", "-STA", "-Command", nativeFolderPowerShell)
		if err == nil {
			cmd.Env = append(os.Environ(), "MANI_FOLDER_START="+start)
		}
		return cmd, err
	default:
		return nil, fmt.Errorf("%w: unsupported operating system %s", errFolderUnavailable, p.goos)
	}
}

const nativeFolderAppleScript = `on run argv
	try
		set selectedFolder to choose folder with prompt "Choose agent workspace" default location (POSIX file (item 1 of argv))
		return POSIX path of selectedFolder
	on error messageText number errorNumber
		if errorNumber is -128 then return ""
		error messageText number errorNumber
	end try
end run`

const nativeFolderPowerShell = `$ErrorActionPreference = 'Stop'
if (-not [Environment]::UserInteractive) { exit 3 }
try {
	Add-Type -AssemblyName System.Windows.Forms
	[Console]::OutputEncoding = New-Object System.Text.UTF8Encoding($false)
	$dialog = New-Object System.Windows.Forms.FolderBrowserDialog
	try {
		$dialog.Description = 'Choose agent workspace'
		$dialog.SelectedPath = $env:MANI_FOLDER_START
		$dialog.ShowNewFolderButton = $false
		$result = $dialog.ShowDialog()
		if ($result -eq [System.Windows.Forms.DialogResult]::OK) {
			[Console]::WriteLine($dialog.SelectedPath)
		} elseif ($result -ne [System.Windows.Forms.DialogResult]::Cancel) {
			throw "Unexpected folder dialog result: $result"
		}
	} finally { $dialog.Dispose() }
} catch {
	[Console]::Error.WriteLine($_.Exception.Message)
	exit 2
}`
