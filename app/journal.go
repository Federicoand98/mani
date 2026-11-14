package app

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

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

// maxJournalText caps the task and the answer a run records, and
// maxJournalPreview the output of each tool: a journal is read by people,
// a 2 MB pasted document is not.
const (
	maxJournalText    = 16 << 10
	maxJournalPreview = 2 << 10
)

// clip cuts s to at most n bytes, on a rune boundary, marking the cut.
func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	cut := n
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "\n[... cut at " + strconv.Itoa(n) + " bytes]"
}

// attachJournalHooks: attachs the hooks of the journal for a single agent,
// except tool results: see attachJournalResults.
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

}

// attachJournalResults records each tool result with a preview of its output.
// Register it after every hook that rewrites results (redaction, truncation):
// the preview must never show what the model was not shown.
func attachJournalResults(h *core.HookManager, j journal.Journal) {
	h.OnPostToolUse(func(ctx context.Context, ptup *core.PostToolUsePayload) error {
		appendSafe(j, journal.RunEvent{
			RunID: runIDFrom(ctx), Depth: depthFrom(ctx), At: time.Now(), Kind: journal.EvToolResult,
			Data: map[string]any{"tool": ptup.ToolName, "is_error": ptup.IsError, "result_len": len(ptup.Result),
				"preview": clip(ptup.Result, maxJournalPreview)},
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

// RegisterJournal attaches the journal to the runtime and registers the journal
// hooks. Call it after the hooks that rewrite tool results, or the previews it
// records will show results before they were rewritten.
func RegisterJournal(rt *Runtime, j journal.Journal) {
	rt.journal = j
	attachJournalHooks(rt.agent.Hooks(), j)
	attachJournalResults(rt.agent.Hooks(), j)
}

// recordPermission writes the answer to an "ask" in the journal.
func (r *Runtime) recordPermission(ctx context.Context, tool, decision string) {
	if r.journal == nil {
		return
	}
	appendSafe(r.journal, journal.RunEvent{
		RunID: runIDFrom(ctx), Depth: depthFrom(ctx), At: time.Now(), Kind: journal.EvPermission,
		Data: map[string]any{"tool": tool, "decision": decision},
	})
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
