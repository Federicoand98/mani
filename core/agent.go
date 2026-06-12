package core

import (
	"context"
	"fmt"
)

const maxIterations = 10

type Agent struct {
	Client           LLMClient
	tools            []ToolDefinition
	executors        map[string]ToolExecutor
	streamHandler    TokenHandler
	toolEventHandler ToolEventHandler
	preToolUseHooks  []PreToolUseHook // TODO: in futuro voglio un manager per tutti gli hooks
}

func NewAgent(client LLMClient) *Agent {
	return &Agent{
		Client:    client,
		tools:     []ToolDefinition{},
		executors: make(map[string]ToolExecutor),
	}
}

func (a *Agent) Run(ctx context.Context, memory Memory, userInput string) error {
	memory.Add(Message{Role: RoleUser, Content: []ContentBlock{TextBlock{Text: userInput}}})

	for range maxIterations {
		resp, err := a.Client.Send(ctx, memory.Messages(), a.tools, a.streamHandler)
		if err != nil {
			return fmt.Errorf("agent: %w", err)
		}

		memory.Add(Message{Role: RoleAssistant, Content: resp.Content})

		switch resp.StopReason {
		case StopReasonEndTurn:
			return nil
		case StopReasonToolUse:
			if err := a.executeTools(ctx, memory, resp.Content); err != nil {
				return fmt.Errorf("agent: execute tools: %w", err)
			}
		case StopReasonMaxTokens:
			return fmt.Errorf("agent: max_token reached")
		}
	}

	return nil
}

func (a *Agent) AddTool(def ToolDefinition, exec ToolExecutor) {
	a.tools = append(a.tools, def)
	a.executors[def.Name] = exec
}

func (a *Agent) AddPreToolUseHook(hook PreToolUseHook) {
	a.preToolUseHooks = append(a.preToolUseHooks, hook)
}

func (a *Agent) SetStreamHandler(handler TokenHandler) {
	a.streamHandler = handler
}

func (a *Agent) SetToolEventHandler(h ToolEventHandler) {
	a.toolEventHandler = h
}

func (a *Agent) executeTools(ctx context.Context, memory Memory, blocks []ContentBlock) error {
	for _, block := range blocks {
		call, ok := block.(ToolUseBlock)
		if !ok {
			continue
		}

		executor, found := a.executors[call.Name]
		if !found {
			return fmt.Errorf("agent: no executor found for tool %s", call.Name)
		}

		riskLevel := RiskNone
		for _, tool := range a.tools {
			if tool.Name == call.Name {
				riskLevel = tool.RiskLevel
				break
			}
		}

		if err := a.runPreToolUseHooks(call.Name, riskLevel, call.Input); err != nil {
			memory.Add(Message{Role: RoleTool, Content: []ContentBlock{
				ToolResultBlock{
					ToolUseID: call.ID,
					Content:   fmt.Sprintf("[blocked tool: %s]", err.Error()),
					IsError:   true,
				},
			}})
			continue
		}

		result, err := executor.Execute(ctx, call.Input)
		if err != nil {
			// l'errore non deve rompere il loop ma rimandato al modello in modo da correggersi
			memory.Add(Message{Role: RoleTool, Content: []ContentBlock{
				ToolResultBlock{
					ToolUseID: call.ID,
					Content:   err.Error(),
					IsError:   true,
				},
			}})
			continue
		}

		if a.toolEventHandler != nil {
			a.toolEventHandler(call.Name, call.Input, result, false)
		}

		memory.Add(Message{Role: RoleTool, Content: []ContentBlock{
			ToolResultBlock{
				ToolUseID: call.ID,
				Content:   result,
				IsError:   false,
			},
		}})
	}
	return nil
}

func (a *Agent) runPreToolUseHooks(toolName string, level RiskLevel, input map[string]any) error {
	for _, hook := range a.preToolUseHooks {
		if err := hook(toolName, level, input); err != nil {
			return err
		}
	}
	return nil
}
