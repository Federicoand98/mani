package command

import (
	"fmt"

	"github.com/Federicoand98/mani/app"
)

// -----------------------------------
// -------- Command interface --------
// -----------------------------------
type Result struct {
	Output string
	Quit   bool
}

type Command interface {
	Name() string
	Description() string
	Execute(args []string) (Result, error)
}

// -----------------------------------
// -------- Command implementation ---
// -----------------------------------

/* Quit command */
type QuitCommand struct {
	runtime *app.Runtime
}

func NewQuitCommand(rt *app.Runtime) *QuitCommand { return &QuitCommand{runtime: rt} }

func (c *QuitCommand) Name() string        { return "/quit" }
func (c *QuitCommand) Description() string { return "Quits the session" }

func (c *QuitCommand) Execute(args []string) (Result, error) {
	return Result{Output: "[quitting]", Quit: true}, nil
}

/* Thinking command */
type ThinkingCommand struct {
	runtime *app.Runtime
}

func NewThinkingCommand(rt *app.Runtime) *ThinkingCommand { return &ThinkingCommand{runtime: rt} }

func (c *ThinkingCommand) Name() string        { return "/thinking" }
func (c *ThinkingCommand) Description() string { return "Toggles the thinking process of the session" }

func (c *ThinkingCommand) Execute(args []string) (Result, error) {
	think := c.runtime.ToggleThinking()
	state := "off"
	if think {
		state = "on"
	}
	return Result{Output: fmt.Sprintf("[thinking output]: %s", state)}, nil
}

/* Clear command */
type ClearCommand struct {
	runtime *app.Runtime
}

func NewClearCommand(rt *app.Runtime) *ClearCommand { return &ClearCommand{runtime: rt} }

func (c *ClearCommand) Name() string        { return "/clear" }
func (c *ClearCommand) Description() string { return "Clears the memory of the session" }

func (c *ClearCommand) Execute(args []string) (Result, error) {
	c.runtime.ClearMemory()
	return Result{Output: "[memory cleared]"}, nil
}

/* Memory command */
type MemoryCommand struct {
	runtime *app.Runtime
}

func NewMemoryCommand(rt *app.Runtime) *MemoryCommand { return &MemoryCommand{runtime: rt} }

func (c *MemoryCommand) Name() string        { return "/memory" }
func (c *MemoryCommand) Description() string { return "Shows all the memory of the session" }

func (c *MemoryCommand) Execute(args []string) (Result, error) {
	memory := c.runtime.Memory()
	return Result{Output: fmt.Sprintf("[memory]:\n%s\n", memory)}, nil
}

// TODO: /help and /export
