package core

import "context"

type HookType string

const (
	HookPreToolUse  HookType = "pre_tool_use"
	HookPostToolUse HookType = "post_tool_use"
	HookPreLLMCall  HookType = "pre_llm_call"
	HookPostLLMCall HookType = "post_llm_call"
)

type HookEvent struct {
	Type    HookType
	Payload any
}

type Hook func(ctx context.Context, ev HookEvent) error

type PreToolUsePayload struct {
	ToolName string
	Input    map[string]any // mutabile
}

type PostToolUsePayload struct {
	ToolName string
	Input    map[string]any
	Result   string // mutabile
	IsError  bool   // mutabile
}

type PreLLMCallPayload struct {
	Messages []Message // mutabile
	Tools    []ToolDefinition
}

type PostLLMCallPayload struct {
	Response *LLMResponse // mutabile
}

// ---- manager ----

type HookManager struct {
	hooks []Hook
}

func NewHookManager() *HookManager {
	return &HookManager{}
}

func (m *HookManager) Register(hook Hook) {
	m.hooks = append(m.hooks, hook)
}

func (m *HookManager) Fire(ctx context.Context, ev HookEvent) error {
	for _, h := range m.hooks {
		if err := h(ctx, ev); err != nil {
			return err
		}
	}
	return nil
}

func (m *HookManager) OnPreToolUse(fn func(context.Context, *PreToolUsePayload) error) {
	m.Register(func(ctx context.Context, ev HookEvent) error {
		if ev.Type != HookPreToolUse {
			return nil
		}
		return fn(ctx, ev.Payload.(*PreToolUsePayload))
	})
}

func (m *HookManager) OnPostToolUse(fn func(context.Context, *PostToolUsePayload) error) {
	m.Register(func(ctx context.Context, ev HookEvent) error {
		if ev.Type != HookPostToolUse {
			return nil
		}
		return fn(ctx, ev.Payload.(*PostToolUsePayload))
	})
}

func (m *HookManager) OnPreLLMCall(fn func(context.Context, *PreLLMCallPayload) error) {
	m.Register(func(ctx context.Context, ev HookEvent) error {
		if ev.Type != HookPreLLMCall {
			return nil
		}
		return fn(ctx, ev.Payload.(*PreLLMCallPayload))
	})
}

func (m *HookManager) OnPostLLMCall(fn func(context.Context, *PostLLMCallPayload) error) {
	m.Register(func(ctx context.Context, ev HookEvent) error {
		if ev.Type != HookPostLLMCall {
			return nil
		}
		return fn(ctx, ev.Payload.(*PostLLMCallPayload))
	})
}

type RiskLevel int

const (
	RiskNone RiskLevel = iota
	RiskWrite
	RiskExecute
)

func (r RiskLevel) String() string {
	switch r {
	case RiskWrite:
		return "write"
	case RiskExecute:
		return "execute"
	default:
		return "none"
	}
}

type PreToolUseHook func(toolName string, level RiskLevel, input map[string]any) error
