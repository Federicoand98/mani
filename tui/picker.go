package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type picker struct {
	title  string
	items  []string
	cursor int
	onPick func(string) tea.Cmd
}

func (p *picker) update(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "up", "k":
		if p.cursor > 0 {
			p.cursor--
		}
	case "down", "j":
		if p.cursor < len(p.items)-1 {
			p.cursor++
		}
	case "enter":
		return p.onPick(p.items[p.cursor])
	}
	return nil
}

func (p picker) view() string {
	var b strings.Builder
	b.WriteString(cyanStyle.Render(p.title) + "\n\n")
	for i, it := range p.items {
		line := "  " + it
		if i == p.cursor {
			line = cyanStyle.Bold(true).Render("  " + it)
		} else {
			line = dimStyle.Render(line)
		}
		b.WriteString(line + "\n")
	}
	b.WriteString(dimStyle.Render("\n(↑/↓ navigate · enter select · esc cancel)"))
	return b.String()
}
