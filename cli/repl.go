package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Federicoand98/mani/app"
)

const (
	colorReset   = "\033[0m"
	colorDimGrey = "\033[2m\033[90m"
)

type REPL struct {
	runtime         *app.Runtime
	thinkingEnabled bool
}

func New(rt *app.Runtime) *REPL {
	return &REPL{runtime: rt, thinkingEnabled: true}
}

func (r *REPL) Run(ctx context.Context) {
	fmt.Println("mani - Ctrl+C to exit")

	scanner := bufio.NewScanner(os.Stdin)

	for {
		fmt.Print("> ")
		if !scanner.Scan() {
			return
		}

		input := strings.TrimSpace(scanner.Text())

		if r.handleCommand(input) {
			continue
		}

		events := r.runtime.Execute(ctx, input)

		spinCtx, stopSpinner := context.WithCancel(ctx)
		go runSpinner(spinCtx)

		r.handleEvents(events, stopSpinner)
		fmt.Println()
	}
}

func (r *REPL) handleCommand(cmd string) bool {
	switch cmd {
	case "/quit":
		fmt.Println("Goodbye :(")
		return true
	case "/thinking":
		r.thinkingEnabled = !r.thinkingEnabled
		fmt.Println("Thinking mode:", r.thinkingEnabled)
		return false
	default:
		return false
	}
}

func (r *REPL) handleEvents(events <-chan app.Event, stopSpinner context.CancelFunc) {
	spinnerStopped := false
	stopOnce := func() {
		if !spinnerStopped {
			stopSpinner()
			fmt.Print("\r\033[2k")
			spinnerStopped = true
		}
	}

	for event := range events {
		switch event.Type {
		case app.EventToken:
			stopOnce()
			p := event.Payload.(app.TokenPayload)
			fmt.Print(p)
		case app.EventThinking:
			if r.thinkingEnabled {
				stopOnce()
				p := event.Payload.(app.TokenPayload)
				fmt.Print("\033[2m\033[90m" + p.Text + "\033[0m")
			}
		case app.EventToolCall:
			stopOnce()
			p := event.Payload.(app.ToolCallPayload)
			fmt.Printf("\n[tool call] %s\n", p.Name)
		case app.EventToolResult:
			// per ora silenzioso
		case app.EventError:
			stopOnce()
			p := event.Payload.(app.ErrorPayload)
			fmt.Printf("[error] %s\n", p.Err)
		case app.EventDone:
			stopOnce()
			// silezioso
		}
	}
}

func runSpinner(ctx context.Context) {
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

	fmt.Print("\r" + frames[0])

	ticker := time.NewTicker(150 * time.Millisecond)
	defer ticker.Stop()

	i := 1
	for {
		select {
		case <-ctx.Done():
			// il chiamante (handler o main loop) pulisce la riga: qui non stampiamo nulla
			return
		case <-ticker.C:
			select {
			case <-ctx.Done():
				return
			default:
				fmt.Printf("\r%s", frames[i%len(frames)])
			}
			i++
		}
	}
}
