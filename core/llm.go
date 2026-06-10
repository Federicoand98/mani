package core

import "context"

type TokenHandler func(token string, isThinking bool)

type LLMClient interface {
	Send(ctx context.Context, messages []Message, tools []ToolDefinition, tokenHandler TokenHandler) (LLMResponse, error)
}

type LLMResponse struct {
	Content    []ContentBlock
	StopReason StopReason
	Usage      TokenUsage
}

type StopReason string

const (
	StopReasonEndTurn   StopReason = "end_turn"
	StopReasonToolUse   StopReason = "tool_use"
	StopReasonMaxTokens StopReason = "max_tokens"
)

type TokenUsage struct {
	InputTokens  int
	OutputTokens int
}

type ToolDefinition struct {
	Name        string
	Description string
	InputSchema ToolInputSchema
}

type ToolInputSchema struct {
	Type       string
	Properties map[string]ToolProperty
	Required   []string
}

type ToolProperty struct {
	Type        string
	Description string
}
