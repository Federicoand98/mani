package app

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Federicoand98/mani/core"
)

// Decorator that retries a LLM client with exponential backoff.

// maxRetryWait is the longest Retry-After the client waits for. A provider asking
// for more gets the error back at once: holding a run for minutes is worse than
// failing it, and the scheduler has its own retries for unattended runs.
const maxRetryWait = 60 * time.Second

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
			d, ok := r.delay(attempt, lastErr)
			if !ok {
				return core.LLMResponse{}, lastErr
			}

			select {
			case <-ctx.Done():
				return core.LLMResponse{}, ctx.Err()
			case <-time.After(d):
			}
		}

		streamed := false
		var handler core.TokenHandler
		if tokenHandler != nil {
			handler = func(s string, thinking bool) { streamed = true; tokenHandler(s, thinking) }
		}
		resp, err := r.inner.Send(ctx, messages, tools, handler)
		if err == nil {
			return resp, nil
		}

		if streamed || ctx.Err() != nil || !isRetryable(err) {
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

// ModelInfo preserves the inner client's optional metadata capability.
func (r *retryClient) ModelInfo(ctx context.Context) (core.ModelLimits, error) {
	if mi, ok := r.inner.(core.ModelInformer); ok {
		return mi.ModelInfo(ctx)
	}
	return core.ModelLimits{}, nil
}

// delay is the wait before retry number attempt: exponential backoff with jitter,
// or the provider's Retry-After when it asks for longer. ok is false when the
// provider asks to wait more than maxRetryWait.
func (r *retryClient) delay(attempt int, err error) (time.Duration, bool) {
	d := r.base * time.Duration(1<<uint(attempt-1))
	if r.base > 0 {
		d += time.Duration(rand.Int63n(int64(r.base)))
	}

	var pe *core.ProviderError
	if errors.As(err, &pe) {
		if ra, ok := parseRetryAfter(pe.RetryAfter, time.Now()); ok {
			if ra > maxRetryWait {
				return 0, false
			}
			d = max(d, ra)
		}
	}
	return d, true
}

// parseRetryAfter reads a Retry-After header: delay seconds or an HTTP date.
func parseRetryAfter(v string, now time.Time) (time.Duration, bool) {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0, false
	}
	if s, err := strconv.Atoi(v); err == nil && s >= 0 {
		return time.Duration(s) * time.Second, true
	}
	if t, err := http.ParseTime(v); err == nil {
		return max(t.Sub(now), 0), true
	}
	return 0, false
}

// isRetryable says whether the same request may succeed if sent again. When the
// provider answered, its status decides: 408, 409, 429 and 5xx are transient,
// the set the official OpenAI and Anthropic SDKs retry; any other 4xx fails the
// same way again, a prompt too long for the model's context included. An error
// without a status is the network or a stream cut short, worth another try.
func isRetryable(err error) bool {
	var pe *core.ProviderError
	if errors.As(err, &pe) {
		if pe.ContextOverflow {
			return false
		}
		switch {
		case pe.StatusCode == http.StatusRequestTimeout,
			pe.StatusCode == http.StatusConflict,
			pe.StatusCode == http.StatusTooManyRequests,
			pe.StatusCode >= 500:
			return true
		}
		return false
	}
	return err != nil
}
