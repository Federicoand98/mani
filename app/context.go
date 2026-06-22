package app

import (
	"context"
	"os"
	"path/filepath"

	"github.com/Federicoand98/mani/core"
)

const baseSystemPrompt = "Sei mani, un micro harness. Sei conciso e preciso. Usa sempre i tool a tua disposizione per risolvere i problemi. Chiedi prima di azioni distruttive."

// RegistrerContextInjection aggancia un prellmcall hook che antepone un messaggio
// Role: system (base + AGENTS.md del workspace se presente) alla COPIA dei messaggi ad ogni chiamata.
func RegistrerContextInjection(rt *Runtime, workspace string) {
	rt.OnPreLLMCall(func(ctx context.Context, p *core.PreLLMCallPayload) error {
		sys := buildSystemPrompt(workspace)
		if sys == "" {
			return nil
		}

		if len(p.Messages) > 0 && p.Messages[0].Role == core.RoleSystem {
			return nil
		}

		p.Messages = append([]core.Message{{Role: core.RoleSystem, Content: []core.ContentBlock{core.TextBlock{Text: sys}}}}, p.Messages...)
		return nil
	})
}

func buildSystemPrompt(workspace string) string {
	sys := baseSystemPrompt

	if data, err := os.ReadFile(filepath.Join(workspace, "AGENTS.nd")); err == nil {
		sys += "\n\n# Project Context (AGENTS.md)\n" + string(data)
	}

	return sys
}

// Simple compaction
func RegisterTrimCompaction(rt *Runtime, keepRecent int) {
	rt.OnPreLLMCall(func(ctx context.Context, p *core.PreLLMCallPayload) error {
		if len(p.Messages) <= keepRecent+1 {
			return nil
		}

		head := 0
		if p.Messages[0].Role == core.RoleSystem {
			head = 1
		}

		kept := append([]core.Message{}, p.Messages[:head]...)
		kept = append(kept, p.Messages[len(p.Messages)-keepRecent:]...)
		p.Messages = kept
		return nil
	})
}
