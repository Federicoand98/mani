package core

import (
	"context"
	"fmt"
)

// ModelLimits distinguishes input and output ceilings from a shared context window.
// Zero means that the endpoint did not declare that limit.
type ModelLimits struct {
	ContextWindow   int `json:"context_window,omitempty"`
	MaxInputTokens  int `json:"max_input_tokens,omitempty"`
	MaxOutputTokens int `json:"max_output_tokens,omitempty"`
}

func (l ModelLimits) Validate() error {
	for _, field := range []struct {
		name  string
		value int
	}{
		{"context_window", l.ContextWindow}, {"max_input_tokens", l.MaxInputTokens},
		{"max_output_tokens", l.MaxOutputTokens},
	} {
		if field.value < 0 || field.value > 1<<30 {
			return fmt.Errorf("%s must be between 0 and %d", field.name, 1<<30)
		}
	}
	return nil
}

type ModelInformer interface {
	ModelInfo(context.Context) (ModelLimits, error)
}

// TokenLimits apply to one request, not to the accumulated consumption of a run.
type TokenLimits struct {
	ContextWindow int
	OutputTokens  int
}

type tokenLimitsKey struct{}

func WithTokenLimits(ctx context.Context, limits TokenLimits) context.Context {
	return context.WithValue(ctx, tokenLimitsKey{}, limits)
}

func TokenLimitsFrom(ctx context.Context) TokenLimits {
	limits, _ := ctx.Value(tokenLimitsKey{}).(TokenLimits)
	return limits
}

type ContextUsage struct {
	EstimatedInput  int    `json:"estimated_input"`
	InputBudget     int    `json:"input_budget"`
	OutputLimit     int    `json:"output_limit"`
	ContextWindow   int    `json:"context_window"`
	MaxInputTokens  int    `json:"max_input_tokens"`
	MaxOutputTokens int    `json:"max_output_tokens"`
	LimitSource     string `json:"limit_source"`
	LastInput       *int   `json:"last_input,omitempty"`
	State           string `json:"state"`
	Compacted       bool   `json:"compacted"`
}
