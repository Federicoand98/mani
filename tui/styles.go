package tui

import "github.com/charmbracelet/lipgloss"

var (
	dimStyle    = lipgloss.NewStyle().Faint(true).Foreground(lipgloss.Color("240"))
	toolStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("12"))
	cyanStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("14"))
	errStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	promptStyle = lipgloss.NewStyle().Border(lipgloss.NormalBorder(), true, false).BorderForeground(lipgloss.Color("240"))
	addStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	delStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
)
