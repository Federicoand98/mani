// Package core is the domain of mani: the agent loop and the ports it depends on.
//
// It holds Agent (given a Memory and an input, calls the LLM and executes tools until
// the model ends the turn), the LLMClient, Memory, ToolExecutor and Emitter ports, the
// hook system, and the shared message types.
//
// The single invariant of the project: core has zero external dependencies. It knows
// nothing about HTTP, providers, the filesystem or the UI — everything else is an
// adapter at the edges.
package core

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

const defaultMaxIterations = 10

type Agent struct {
	Client          LLMClient
	tools           []ToolDefinition
	executors       map[string]ToolExecutor
	preToolUseHooks []PreToolUseHook // gate permesso
	hooks           *HookManager
	contextLimit    int // 0: unlimited
	maxIterations   int
	finalTool       string // "" = nessuno
}

type RunResult struct {
	FinalResult map[string]any // nil = nessuno
	Text        string
}

func NewAgent(client LLMClient) *Agent {
	return &Agent{
		Client:        client,
		tools:         []ToolDefinition{},
		executors:     make(map[string]ToolExecutor),
		hooks:         NewHookManager(),
		maxIterations: defaultMaxIterations,
	}
}

func (a *Agent) Run(ctx context.Context, memory Memory, userInput string, em Emitter, attachments ...ContentBlock) (RunResult, error) {
	if em == nil {
		em = nopEmitter{}
	}

	var finalResult map[string]any

	content := []ContentBlock{TextBlock{Text: userInput}}
	content = append(content, attachments...)
	memory.Add(Message{Role: RoleUser, Content: content})

	onToken := func(token string, isThinking bool) {
		if isThinking {
			em.Thinking(token)
		} else {
			em.Token(token)
		}
	}

	for range a.maxIterations {
		// HOOK: pre llm call
		pre := &PreLLMCallPayload{Messages: cloneMessages(memory.Messages()), Tools: a.tools}
		if err := a.hooks.Fire(ctx, HookEvent{Type: HookPreLLMCall, Payload: pre}); err != nil {
			return RunResult{}, fmt.Errorf("agent: pre llm call hook: %w", err)
		}

		// context window tracking
		if a.contextLimit > 0 {
			tokens := EstimateTokens(pre.Messages)
			if tokens > a.contextLimit*8/10 { // soglia 80%
				cf := &ContextFullPayload{Messages: pre.Messages, Tokens: tokens, Limit: a.contextLimit}
				if err := a.hooks.Fire(ctx, HookEvent{Type: HookContextFull, Payload: cf}); err != nil {
					return RunResult{}, fmt.Errorf("agent: context full hook: %w", err)
				}
				pre.Messages = cf.Messages
			}
		}

		resp, err := a.Client.Send(ctx, pre.Messages, a.tools, onToken)
		if err != nil {
			return RunResult{}, fmt.Errorf("agent: %w", err)
		}

		// HOOK: post llm call
		post := &PostLLMCallPayload{Response: &resp}
		if err := a.hooks.Fire(ctx, HookEvent{Type: HookPostLLMCall, Payload: post}); err != nil {
			return RunResult{}, fmt.Errorf("agent: post llm call hook: %w", err)
		}

		em.Usage(resp.Usage.InputTokens, resp.Usage.OutputTokens)

		memory.Add(Message{Role: RoleAssistant, Content: resp.Content})

		switch resp.StopReason {
		case StopReasonEndTurn:
			// guard: se c'e' uno schema ma il modello ha risposto in testo lo forzo
			if a.finalTool != "" && finalResult == nil {
				memory.Add(Message{Role: RoleUser, Content: []ContentBlock{
					TextBlock{Text: "You must return the result ONLY by calling the tool " + a.finalTool + " with the requested schema. Do not reply in free text."},
				}})
				continue
			}
			return RunResult{FinalResult: finalResult, Text: lastAssistantText(memory)}, nil
		case StopReasonToolUse:
			// intercetto il tool terminale prima di executeTools
			if a.finalTool != "" {
				if call, input, ok := findTollCall(resp.Content, a.finalTool); ok {
					if verr := validateAgainstSchema(input, a.schemaFor(a.finalTool)); verr != nil {
						// output non valido: feedback → il modello ritenta
						memory.Add(*toolResult(call.ID, "output not valid: "+verr.Error()+" - call "+a.finalTool+" with the correct schema", true))
						continue
					}
					// output valido: cattura il risultato e termina il turno
					finalResult = input
					memory.Add(*toolResult(call.ID, "ok", false))
					return RunResult{FinalResult: finalResult, Text: lastAssistantText(memory)}, nil
				}
			}

			if err := a.executeTools(ctx, memory, resp.Content, em); err != nil {
				return RunResult{}, fmt.Errorf("agent: execute tools: %w", err)
			}
		case StopReasonMaxTokens:
			return RunResult{}, fmt.Errorf("agent: max_token reached")
		}
	}

	return RunResult{}, fmt.Errorf("agent: reached max iterations without completing the task (%d)", a.maxIterations)
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

func (a *Agent) SetContextLimit(limit int) {
	a.contextLimit = limit
}

func (a *Agent) SetMaxIterations(iterations int) {
	if iterations > 0 {
		a.maxIterations = iterations
	}
}

func (a *Agent) SetFinalTool(tool string) {
	a.finalTool = tool
}

func (a *Agent) executeTools(ctx context.Context, memory Memory, blocks []ContentBlock, em Emitter) error {
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
		if err := a.runPreToolUseHooks(ctx, call.Name, riskLevel, input); err != nil {
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
				results[t.idx] = a.runTool(ctx, t.call, t.executor, t.input, em)
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
		results[t.idx] = a.runTool(ctx, t.call, t.executor, t.input, em)
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

func (a *Agent) runTool(ctx context.Context, call ToolUseBlock, executor ToolExecutor, input map[string]any, em Emitter) *Message {
	em.ToolCall(call.Name, input)
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
		em.ToolResult(call.Name, result, false)
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

func (a *Agent) runPreToolUseHooks(ctx context.Context, toolName string, level RiskLevel, input map[string]any) error {
	for _, hook := range a.preToolUseHooks {
		if err := hook(ctx, toolName, level, input); err != nil {
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

func (a *Agent) schemaFor(toolName string) ToolInputSchema {
	for _, t := range a.tools {
		if t.Name == toolName {
			return t.InputSchema
		}
	}
	return ToolInputSchema{}
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

func findTollCall(blocks []ContentBlock, toolName string) (ToolUseBlock, map[string]any, bool) {
	for _, b := range blocks {
		if call, ok := b.(ToolUseBlock); ok && call.Name == toolName {
			return call, call.Input, true
		}
	}
	return ToolUseBlock{}, nil, false
}
