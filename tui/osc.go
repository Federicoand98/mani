package tui

import (
	"fmt"
	"os/exec"
	goruntime "runtime"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// hyperlink avvolge label in un link OSC 8: cliccabile nei terminali moderni
// (iTerm2, kitty, wezterm, gnome-terminal, …). Sui terminali che non lo supportano
// degrada mostrando solo la label.
func hyperlink(url, label string) string {
	return fmt.Sprintf("\x1b]8;;%s\x1b\\%s\x1b]8;;\x1b\\", url, label)
}

type clipboardMsg struct{ ok bool }

// copyToClipboard ritorna un Cmd che copia s nella clipboard di sistema (best-effort).
func copyToClipboard(s string) tea.Cmd {
	return func() tea.Msg {
		var candidates [][]string
		switch goruntime.GOOS {
		case "darwin":
			candidates = [][]string{{"pbcopy"}}
		case "windows":
			candidates = [][]string{{"clip"}}
		default: // linux & co.
			candidates = [][]string{
				{"wl-copy"},
				{"xclip", "-selection", "clipboard"},
				{"xsel", "--clipboard", "--input"},
			}
		}

		for _, c := range candidates {
			path, err := exec.LookPath(c[0])
			if err != nil {
				continue
			}
			cmd := exec.Command(path, c[1:]...)
			cmd.Stdin = strings.NewReader(s)
			if cmd.Run() == nil {
				return clipboardMsg{ok: true}
			}
		}
		return clipboardMsg{ok: false}
	}
}
