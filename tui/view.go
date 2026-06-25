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

		if p.Preview != "" {
			b.WriteString("\n")
			b.WriteString(renderDiffColored(p.Preview))
			b.WriteString("\n")
		}

		b.WriteString(fmt.Sprintf("[permission] %s (risk: %s) - [y]once / [n]no / [a]always", p.ToolName, p.RiskLevel))

	case stateRunning:
		b.WriteString(m.spinner.View() + " thinking...")

	case stateIdle:
		// command palette
		matches := m.matchingCommands()
		if len(matches) > 0 {
			idx := m.paletteIndex
			if idx >= len(matches) {
				idx = 0
			}

			for i, c := range matches {
				if i == idx {
					b.WriteString(cyanStyle.Render("> "+c.Name) + "	" + dimStyle.Render(c.Description) + "\n")
				} else {
					b.WriteString("  " + c.Name + "	" + dimStyle.Render(c.Description) + "\n")
				}
			}
		}

		b.WriteString(promptStyle.Width(m.viewport.Width).Render(m.input.View()))

		b.WriteString("\n")
		b.WriteString(cyanStyle.Render(m.runtime.Provider() + " · " + m.runtime.ModelName()))

		if m.lastInput > 0 {
			limit := m.runtime.ContextLimit()
			pct := 0
			if limit > 0 {
				pct = m.lastInput * 100 / limit
			}

			b.WriteString(dimStyle.Render(fmt.Sprintf("   [%d/%d (%d%%)] out %d tokens", m.lastInput, limit, pct, m.lastOutput)))
		}

	case stateChoosing:
		return m.picker.view()

	case stateLogin:
		if m.loginTarget == "copilot" {
			return m.output
		}
		return "Login " + m.loginTarget + "\n\n" + m.input.View()
	}

	return b.String()
}

func renderDiffColored(diff string) string {
	var b strings.Builder
	for _, line := range strings.Split(diff, "\n") {
		switch {
		case strings.HasPrefix(line, "+"):
			b.WriteString(addStyle.Render(line))
		case strings.HasPrefix(line, "-"):
			b.WriteString(delStyle.Render(line))
		default:
			b.WriteString(line)
		}
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}
