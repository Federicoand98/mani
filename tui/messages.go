package tui

import (
	"context"

	"github.com/Federicoand98/mani/app"
	"github.com/Federicoand98/mani/config"
	"github.com/Federicoand98/mani/llm/copilot"
	tea "github.com/charmbracelet/bubbletea"
)

// incapsulo un app.Event come msg bubbletea
type eventMsg app.Event

type modelMsg struct {
	models []string
	err    error
}

type deviceCodeMsg struct {
	dc  copilot.DeviceCode
	err error
}

type loginDoneMsg struct {
	provider string
	err      error
}

type appliedMsg struct {
	text string
	err  error
}

type needModelMsg struct {
	provider string
}

// segnala che lo stream è stato chiuso
type streamClosedMsg struct{}

func waitForEvent(ch <-chan app.Event) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return streamClosedMsg{}
		}
		return eventMsg(ev)
	}
}

func fetchModels(rt *app.Runtime) tea.Cmd {
	return func() tea.Msg {
		ms, err := rt.ListModels(context.Background())
		return modelMsg{models: ms, err: err}
	}
}

func applyModel(rt *app.Runtime, model string) tea.Cmd {
	return func() tea.Msg {
		if err := rt.UseModel(model); err != nil {
			return appliedMsg{err: err}
		}
		return appliedMsg{text: "[model: " + model + "]"}
	}
}

func applyProvider(rt *app.Runtime, provider string) tea.Cmd {
	return func() tea.Msg {
		if err := rt.UseProvider(provider); err != nil {
			return appliedMsg{err: err}
		}
		if rt.ModelName() == "" {
			return needModelMsg{provider: provider}
		}
		return appliedMsg{text: "[provider: " + provider + "]"}
	}
}

func startCopilotLogin(rt *app.Runtime) tea.Cmd {
	return func() tea.Msg {
		dc, err := copilot.StartDeviceFlow(context.Background())
		return deviceCodeMsg{dc: dc, err: err}
	}
}

func pollCopilotLogin(rt *app.Runtime, dc copilot.DeviceCode) tea.Cmd {
	return func() tea.Msg {
		tok, err := copilot.PollDeviceFlow(context.Background(), dc)
		if err != nil {
			return loginDoneMsg{provider: "copilot", err: err}
		}
		err = rt.Login("copilot", config.Credential{Type: config.CredOAuth, Refresh: tok})
		return loginDoneMsg{provider: "copilot", err: err}
	}
}
