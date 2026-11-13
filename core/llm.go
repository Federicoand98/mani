package core

import (
	"context"
	"fmt"
)

type TokenHandler func(token string, isThinking bool)

// ProviderError is a provider answering a request with an HTTP status other than
// 200. Adapters return it so a caller can tell a request worth repeating (429,
// 5xx) from one that fails the same way every time (400, 401, 404).
type ProviderError struct {
	Provider        string // "openai", "anthropic", "ollama"
	StatusCode      int
	Body            string
	RetryAfter      string // the raw Retry-After header, empty when absent
	ContextOverflow bool
}

func (e *ProviderError) Error() string {
	return fmt.Sprintf("%s: HTTP %d: %s", e.Provider, e.StatusCode, e.Body)
}

type LLMClient interface {
	Send(ctx context.Context, messages []Message, tools []ToolDefinition, tokenHandler TokenHandler) (LLMResponse, error)
}

type ModelLister interface {
	ListModels(ctx context.Context) ([]string, error)
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
	RiskLevel   RiskLevel
}

type ToolInputSchema struct {
	Type       string
	Properties map[string]ToolProperty
	Required   []string
}

type ToolProperty struct {
	Type        string
	Description string
	Items       *ToolProperty           // se Type == "array"
	Properties  map[string]ToolProperty // se Type == "object"
	Required    []string                // se Type == "object"
	Enum        []string                // opzionale
}
