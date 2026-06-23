package tui

import (
	"strings"

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
	history      []string
	historyIndex int
	paletteIndex int
	lastInput    int
	lastOutput   int
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
	registry.Register(command.NewConfigCommand(rt))

	return Model{
		runtime:      rt,
		input:        ti,
		spinner:      sp,
		state:        stateIdle,
		commands:     registry,
		history:      make([]string, 0),
		historyIndex: 0,
		paletteIndex: 0,
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

func (m Model) matchingCommands() []command.Info {
	val := m.input.Value()

	if !strings.HasPrefix(val, "/") {
		return nil
	}

	var out []command.Info
	for _, cmd := range m.commands.List() {
		if strings.HasPrefix(cmd.Name, val) {
			out = append(out, cmd)
		}
	}
	return out
}
