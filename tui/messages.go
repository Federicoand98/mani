package tui

import (
	"github.com/Federicoand98/mani/app"
	tea "github.com/charmbracelet/bubbletea"
)

// incapsulo un app.Event come msg bubbletea
type eventMsg app.Event

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
