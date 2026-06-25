package core

import (
	"context"
	"errors"
	"fmt"
	"sync"
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

		a.emitter.Usage(resp.Usage.InputTokens, resp.Usage.OutputTokens)

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

	return fmt.Errorf("agent: reached max iterations without completing the task (%d)", maxIterations)
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
	type task struct {
		idx      int
		call     ToolUseBlock
		executor ToolExecutor
		input    map[string]any
		risk     RiskLevel
	}

	results := make([]*Message, len(blocks))
	var pending []task

	for i, block := range blocks {
		call, ok := block.(ToolUseBlock)
		if !ok {
			continue
		}

		// tool interrotto prima di eseguire
		if ctx.Err() != nil {
			break
		}

		executor, found := a.executors[call.Name]
		if !found {
			results[i] = toolResult(call.ID, "no executor for tool "+call.Name, true)
			continue
		}

		// fase 1: HOOK pre_tool_use
		pre := &PreToolUsePayload{ToolName: call.Name, Input: call.Input}
		if err := a.hooks.Fire(ctx, HookEvent{Type: HookPreToolUse, Payload: pre}); err != nil {
			results[i] = toolResult(call.ID, "hook blocked: "+err.Error(), true)
			continue
		}

		input := pre.Input

		// fase 2: gate permesso (risk level check)
		riskLevel := a.riskFor(call.Name)
		if err := a.runPreToolUseHooks(call.Name, riskLevel, input); err != nil {
			results[i] = toolResult(call.ID, "hook blocked: "+err.Error(), true)
			continue
		}

		pending = append(pending, task{idx: i, call: call, executor: executor, input: input, risk: riskLevel})
	}

	// riskNone in parallelo
	if ctx.Err() == nil {
		var wg sync.WaitGroup
		for _, t := range pending {
			if t.risk != RiskNone {
				continue
			}

			wg.Add(1)

			go func(t task) {
				defer wg.Done()
				results[t.idx] = a.runTool(ctx, t.call, t.executor, t.input)
			}(t)
		}
		wg.Wait()
	}

	// write/execute sequenziali in ordine
	for _, t := range pending {
		if t.risk == RiskNone {
			continue
		}
		if ctx.Err() != nil {
			break
		}
		results[t.idx] = a.runTool(ctx, t.call, t.executor, t.input)
	}

	for i, block := range blocks {
		if call, ok := block.(ToolUseBlock); ok && results[i] == nil {
			results[i] = toolResult(call.ID, "[cancelled by user]", true)
		}
	}

	// scrivo tutto in memoria
	for _, m := range results {
		if m != nil {
			memory.Add(*m)
		}
	}

	return ctx.Err()
}

func (a *Agent) runTool(ctx context.Context, call ToolUseBlock, executor ToolExecutor, input map[string]any) *Message {
	a.emitter.ToolCall(call.Name, input)
	result, execErr := executor.Execute(ctx, input)

	if errors.Is(execErr, context.Canceled) {
		return toolResult(call.ID, "[cancelled by user]", true)
	}

	isError := false
	if execErr != nil {
		result = execErr.Error()
		isError = true
	}

	pp := &PostToolUsePayload{ToolName: call.Name, Input: input, Result: result, IsError: isError}
	if err := a.hooks.Fire(ctx, HookEvent{Type: HookPostToolUse, Payload: pp}); err != nil {
		return toolResult(call.ID, "post_tool_use hook: "+err.Error(), true)
	}

	result, isError = pp.Result, pp.IsError

	if !isError {
		a.emitter.ToolResult(call.Name, result, false)
	}

	return toolResult(call.ID, result, isError)
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

func toolResult(id, content string, isError bool) *Message {
	return &Message{Role: RoleTool, Content: []ContentBlock{
		ToolResultBlock{ToolUseID: id, Content: content, IsError: isError},
	}}
}
