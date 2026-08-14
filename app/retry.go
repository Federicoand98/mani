package app

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/Federicoand98/mani/core"
)

// Decorator that retries a LLM client with exponential backoff.

type retryClient struct {
	inner       core.LLMClient
	maxAttempts int
	base        time.Duration
}

func NewRetryClient(inner core.LLMClient, maxAttempts int, base time.Duration) core.LLMClient {
	return &retryClient{inner: inner, maxAttempts: maxAttempts, base: base}
}

func (r *retryClient) Send(ctx context.Context, messages []core.Message, tools []core.ToolDefinition, tokenHandler core.TokenHandler) (core.LLMResponse, error) {
	var lastErr error

	for attempt := 0; attempt < r.maxAttempts; attempt++ {
		if attempt > 0 {
			// backoff esponenziale
			d := r.base * time.Duration(1<<uint(attempt-1))
			d += time.Duration(rand.Int63n(int64(r.base)))

			select {
			case <-ctx.Done():
				return core.LLMResponse{}, ctx.Err()
			case <-time.After(d):
			}
		}

		resp, err := r.inner.Send(ctx, messages, tools, tokenHandler)
		if err == nil {
			return resp, nil
		}

		if ctx.Err() != nil || !isRetryable(err) {
			return resp, err
		}

		lastErr = err
	}

	return core.LLMResponse{}, fmt.Errorf("retry: %d failed attempts: %w", r.maxAttempts, lastErr)
}

// ListModels: passthrough verso l'inner se supporta ModelLister. Senza questo, il
// decorator nasconde la capability e /model fallisce con "does not support model listing".
func (r *retryClient) ListModels(ctx context.Context) ([]string, error) {
	if ml, ok := r.inner.(core.ModelLister); ok {
		return ml.ListModels(ctx)
	}
	return nil, fmt.Errorf("retry: inner client does not support model listing")
}

func isRetryable(err error) bool {
	return err != nil
}
