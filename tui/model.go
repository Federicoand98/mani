package tui

import (
	"github.com/Federicoand98/mani/app"
	"github.com/Federicoand98/mani/cli/command"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type state int

const (
	stateIdle state = iota
	stateRunning
	stateAvaitingPermission
)

type Model struct {
	runtime      *app.Runtime
	input        textinput.Model
	viewport     viewport.Model
	spinner      spinner.Model
	state        state
	events       <-chan app.Event
	pending      *app.PermissionRequestPayload
	commands     *command.Registry
	output       string
	ready        bool
	showThinking bool
}

func NewModel(rt *app.Runtime) Model {
	ti := textinput.New()
	ti.Placeholder = "write a message"
	ti.Focus()

	sp := spinner.New()
	sp.Spinner = spinner.Dot

	registry := command.NewRegistry()
	registry.Register(command.NewClearCommand(rt))
	registry.Register(command.NewThinkingCommand(rt))
	registry.Register(command.NewMemoryCommand(rt))
	registry.Register(command.NewQuitCommand(rt))
	registry.Register(command.NewSessionCommand(rt))

	return Model{
		runtime:  rt,
		input:    ti,
		spinner:  sp,
		state:    stateIdle,
		commands: registry,
	}
}

func (m Model) Init() tea.Cmd {
	return textinput.Blink
}

func (m Model) rendered() string {
	if m.viewport.Width == 0 {
		return m.output
	}

	return lipgloss.NewStyle().Width(m.viewport.Width).Render(m.output)
}
