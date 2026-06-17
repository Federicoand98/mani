package tui

import (
	"github.com/Federicoand98/mani/app"
	tea "github.com/charmbracelet/bubbletea"
)

func Run(rt *app.Runtime) error {
	p := tea.NewProgram(NewModel(rt), tea.WithAltScreen())
	_, err := p.Run()
	return err
}
