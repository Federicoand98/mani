package app

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Federicoand98/mani/core"
	"github.com/Federicoand98/mani/tool"
)

// budgetState: counter PER-RUN. Si azzera ad ogni execute.
type budgetState struct {
	mu        sync.Mutex
	tokens    int
	toolCalls int
}

type budgetKey struct{}

func budgetFrom(ctx context.Context) *budgetState {
	bs, _ := ctx.Value(budgetKey{}).(*budgetState)
	return bs
}

func RegisterBudget(rt *Runtime, spec BudgetSpec) {
	if spec.MaxTokens > 0 {
		rt.OnPostLLMCall(func(ctx context.Context, p *core.PostLLMCallPayload) error {
			bs := budgetFrom(ctx)

			if bs == nil {
				return nil
			}

			bs.mu.Lock()
			bs.tokens += p.Response.Usage.InputTokens + p.Response.Usage.OutputTokens
			over := bs.tokens > spec.MaxTokens
			bs.mu.Unlock()

			if over {
				return fmt.Errorf("budget: max_tokens reached (limit: %d)", spec.MaxTokens)
			}

			return nil
		})
	}

	if spec.MaxToolCalls > 0 {
		rt.OnPreToolUse(func(ctx context.Context, ptup *core.PreToolUsePayload) error {
			if bs := budgetFrom(ctx); bs != nil {
				bs.mu.Lock()
				bs.toolCalls++
				bs.mu.Unlock()
			}
			return nil
		})

		rt.OnPreLLMCall(func(ctx context.Context, plp *core.PreLLMCallPayload) error {
			bs := budgetFrom(ctx)
			if bs == nil {
				return nil
			}

			bs.mu.Lock()
			over := bs.toolCalls > spec.MaxToolCalls
			bs.mu.Unlock()

			if over {
				return fmt.Errorf("budget: max_tool_calls reached (limit: %d)", spec.MaxToolCalls)
			}

			return nil
		})
	}
}

type timeoutTool struct {
	tool.Tool
	timeout time.Duration
}

func withToolTimeout(t tool.Tool, d time.Duration) tool.Tool {
	return &timeoutTool{
		Tool:    t,
		timeout: d,
	}
}

func (t *timeoutTool) Execute(ctx context.Context, input map[string]any) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, t.timeout)
	defer cancel()
	return t.Tool.Execute(ctx, input)
}
