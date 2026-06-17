package tui

import "github.com/charmbracelet/lipgloss"

var (
	dimStyle  = lipgloss.NewStyle().Faint(true)
	toolStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("12"))
	cyanStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("14"))
	errStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
)
