package tui

import (
	"context"

	"github.com/Federicoand98/mani/app"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		h := msg.Height - 3
		if !m.ready {
			m.viewport = viewport.New(msg.Width, h)
			m.ready = true
		} else {
			m.viewport.Width = msg.Width
			m.viewport.Height = h
		}
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case eventMsg:
		return m.handleEvent(app.Event(msg))

	case streamClosedMsg:
		m.state = stateIdle
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.Type == tea.KeyCtrlC {
		return m, tea.Quit
	}

	if m.state == stateAvaitingPermission {
		switch msg.String() {
		case "y":
			m.pending.Respond <- app.AllowOnce
		case "a":
			m.pending.Respond <- app.AllowAlways
		case "n", "esc":
			m.pending.Respond <- app.Deny
		default:
			return m, nil
		}

		m.pending = nil
		m.state = stateRunning
		return m, waitForEvent(m.events)
	}

	if msg.Type == tea.KeyEnter && m.state == stateIdle {
		text := m.input.Value()

		if text == "" {
			return m, nil
		}

		m.input.Reset()
		m.output.WriteString("\n> " + text + "\n")
		m.viewport.SetContent(m.output.String())
		m.events = m.runtime.Execute(context.Background(), text)
		m.state = stateRunning
		return m, tea.Batch(m.spinner.Tick, waitForEvent(m.events))
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m Model) handleEvent(ev app.Event) (tea.Model, tea.Cmd) {
	switch ev.Type {
	case app.EventToken:
		m.output.WriteString(ev.Payload.(app.TokenPayload).Text)
		m.viewport.SetContent(m.output.String())
		m.viewport.GotoBottom()
		return m, waitForEvent(m.events)

	case app.EventThinking:
		m.output.WriteString(dimStyle.Render(ev.Payload.(app.TokenPayload).Text))
		m.viewport.SetContent(m.output.String())
		m.viewport.GotoBottom()
		return m, waitForEvent(m.events)

	case app.EventToolCall:
		p := ev.Payload.(app.ToolCallPayload)
		m.output.WriteString(toolStyle.Render("\n[tool: " + p.Name + "]\n"))
		m.viewport.SetContent(m.output.String())
		m.viewport.GotoBottom()
		return m, waitForEvent(m.events)

	case app.EventPermissionRequest:
		p := ev.Payload.(app.PermissionRequestPayload)
		m.pending = &p
		m.state = stateAvaitingPermission
		return m, nil

	case app.EventError:
		m.output.WriteString(errStyle.Render("\n[error] " + ev.Payload.(app.ErrorPayload).Err.Error() + "\n"))
		m.viewport.SetContent(m.output.String())
		m.state = stateIdle
		return m, nil

	case app.EventDone:
		m.state = stateIdle
		return m, nil
	}

	return m, waitForEvent(m.events)
}
