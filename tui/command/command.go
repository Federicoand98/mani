// Package command implements the slash commands of the TUI.
//
// A Command parses its arguments, acts on the Runtime and returns a Result: either
// synchronous output, or an Action asking the TUI to enter a mode such as a picker.
package command

import (
	"fmt"
	"strings"

	"github.com/Federicoand98/mani/app"
	"github.com/Federicoand98/mani/config"
)

// -----------------------------------
// -------- Command interface --------
// -----------------------------------
type Action int

const (
	ActionNone Action = iota
	ActionPickModel
	ActionPickProvider
	ActionLogin
)

type Result struct {
	Output string
	Quit   bool
	Action Action
	Arg    string
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

/* Session command */
type SessionCommand struct {
	runtime *app.Runtime
}

func NewSessionCommand(rt *app.Runtime) *SessionCommand { return &SessionCommand{runtime: rt} }

func (c *SessionCommand) Name() string        { return "/session" }
func (c *SessionCommand) Description() string { return "Manages sessions: list, new, switch <id>" }

func (c *SessionCommand) Execute(args []string) (Result, error) {
	if len(args) == 0 {
		return Result{Output: "Usage: /session <list|new|switch> [id]"}, nil
	}

	switch args[0] {
	case "list":
		metas, err := c.runtime.ListSessions()
		if err != nil {
			return Result{}, err
		}

		if len(metas) == 0 {
			return Result{Output: "[no sessions found]"}, nil
		}

		var b strings.Builder
		cur := c.runtime.CurrentSession().ID
		for _, m := range metas {
			marker := "  "
			if m.ID == cur {
				marker = "* "
			}
			b.WriteString(fmt.Sprintf("%s%s  %s (%s)\n", marker, m.ID, m.Title, m.UpdatedAt.Format("15:04 02/01")))
		}

		return Result{Output: b.String()}, nil

	case "new":
		c.runtime.NewSession()
		return Result{Output: "[new session: " + c.runtime.CurrentSession().ID + "]"}, nil

	case "switch":
		if len(args) < 2 {
			return Result{Output: "Usage: /session switch <id>"}, nil
		}

		if err := c.runtime.SwitchSession(args[1]); err != nil {
			return Result{}, err
		}

		return Result{Output: "[switched to session: " + c.runtime.CurrentSession().ID + "]"}, nil

	default:
		return Result{Output: "Unknown subcommand: " + args[0]}, nil
	}
}

/* Config command */
type ConfigCommand struct {
	runtime *app.Runtime
}

func NewConfigCommand(rt *app.Runtime) *ConfigCommand { return &ConfigCommand{runtime: rt} }

func (c *ConfigCommand) Name() string        { return "/config" }
func (c *ConfigCommand) Description() string { return "Displays the current configuration" }

func (c *ConfigCommand) Execute(args []string) (Result, error) {
	return Result{Output: fmt.Sprintf("file: %s\n%s", config.ConfigPath(), c.runtime.ConfigString())}, nil
}

/* Help Command */
type HelpCommand struct {
	reg *Registry
}

func NewHelpCommand(reg *Registry) *HelpCommand { return &HelpCommand{reg: reg} }

func (c *HelpCommand) Name() string        { return "/help" }
func (c *HelpCommand) Description() string { return "List all available commands" }

func (c *HelpCommand) Execute(args []string) (Result, error) {
	var b strings.Builder
	b.WriteString("Commands:\n")
	for _, cmd := range c.reg.List() {
		b.WriteString(fmt.Sprintf("  %-12s: %s\n", cmd.Name, cmd.Description))
	}
	return Result{Output: b.String()}, nil
}

/* Model Command */
type ModelCommand struct {
	runtime *app.Runtime
}

func NewModelCommand(rt *app.Runtime) *ModelCommand { return &ModelCommand{runtime: rt} }

func (c *ModelCommand) Name() string        { return "/model" }
func (c *ModelCommand) Description() string { return "Switches the active model" }

func (c *ModelCommand) Execute(args []string) (Result, error) {
	return Result{Action: ActionPickModel}, nil
}

/* Provider Command */
type ProviderCommand struct {
	runtime *app.Runtime
}

func NewProviderCommand(rt *app.Runtime) *ProviderCommand { return &ProviderCommand{runtime: rt} }

func (c *ProviderCommand) Name() string        { return "/provider" }
func (c *ProviderCommand) Description() string { return "Switches the active provider" }

func (c *ProviderCommand) Execute(args []string) (Result, error) {
	return Result{Action: ActionPickProvider}, nil
}

/* Login Command */
type LoginCommand struct {
	runtime *app.Runtime
}

func NewLoginCommand(rt *app.Runtime) *LoginCommand { return &LoginCommand{runtime: rt} }

func (c *LoginCommand) Name() string        { return "/login" }
func (c *LoginCommand) Description() string { return "Logs in to the provider: /login <provider>" }

func (c *LoginCommand) Execute(args []string) (Result, error) {
	if len(args) == 0 {
		return Result{Output: "Usage: /login <openai|anthropic|copilot|openrouter>"}, nil
	}
	return Result{Action: ActionLogin, Arg: args[0]}, nil
}

/* Logout Command */
type LogoutCommand struct {
	runtime *app.Runtime
}

func NewLogoutCommand(rt *app.Runtime) *LogoutCommand { return &LogoutCommand{runtime: rt} }

func (c *LogoutCommand) Name() string { return "/logout" }
func (c *LogoutCommand) Description() string {
	return "Logs out of the current provider: /logout <provider>"
}

func (c *LogoutCommand) Execute(args []string) (Result, error) {
	if len(args) == 0 {
		return Result{Output: "Usage: /logout <provider>"}, nil
	}

	if err := c.runtime.Logout(args[0]); err != nil {
		return Result{}, err
	}

	return Result{Output: "Logged out successfully"}, nil
}
