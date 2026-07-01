package app

import (
	"context"
	"fmt"

	"github.com/Federicoand98/mani/core"
	"github.com/Federicoand98/mani/tool"
)

type depthKey struct{}

func depthFrom(ctx context.Context) int {
	d, _ := ctx.Value(depthKey{}).(int)
	return d
}

func withDepth(ctx context.Context, d int) context.Context {
	return context.WithValue(ctx, depthKey{}, d)
}

func RegisterSubagents(rt *Runtime, maxDepth int) {
	rt.WithTool(newDelegateTool(rt, maxDepth))
}

func newDelegateTool(rt *Runtime, maxDepth int) tool.Tool {
	schema := tool.ToolSchema{
		Name: "delegate",
		Description: "Delegates a sub-task to a sub-agent with fresh memory and the same tools." +
			"Use it to isolate focused work (reasearch, a narrow edit) without bloating your context." +
			"'task' is what to do; 'instructions' (optional) is the sub-agent's system prompt." +
			"Returns the final response from the sub-agent.",
		InputSchema: tool.InputSchema{
			Type: "object",
			Properties: map[string]tool.PropertySchema{
				"task":         {Type: "string", Description: "The sub-task to be performed by the sub-agent."},
				"instructions": {Type: "string", Description: "The sub-agent's system prompt (optional)."},
			},
			Required: []string{"task"},
		},
	}

	tool := tool.New(schema, core.RiskNone, func(ctx context.Context, input map[string]any) (string, error) {
		depth := depthFrom(ctx)

		if depth >= maxDepth {
			return "", fmt.Errorf("delegate: max depth reached (%d), no further delegation.", maxDepth)
		}

		task, _ := input["task"].(string)
		if task == "" {
			return "", fmt.Errorf("delegate: task is required.")
		}

		instructions, _ := input["instructions"].(string)

		child := rt.spawnChild()
		mem := core.NewInMemory()

		if instructions != "" {
			mem.Add(core.Message{
				Role:    core.RoleSystem,
				Content: []core.ContentBlock{core.TextBlock{Text: instructions}},
			})
		}

		childCtx := withDepth(ctx, depth+1)

		if err := child.Run(childCtx, mem, task); err != nil {
			return "", fmt.Errorf("delegate: sub-agent error: %w", err)
		}

		return lastAssistantText(mem), nil
	})

	return tool
}

func lastAssistantText(mem *core.InMemory) string {
	msgs := mem.Messages()

	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == core.RoleAssistant {
			continue
		}

		for _, b := range msgs[i].Content {
			if tb, ok := b.(core.TextBlock); ok {
				return tb.Text
			}
		}
	}

	return "(subagent: no text response)"
}
