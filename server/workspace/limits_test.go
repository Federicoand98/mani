package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/Federicoand98/mani/config"
	"github.com/Federicoand98/mani/core"
)

func TestProviderModelLimitsRoundTripAndMerge(t *testing.T) {
	f := newFixture(t)
	var p Provider
	if code := f.do("PUT", "/api/providers/ollama", `{"modelLimits":{"a":{"context_window":8192,"max_output_tokens":2048}}}`, &p); code != 200 {
		t.Fatalf("save: %d", code)
	}
	if code := f.do("PUT", "/api/providers/ollama", `{"modelLimits":{"b":{"max_input_tokens":6000}}}`, &p); code != 200 {
		t.Fatalf("merge: %d", code)
	}
	cfg, err := config.Load()
	if err != nil || cfg.Providers["ollama"].ModelLimits["a"].ContextWindow != 8192 || cfg.Providers["ollama"].ModelLimits["b"].MaxInputTokens != 6000 {
		t.Fatalf("saved limits: %+v, %v", cfg.Providers["ollama"].ModelLimits, err)
	}
	var list ProviderList
	f.do("GET", "/api/providers", "", &list)
	if len(list.Providers[0].ModelLimits) != 2 || p.ModelLimits["a"].MaxOutputTokens != 2048 {
		t.Fatalf("DTO limits: %+v", p.ModelLimits)
	}
}

func TestProviderModelLimitsRefusesInvalidValuesBeforeSaving(t *testing.T) {
	f := newFixture(t)
	for _, body := range []string{
		`{"modelLimits":{"":{"context_window":8000}}}`,
		`{"modelLimits":{" a":{"context_window":8000}}}`,
		`{"modelLimits":{"a":{}}}`,
		`{"modelLimits":{"a":{"max_input_tokens":-1}}}`,
		`{"modelLimits":{"a":{"context_window":1.5}}}`,
		`{"modelLimits":{"a":{"context_window":1073741825}}}`,
		`{"modelLimits":{"a":{"context_window":8000,"max_input_tokens":9000}}}`,
		`{"modelLimits":{"a":{"context_window":8000,"max_output_tokens":8000}}}`,
	} {
		var out APIError
		if code := f.do("PUT", "/api/providers/ollama", body, &out); code != 400 {
			t.Errorf("%s: %d, %s", body, code, out.Error)
		}
	}
	cfg, _ := config.Load()
	if len(cfg.Providers["ollama"].ModelLimits) != 0 {
		t.Fatal("invalid input changed config")
	}
}

func TestDiscoverModelLimitsUnsupportedAndErrors(t *testing.T) {
	f := newFixture(t)
	old := modelLimits
	t.Cleanup(func() { modelLimits = old })
	var result core.ModelLimits
	var failure error
	modelLimits = func(ctx context.Context, cfg config.Config, auth config.Auth, provider, model string) (core.ModelLimits, error) {
		if provider != "ollama" || model != "a" {
			t.Fatalf("discovery target: %s/%s", provider, model)
		}
		if _, ok := ctx.Deadline(); !ok {
			t.Error("discovery is not bounded")
		}
		return result, failure
	}
	var limits core.ModelLimits
	if code := f.do("POST", "/api/providers/ollama/model-limits", `{"model":"a"}`, &limits); code != 200 || limits != (core.ModelLimits{}) {
		t.Fatalf("unsupported: %d, %+v", code, limits)
	}
	result = core.ModelLimits{ContextWindow: 8192}
	if code := f.do("POST", "/api/providers/ollama/model-limits", `{"model":"a"}`, &limits); code != 200 || limits.ContextWindow != 8192 {
		t.Fatalf("discovered: %d, %+v", code, limits)
	}
	var out APIError
	failure = errors.New("endpoint unavailable")
	if code := f.do("POST", "/api/providers/ollama/model-limits", `{"model":"a"}`, &out); code != 502 || !strings.Contains(out.Error, "endpoint unavailable") {
		t.Fatalf("failure: %d, %+v", code, out)
	}
	failure = nil
	result = core.ModelLimits{ContextWindow: -1}
	if code := f.do("POST", "/api/providers/ollama/model-limits", `{"model":"a"}`, &out); code != 502 {
		t.Fatalf("invalid metadata: %d", code)
	}
	for _, body := range []string{`{}`, `{"model":" "}`, `{"model":"a","guess":true}`} {
		if code := f.do("POST", "/api/providers/ollama/model-limits", body, &out); code != 400 {
			t.Errorf("bad body %s: %d", body, code)
		}
	}
}

func TestConversationContextJSON(t *testing.T) {
	last := 123
	data, err := json.Marshal(Conversation{Context: &core.ContextUsage{
		EstimatedInput: 130, InputBudget: 6000, OutputLimit: 2000, ContextWindow: 8000,
		LimitSource: "mixed", LastInput: &last, State: "ready",
	}})
	if err != nil {
		t.Fatal(err)
	}
	var out struct {
		Context ContextUsage `json:"context"`
	}
	if err := json.Unmarshal(data, &out); err != nil || out.Context.InputBudget != 6000 || out.Context.LastInput == nil || *out.Context.LastInput != 123 {
		t.Fatalf("round trip: %s, %v", data, err)
	}
	if !strings.Contains(string(data), `"estimated_input":130`) {
		t.Fatalf("context protocol must retain generated snake_case fields: %s", data)
	}
}
