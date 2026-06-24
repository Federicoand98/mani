package tui

import (
	"context"

	"github.com/Federicoand98/mani/app"
	"github.com/Federicoand98/mani/config"
	"github.com/Federicoand98/mani/tui/command"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
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
		m.input.Width = msg.Width
		return m, nil

	case tea.MouseMsg:
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd

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

	case modelMsg:
		if msg.err != nil {
			m.state = stateIdle
			m.output += "\nmodel list error: " + msg.err.Error() + "\n"
			m.viewport.SetContent(m.rendered())
			m.viewport.GotoBottom()
			return m, nil
		}
		m.picker = picker{
			title:  "Model",
			items:  msg.models,
			onPick: func(model string) tea.Cmd { return applyModel(m.runtime, model) },
		}
		return m, nil

	case deviceCodeMsg:
		if msg.err != nil {
			m.state = stateIdle
			m.output += "\ndevice code error: " + msg.err.Error() + "\n"
			m.viewport.SetContent(m.rendered())
			m.viewport.GotoBottom()
			return m, nil
		}
		m.output += "\nOpen " + hyperlink(msg.dc.VerificationURI, cyanStyle.Render(msg.dc.VerificationURI)) +
			" and insert this code:\n\n  " + cyanStyle.Bold(true).Render(msg.dc.UserCode) + "\n"
		m.viewport.SetContent(m.rendered())
		m.viewport.GotoBottom()
		return m, tea.Batch(
			copyToClipboard(msg.dc.UserCode),
			pollCopilotLogin(m.runtime, msg.dc),
		)

	case clipboardMsg:
		if msg.ok {
			m.output += dimStyle.Render("  (code copied to clipboard)") + "\n"
		}
		m.viewport.SetContent(m.rendered())
		m.viewport.GotoBottom()
		return m, nil

	case loginDoneMsg:
		m.state = stateIdle
		m.input.EchoMode = textinput.EchoNormal
		if msg.err != nil {
			m.output += "\nlogin error: " + msg.err.Error() + "\n"
		} else {
			m.output += "\nlogin done: " + msg.provider + "\n"
		}
		m.viewport.SetContent(m.rendered())
		m.viewport.GotoBottom()
		return m, nil

	case appliedMsg:
		m.state = stateIdle
		if msg.err != nil {
			m.output += "\n" + msg.err.Error() + "\n"
		} else {
			m.output += "\n" + msg.text + "\n"
		}
		m.viewport.SetContent(m.rendered())
		m.viewport.GotoBottom()
		return m, nil
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

	if m.state == stateChoosing {
		if msg.Type == tea.KeyEsc {
			m.state = stateIdle
			return m, nil
		}
		return m, m.picker.update(msg)
	}

	if m.state == stateLogin {
		if msg.Type == tea.KeyEsc {
			m.state = stateIdle
			m.input.SetValue("")
			m.input.EchoMode = textinput.EchoNormal
			return m, nil
		}
		if m.loginTarget == "copilot" {
			return m, nil // device flow in corso
		}
		if msg.Type == tea.KeyEnter {
			key := m.input.Value()
			target := m.loginTarget
			m.input.SetValue("")
			m.input.EchoMode = textinput.EchoNormal
			return m, func() tea.Msg {
				err := m.runtime.Login(target, config.Credential{Type: config.CredAPI, Key: key})
				return loginDoneMsg{provider: target, err: err}
			}
		}
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}

	if msg.Type == tea.KeyEsc && m.state == stateRunning {
		m.runtime.Cancel()
		return m, nil
	}

	// commands palette
	matches := m.matchingCommands()
	paletteActive := m.state == stateIdle && len(matches) > 0
	if paletteActive {
		if m.paletteIndex >= len(matches) {
			m.paletteIndex = 0
		}

		switch msg.String() {
		case "up":
			if m.paletteIndex > 0 {
				m.paletteIndex--
			}
			return m, nil
		case "down":
			if m.paletteIndex < len(matches)-1 {
				m.paletteIndex++
			}
			return m, nil

		case "tab":
			m.input.SetValue(matches[m.paletteIndex].Name + " ")
			m.input.CursorEnd()
			m.paletteIndex = 0
			return m, nil
		}
	}

	if msg.Type == tea.KeyEnter && m.state == stateIdle {
		text := m.input.Value()

		if text == "" {
			return m, nil
		}

		m.history = append(m.history, text)
		m.historyIndex = len(m.history)

		m.input.Reset()

		res, handled, err := m.commands.Dispatch(text)
		if err != nil {
			m.output += "\n" + err.Error() + "\n"
			m.input.SetValue("")
			return m, nil
		}

		if handled {
			if res.Quit {
				return m, tea.Quit
			}

			switch res.Action {
			case command.ActionPickProvider:
				m.picker = picker{
					title:  "Provider",
					items:  m.runtime.AvailableProviders(),
					onPick: func(p string) tea.Cmd { return applyProvider(m.runtime, p) },
				}
				m.state = stateChoosing

			case command.ActionPickModel:
				m.state = stateChoosing
				m.picker = picker{title: "Loading models..."}
				return m, fetchModels(m.runtime)

			case command.ActionLogin:
				m.loginTarget = res.Arg
				m.state = stateLogin
				if res.Arg == "copilot" {
					return m, startCopilotLogin(m.runtime)
				}
				m.input.SetValue("")
				m.input.Placeholder = "paste API key and press enter"
				m.input.EchoMode = textinput.EchoPassword
				return m, nil

			default:
				m.output += "\n" + res.Output + "\n"
			}

			m.input.SetValue("")
			m.viewport.SetContent(m.rendered())
			m.viewport.GotoBottom()
			return m, nil
		}

		// if res, handled, err := m.commands.Dispatch(text); handled {
		// 	m.output += "\n> " + text + "\n"
		// 	if err != nil {
		// 		m.output += "Error: " + err.Error() + "\n"
		// 	} else if res.Output != "" {
		// 		m.output += res.Output + "\n"
		// 	}

		// 	m.viewport.SetContent(m.rendered())
		// 	m.viewport.GotoBottom()

		// 	if res.Quit {
		// 		return m, tea.Quit
		// 	}

		// 	return m, nil
		// }

		m.output += "\n> " + text + "\n"
		m.viewport.SetContent(m.rendered())
		m.events = m.runtime.Execute(context.Background(), text)
		m.state = stateRunning
		return m, tea.Batch(m.spinner.Tick, waitForEvent(m.events))
	}

	switch msg.String() {
	case "up":
		if m.historyIndex > 0 {
			m.historyIndex--
			m.input.SetValue(m.history[m.historyIndex])
			m.input.CursorEnd()
		}
		return m, nil

	case "down":
		if m.historyIndex < len(m.history)-1 {
			m.historyIndex++
			m.input.SetValue(m.history[m.historyIndex])
			m.input.CursorEnd()
		} else {
			m.historyIndex = len(m.history)
			m.input.SetValue("")
		}
		return m, nil

	case "pgup", "pgdown", "ctrl+u", "ctrl+d":
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m Model) handleEvent(ev app.Event) (tea.Model, tea.Cmd) {
	switch ev.Type {
	case app.EventToken:
		if m.showThinking {
			m.output += "\n\n"
			m.showThinking = false
		}
		m.output += ev.Payload.(app.TokenPayload).Text
		m.viewport.SetContent(m.rendered())
		m.viewport.GotoBottom()
		return m, waitForEvent(m.events)

	case app.EventThinking:
		m.output += dimStyle.Render(ev.Payload.(app.TokenPayload).Text)
		m.showThinking = true
		m.viewport.SetContent(m.rendered())
		m.viewport.GotoBottom()
		return m, waitForEvent(m.events)

	case app.EventToolCall:
		p := ev.Payload.(app.ToolCallPayload)
		m.output += toolStyle.Render("\n[tool: " + p.Name + "]\n")
		m.viewport.SetContent(m.rendered())
		m.viewport.GotoBottom()
		return m, waitForEvent(m.events)

	case app.EventPermissionRequest:
		p := ev.Payload.(app.PermissionRequestPayload)
		m.pending = &p
		m.state = stateAvaitingPermission
		return m, nil

	case app.EventUsage:
		p := ev.Payload.(app.UsagePayload)
		m.lastInput = p.Input
		m.lastOutput = p.Output
		return m, waitForEvent(m.events)

	case app.EventCancelled:
		m.output += dimStyle.Render("\n[cancelled]\n")
		m.viewport.SetContent(m.rendered())
		m.state = stateIdle
		return m, nil

	case app.EventError:
		m.output += errStyle.Render("\n[error] " + ev.Payload.(app.ErrorPayload).Err.Error() + "\n")
		m.viewport.SetContent(m.rendered())
		m.state = stateIdle
		return m, nil

	case app.EventDone:
		m.state = stateIdle
		return m, nil
	}

	return m, waitForEvent(m.events)
}
