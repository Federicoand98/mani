package tui

import (
	"fmt"
	"strings"
)

func (m Model) View() string {
	if !m.ready {
		return "loading..."
	}

	var b strings.Builder
	b.WriteString(m.viewport.View())
	b.WriteString("\n")

	switch m.state {
	case stateAvaitingPermission:
		p := m.pending
		b.WriteString(toolStyle.Render(fmt.Sprintf("[tool: %s]\n", p.ToolName)))

		if cmd, ok := p.Input["command"].(string); ok {
			b.WriteString(cyanStyle.Render("$ " + cmd))
			b.WriteString("\n")
		} else if path, ok := p.Input["path"].(string); ok {
			b.WriteString(cyanStyle.Render("path: " + path))
			b.WriteString("\n")
		}

		b.WriteString(fmt.Sprintf("[permission] %s (risk: %s) - [y]once / [n]no / [a]always", p.ToolName, p.RiskLevel))

	case stateRunning:
		b.WriteString(m.spinner.View() + " thinking...")

	case stateIdle:
		b.WriteString(m.input.View())
	}

	return b.String()
}
