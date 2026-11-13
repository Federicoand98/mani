package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Federicoand98/mani/core"
)

// scriptedClient fails with errs in order, then answers; it counts the calls.
type scriptedClient struct {
	errs  []error
	calls int
}

func (c *scriptedClient) Send(context.Context, []core.Message, []core.ToolDefinition, core.TokenHandler) (core.LLMResponse, error) {
	c.calls++
	if c.calls <= len(c.errs) {
		return core.LLMResponse{}, c.errs[c.calls-1]
	}
	return core.RespText("ok"), nil
}

func status(code int) error {
	return &core.ProviderError{Provider: "openai", StatusCode: code, Body: "x"}
}

func TestRetry_WhatIsRetried(t *testing.T) {
	cases := []struct {
		name      string
		errs      []error
		wantCalls int
		wantErr   bool
	}{
		{"a 400 fails at once", []error{status(400), status(400), status(400)}, 1, true},
		{"a 401 fails at once", []error{status(401)}, 1, true},
		{"a 404 fails at once", []error{status(404)}, 1, true},
		{"a 429 is retried", []error{status(429)}, 2, false},
		{"a 408 is retried", []error{status(408)}, 2, false},
		{"a 500 is retried", []error{status(500)}, 2, false},
		{"a 529 is retried", []error{status(529)}, 2, false},
		{"a network error is retried", []error{errors.New("connection refused")}, 2, false},
		{"three 503 give up", []error{status(503), status(503), status(503)}, 3, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			inner := &scriptedClient{errs: c.errs}
			_, err := NewRetryClient(inner, 3, time.Millisecond).Send(context.Background(), nil, nil, nil)
			if inner.calls != c.wantCalls {
				t.Errorf("calls: got %d, want %d", inner.calls, c.wantCalls)
			}
			if (err != nil) != c.wantErr {
				t.Errorf("error: got %v, want error %v", err, c.wantErr)
			}
		})
	}
}

// A request the provider refused is reported as it is, once, without the
// "N failed attempts" wrapper that would suggest a flaky network.
func TestRetry_PermanentErrorUnwrapped(t *testing.T) {
	inner := &scriptedClient{errs: []error{status(400)}}
	_, err := NewRetryClient(inner, 3, time.Millisecond).Send(context.Background(), nil, nil, nil)
	if err == nil || strings.Contains(err.Error(), "retry:") {
		t.Fatalf("got %v, want the provider error alone", err)
	}
	var pe *core.ProviderError
	if !errors.As(err, &pe) || pe.StatusCode != 400 {
		t.Errorf("the status must stay readable, got %v", err)
	}
}

// A provider asking to wait longer than maxRetryWait gets its error back at once.
func TestRetry_LongRetryAfterGivesUp(t *testing.T) {
	inner := &scriptedClient{errs: []error{&core.ProviderError{Provider: "anthropic", StatusCode: 429, RetryAfter: "3600"}}}
	_, err := NewRetryClient(inner, 3, time.Millisecond).Send(context.Background(), nil, nil, nil)
	if err == nil || inner.calls != 1 {
		t.Errorf("got %d calls and %v, want 1 call and the 429", inner.calls, err)
	}
}

func TestParseRetryAfter(t *testing.T) {
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		in     string
		want   time.Duration
		wantOK bool
	}{
		{"", 0, false},
		{"soon", 0, false},
		{"-1", 0, false},
		{"0", 0, true},
		{" 7 ", 7 * time.Second, true},
		{"Fri, 13 Nov 2026 12:00:30 GMT", 30 * time.Second, true},
		{"Fri, 13 Nov 2026 11:00:00 GMT", 0, true}, // a date in the past means now
	}
	for _, c := range cases {
		got, ok := parseRetryAfter(c.in, now)
		if got != c.want || ok != c.wantOK {
			t.Errorf("parseRetryAfter(%q) = %v, %v; want %v, %v", c.in, got, ok, c.want, c.wantOK)
		}
	}
}
