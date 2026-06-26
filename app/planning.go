package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Federicoand98/mani/core"
	"github.com/Federicoand98/mani/session"
	"github.com/Federicoand98/mani/tool"
)

func RegisterPlanning(rt *Runtime) {
	rt.WithTool(newPlanTool(rt))
}

func newPlanTool(rt *Runtime) tool.Tool {
	schema := tool.ToolSchema{
		Name:        "todo_write",
		Description: `Create or update the to-do list for the current task. ALWAYS pass the entire list (replacing the previous one). 'steps' is a JSON array of objects: {"description": string, "status": "pending"|"in_progress"|"done"}. Use it to plan multi-step tasks and track progress: mark a step as in_progress while working on it, and done when finished.`,
		InputSchema: tool.InputSchema{
			Type: "object",
			Properties: map[string]tool.PropertySchema{
				"steps": {
					Type:        "string",
					Description: `JSON Array of {"description", "status"}`,
				},
			},
			Required: []string{"step"},
		},
	}

	return tool.New(schema, core.RiskNone, func(ctx context.Context, input map[string]any) (string, error) {
		raw, _ := input["steps"].(string)

		var steps []session.PlanStep
		if err := json.Unmarshal([]byte(raw), &steps); err != nil {
			return "", fmt.Errorf("todo_write: 'steps' not a valid json: %w", err)
		}

		for i := range steps {
			switch steps[i].Status {
			case session.StepPending, session.StepInProgress, session.StepDone:
				// ok
			case "":
				steps[i].Status = session.StepPending
			default:
				return "", fmt.Errorf("todo_write: invalid status '%s' for step %d", steps[i].Status, i)
			}
		}

		rt.SetPlan(steps)
		return "updated plan:\n" + renderPlanText(steps), nil
	})
}

func renderPlanText(steps []session.PlanStep) string {
	var b strings.Builder
	for _, s := range steps {
		mark := "[ ]"
		switch s.Status {
		case session.StepInProgress:
			mark = "[~]"
		case session.StepDone:
			mark = "[x]"
		}
		b.WriteString(mark + " " + s.Description + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}
