package app

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/Federicoand98/mani/core"
	"github.com/Federicoand98/mani/journal"
	"github.com/Federicoand98/mani/journal/sqlite"
	"github.com/Federicoand98/mani/manifest"
)

// ======================================================
// HOOKS
// ======================================================

type sourceKey struct{}

func WithSource(ctx context.Context, src string) context.Context {
	return context.WithValue(ctx, sourceKey{}, src)
}

func sourceFrom(ctx context.Context) string {
	if src, ok := ctx.Value(sourceKey{}).(string); ok && src != "" {
		return src
	}
	return ""
}

// attachJournalHooks: attachs the hooks of the journal for a single agent.
func attachJournalHooks(h *core.HookManager, j journal.Journal) {
	h.OnPreLLMCall(func(ctx context.Context, plp *core.PreLLMCallPayload) error {
		appendSafe(j, journal.RunEvent{
			RunID: runIDFrom(ctx), Depth: depthFrom(ctx), At: time.Now(), Kind: journal.EvLLMCall, Data: map[string]any{"messages": len(plp.Messages), "tools": len(plp.Tools)},
		})
		return nil
	})

	h.OnPostLLMCall(func(ctx context.Context, plp *core.PostLLMCallPayload) error {
		appendSafe(j, journal.RunEvent{
			RunID: runIDFrom(ctx), Depth: depthFrom(ctx), At: time.Now(), Kind: journal.EvLLMReponse,
			Data: map[string]any{
				"stop_reason": string(plp.Response.StopReason),
				"in_tokens":   plp.Response.Usage.InputTokens,
				"out_tokens":  plp.Response.Usage.OutputTokens,
			},
		})
		return nil
	})

	h.OnPreToolUse(func(ctx context.Context, ptup *core.PreToolUsePayload) error {
		appendSafe(j, journal.RunEvent{
			RunID: runIDFrom(ctx), Depth: depthFrom(ctx), At: time.Now(), Kind: journal.EvToolCall, Data: map[string]any{"tool": ptup.ToolName, "input": ptup.Input},
		})
		return nil
	})

	h.OnPostToolUse(func(ctx context.Context, ptup *core.PostToolUsePayload) error {
		appendSafe(j, journal.RunEvent{
			RunID: runIDFrom(ctx), Depth: depthFrom(ctx), At: time.Now(), Kind: journal.EvToolResult, Data: map[string]any{"tool": ptup.ToolName, "is_error": ptup.IsError, "result_len": len(ptup.Result)},
		})
		return nil
	})
}

// appendSafe: best effort append to the journal, logs warnings on failure.
func appendSafe(j journal.Journal, ev journal.RunEvent) {
	if j == nil {
		return
	}

	if err := j.Append(ev); err != nil {
		slog.Warn("journal append failed", "run", ev.RunID, "kind", ev.Kind, "err", err)
	}
}

func (r *Runtime) recordGovernance(ctx context.Context, action, tool, label string) {
	if r.journal == nil {
		return
	}
	data := map[string]any{"tool": tool, "action": action}
	if label != "" {
		data["label"] = label
	}
	appendSafe(r.journal, journal.RunEvent{
		RunID: runIDFrom(ctx), Depth: depthFrom(ctx), At: time.Now(),
		Kind: journal.EvGuardrail, Data: data,
	})
}

// RegisterJournal attaches the journal to the runtime and registers the journal hooks.
func RegisterJournal(rt *Runtime, j journal.Journal) {
	rt.journal = j
	attachJournalHooks(rt.agent.Hooks(), j)
}

func newPersistentJournal(spec manifest.JournalSpec) (journal.Journal, error) {
	switch backend := strings.ToLower(strings.TrimSpace(spec.Backend)); backend {
	case "", "jsonl":
		return journal.NewJSONL(spec.Path)
	case "sqlite":
		return sqlite.New(spec.Path, journal.Retention(spec.Retention))
	default:
		return nil, fmt.Errorf("unsupported journal backend %q (jsonl|sqlite)", spec.Backend)
	}
}

func OpenJournalReader(spec manifest.Spec) (journal.Journal, bool) {
	js := spec.Observability.Journal
	if !js.Enabled || js.Path == "" {
		return nil, false
	}
	j, err := newPersistentJournal(js)
	if err != nil {
		return nil, false
	}
	return j, true
}
