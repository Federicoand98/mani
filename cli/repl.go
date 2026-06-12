package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/Federicoand98/mani/app"
	"github.com/Federicoand98/mani/cli/command"
)

const (
	colorReset   = "\033[0m"
	colorDimGrey = "\033[2m\033[90m"
)

type REPL struct {
	runtime         *app.Runtime
	thinkingEnabled bool
	commandRegistry *command.Registry
	mu              sync.Mutex
}

func New(rt *app.Runtime) *REPL {
	registry := command.NewRegistry()
	registry.Register(command.NewQuitCommand(rt))
	registry.Register(command.NewThinkingCommand(rt))
	registry.Register(command.NewClearCommand(rt))
	registry.Register(command.NewMemoryCommand(rt))

	return &REPL{runtime: rt, thinkingEnabled: true, commandRegistry: registry}
}

func (r *REPL) Run(ctx context.Context) {
	fmt.Println("mani - Ctrl+C to exit")

	scanner := bufio.NewScanner(os.Stdin)

	// hooks
	r.runtime.UsePermissionManager(r.permissionPrompter)

	for {
		fmt.Print("> ")
		if !scanner.Scan() {
			return
		}

		input := strings.TrimSpace(scanner.Text())

		res, handled, err := r.commandRegistry.Dispatch(input)
		if handled {
			if err != nil {
				fmt.Println("Error:", err)
			} else if res.Output != "" {
				fmt.Println(res.Output)
			}
			if res.Quit {
				return
			}
			continue
		}

		fmt.Println()
		events := r.runtime.Execute(ctx, input)

		spinCtx, stopSpinner := context.WithCancel(ctx)
		go r.runSpinner(spinCtx)

		r.handleEvents(events, stopSpinner)
		fmt.Println()
	}
}

func (r *REPL) handleEvents(events <-chan app.Event, stopSpinner context.CancelFunc) {
	spinnerStopped := false
	stopOnce := func() {
		if !spinnerStopped {
			stopSpinner()
			r.mu.Lock()
			fmt.Print("\r\033[2K")
			r.mu.Unlock()
			spinnerStopped = true
		}
	}

	thinkingShown := false

	for event := range events {
		switch event.Type {
		case app.EventToken:
			stopOnce()
			r.mu.Lock()
			if thinkingShown {
				fmt.Println()
				thinkingShown = false
			}
			p := event.Payload.(app.TokenPayload)
			fmt.Print(p.Text)
			r.mu.Unlock()

		case app.EventThinking:
			if r.thinkingEnabled {
				stopOnce()
				r.mu.Lock()
				p := event.Payload.(app.TokenPayload)
				fmt.Print(colorDimGrey + p.Text + colorReset)
				thinkingShown = true
				r.mu.Unlock()
			}

		case app.EventToolCall:
			stopOnce()
			r.mu.Lock()
			p := event.Payload.(app.ToolCallPayload)
			fmt.Printf("\n[tool: %s]\n", p.Name)
			r.mu.Unlock()

		case app.EventToolResult:
			// silenzioso

		case app.EventError:
			stopOnce()
			r.mu.Lock()
			p := event.Payload.(app.ErrorPayload)
			fmt.Printf("[error] %v\n", p.Err)
			r.mu.Unlock()

		case app.EventDone:
			stopOnce()
		}
	}
}

func (r *REPL) permissionPrompter(toolName, riskLevel string) app.Decision {
	r.mu.Lock()
	fmt.Printf("\n[required permission] %s (risk: %s) - continue? [y]once / [n]no / [a]always ", toolName, riskLevel)
	r.mu.Unlock()

	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		switch strings.ToLower(strings.TrimSpace(scanner.Text())) {
		case "y":
			return app.AllowOnce
		case "a":
			return app.AllowAlways
		}
	}

	return app.Deny
}

func (r *REPL) runSpinner(ctx context.Context) {
	frames := []string{
		"⠋ thinking",
		"⠙ thinking",
		"⠹ thinking",
		"⠸ thinking",
		"⠼ thinking",
		"⠴ thinking",
		"⠦ thinking",
		"⠧ thinking",
	}

	r.mu.Lock()
	fmt.Print("\r" + frames[0])
	r.mu.Unlock()

	ticker := time.NewTicker(150 * time.Millisecond)
	defer ticker.Stop()

	i := 1
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.mu.Lock()
			if ctx.Err() != nil {
				r.mu.Unlock()
				return
			}
			fmt.Printf("\r%s", frames[i%len(frames)])
			r.mu.Unlock()
			i++
		}
	}
}
