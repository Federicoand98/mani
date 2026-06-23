package core

import (
	"context"
	"errors"
	"fmt"
)

const maxIterations = 10

type Agent struct {
	Client          LLMClient
	tools           []ToolDefinition
	executors       map[string]ToolExecutor
	emitter         Emitter
	preToolUseHooks []PreToolUseHook // gate permesso
	hooks           *HookManager
	contextLimit    int // 0: unlimited
}

func NewAgent(client LLMClient) *Agent {
	return &Agent{
		Client:    client,
		tools:     []ToolDefinition{},
		executors: make(map[string]ToolExecutor),
		emitter:   nopEmitter{},
		hooks:     NewHookManager(),
	}
}

func (a *Agent) Run(ctx context.Context, memory Memory, userInput string) error {
	memory.Add(Message{Role: RoleUser, Content: []ContentBlock{TextBlock{Text: userInput}}})

	onToken := func(token string, isThinking bool) {
		if isThinking {
			a.emitter.Thinking(token)
		} else {
			a.emitter.Token(token)
		}
	}

	for range maxIterations {
		// HOOK: pre llm call
		pre := &PreLLMCallPayload{Messages: cloneMessages(memory.Messages()), Tools: a.tools}
		if err := a.hooks.Fire(ctx, HookEvent{Type: HookPreLLMCall, Payload: pre}); err != nil {
			return fmt.Errorf("agent: pre llm call hook: %w", err)
		}

		// context window tracking
		if a.contextLimit > 0 {
			tokens := EstimateTokens(pre.Messages)
			if tokens > a.contextLimit*8/10 { // soglia 80%
				cf := &ContextFullPayload{Messages: pre.Messages, Tokens: tokens, Limit: a.contextLimit}
				if err := a.hooks.Fire(ctx, HookEvent{Type: HookContextFull, Payload: cf}); err != nil {
					return fmt.Errorf("agent: context full hook: %w", err)
				}
				pre.Messages = cf.Messages
			}
		}

		resp, err := a.Client.Send(ctx, pre.Messages, a.tools, onToken)
		if err != nil {
			return fmt.Errorf("agent: %w", err)
		}

		// HOOK: post llm call
		post := &PostLLMCallPayload{Response: &resp}
		if err := a.hooks.Fire(ctx, HookEvent{Type: HookPostLLMCall, Payload: post}); err != nil {
			return fmt.Errorf("agent: post llm call hook: %w", err)
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

func (a *Agent) Hooks() *HookManager {
	return a.hooks
}

func (a *Agent) AddTool(def ToolDefinition, exec ToolExecutor) {
	a.tools = append(a.tools, def)
	a.executors[def.Name] = exec
}

func (a *Agent) AddPreToolUseHook(hook PreToolUseHook) {
	a.preToolUseHooks = append(a.preToolUseHooks, hook)
}

func (a *Agent) SetEmitter(emitter Emitter) {
	a.emitter = emitter
}

func (a *Agent) SetContextLimit(limit int) {
	a.contextLimit = limit
}

func (a *Agent) executeTools(ctx context.Context, memory Memory, blocks []ContentBlock) error {
	for i, block := range blocks {
		call, ok := block.(ToolUseBlock)
		if !ok {
			continue
		}

		// tool interrotto prima di eseguire
		if ctx.Err() != nil {
			a.cancelPending(memory, blocks[i:])
			return ctx.Err()
		}

		executor, found := a.executors[call.Name]
		if !found {
			return fmt.Errorf("agent: no executor found for tool %s", call.Name)
		}

		// fase 1: HOOK pre_tool_use
		pre := &PreToolUsePayload{ToolName: call.Name, Input: call.Input}
		if err := a.hooks.Fire(ctx, HookEvent{Type: HookPreToolUse, Payload: pre}); err != nil {
			memory.Add(blockedResult(call.ID, "hook blocked: "+err.Error()))
			continue
		}

		input := pre.Input

		// fase 2: gate permesso (risk level check)
		riskLevel := a.riskFor(call.Name)
		if err := a.runPreToolUseHooks(call.Name, riskLevel, input); err != nil {
			memory.Add(blockedResult(call.ID, err.Error()))
			continue
		}

		// fase 3: esecuzione tool
		a.emitter.ToolCall(call.Name, input)
		result, execErr := executor.Execute(ctx, input)

		// tool interrotto durante l'esecuzione
		if errors.Is(execErr, context.Canceled) {
			a.cancelPending(memory, blocks[i:])
			return ctx.Err()
		}

		isError := false
		if execErr != nil {
			result = execErr.Error()
			isError = true
		}

		// fase 4: HOOK post_tool_use
		pp := &PostToolUsePayload{ToolName: call.Name, Input: input, Result: result, IsError: isError}
		if err := a.hooks.Fire(ctx, HookEvent{Type: HookPostToolUse, Payload: pp}); err != nil {
			return fmt.Errorf("agent: post_tool_use: %w", err)
		}

		result, isError = pp.Result, pp.IsError

		if !isError {
			a.emitter.ToolResult(call.Name, result, false)
		}

		memory.Add(Message{Role: RoleTool, Content: []ContentBlock{
			ToolResultBlock{ToolUseID: call.ID, Content: result, IsError: false},
		}})
	}
	return nil
}

// cancelPending: per ogni tool_use non risposto aggiunge alla memoria un tool_result: CANCELLED
func (a *Agent) cancelPending(memory Memory, remaining []ContentBlock) {
	for _, b := range remaining {
		if call, ok := b.(ToolUseBlock); ok {
			memory.Add(Message{Role: RoleTool, Content: []ContentBlock{ToolResultBlock{ToolUseID: call.ID, Content: "[cancelled by user]", IsError: true}}})
		}
	}
}

func (a *Agent) runPreToolUseHooks(toolName string, level RiskLevel, input map[string]any) error {
	for _, hook := range a.preToolUseHooks {
		if err := hook(toolName, level, input); err != nil {
			return err
		}
	}
	return nil
}

func (a *Agent) riskFor(toolName string) RiskLevel {
	for _, t := range a.tools {
		if t.Name == toolName {
			return t.RiskLevel
		}
	}
	return RiskNone
}

func blockedResult(id, msg string) Message {
	return Message{Role: RoleTool, Content: []ContentBlock{
		ToolResultBlock{ToolUseID: id, Content: "[blocked: " + msg + "]", IsError: true},
	}}
}

func cloneMessages(src []Message) []Message {
	dst := make([]Message, len(src))
	copy(dst, src)
	return dst
}
