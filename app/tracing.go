package app

import (
	"context"
	"log/slog"

	"github.com/Federicoand98/mani/core"
)

func RegisterTracing(rt *Runtime) {
	rt.OnPreLLMCall(func(ctx context.Context, p *core.PreLLMCallPayload) error {
		slog.Debug("llm call", "run", runIDFrom(ctx), "messages", len(p.Messages), "tools", len(p.Tools))
		return nil
	})

	rt.OnPostLLMCall(func(ctx context.Context, p *core.PostLLMCallPayload) error {
		slog.Info("llm response", "run", runIDFrom(ctx),
			"stop", string(p.Response.StopReason), "blocks", len(p.Response.Content),
			"in_tokens", p.Response.Usage.InputTokens, "out_tokens", p.Response.Usage.OutputTokens)
		return nil
	})

	rt.OnPreToolUse(func(ctx context.Context, p *core.PreToolUsePayload) error {
		slog.Info("tool call", "run", runIDFrom(ctx), "tool", p.ToolName, "input", p.Input)
		return nil
	})

	rt.OnPostToolUse(func(ctx context.Context, p *core.PostToolUsePayload) error {
		lvl := slog.LevelInfo
		if p.IsError {
			lvl = slog.LevelWarn
		}

		slog.Log(ctx, lvl, "tool result", "run", runIDFrom(ctx), "tool", p.ToolName, "is_error", p.IsError, "result_len", len(p.Result))
		return nil
	})
}
