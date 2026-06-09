package llm

import "github.com/Federicoand98/mani/core"

type Response struct {
	Content    []core.ContentBlock
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
