package tui

import (
	"strings"

	"github.com/Federicoand98/mani/app"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

type state int

const (
	stateIdle state = iota
	stateRunning
	stateAvaitingPermission
)

type Model struct {
	runtime  *app.Runtime
	input    textinput.Model
	viewport viewport.Model
	spinner  spinner.Model
	state    state
	events   <-chan app.Event
	pending  *app.PermissionRequestPayload
	output   strings.Builder
	ready    bool
}

func NewModel(rt *app.Runtime) Model {
	ti := textinput.New()
	ti.Placeholder = "write a message"
	ti.Focus()

	sp := spinner.New()
	sp.Spinner = spinner.Dot

	return Model{
		runtime: rt,
		input:   ti,
		spinner: sp,
		state:   stateIdle,
	}
}

func (m Model) Init() tea.Cmd {
	return textinput.Blink
}
