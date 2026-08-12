// Package tui is the terminal user interface, built on Bubble Tea.
//
// It consumes the Event stream produced by app.Runtime and renders it: streaming
// tokens, tool calls, permission prompts with a diff preview, and slash commands.
package tui

import (
	"path/filepath"

	"github.com/Federicoand98/mani/app"
	"github.com/Federicoand98/mani/config"
	tea "github.com/charmbracelet/bubbletea"
)

func Run(rt *app.Runtime) error {
	if rt.IsDebugMode() {
		f, err := tea.LogToFile(filepath.Join(config.ConfigDir(), "mani.log"), "mani")
		if err != nil {
			return err
		}
		defer f.Close()
	}

	p := tea.NewProgram(NewModel(rt), tea.WithAltScreen(), tea.WithMouseCellMotion())
	_, err := p.Run()
	return err
}
