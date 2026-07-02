package openai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/Federicoand98/mani/core"
)

type AuthFn func(ctx context.Context) (string, error)

func StaticKey(k string) AuthFn {
	return func(context.Context) (string, error) { return k, nil }
}

type Config struct {
	BaseURL string
	Model   string
	AuthFn  AuthFn
	Headers map[string]string
}

type Client struct {
	cfg  Config
	http *http.Client
}

func New(cfg Config) *Client {
	return &Client{cfg: cfg, http: &http.Client{Timeout: 120 * time.Second}}
}

func (c *Client) Send(ctx context.Context, msgs []core.Message, tools []core.ToolDefinition, h core.TokenHandler) (core.LLMResponse, error) {
	token, err := c.cfg.AuthFn(ctx)
	if err != nil {
		return core.LLMResponse{}, fmt.Errorf("openai: auth: %w", err)
	}
	body := oaiRequest{
		Model:         c.cfg.Model,
		Messages:      mapMessages(msgs),
		Tools:         mapTools(tools),
		Stream:        true,
		StreamOptions: &streamOpts{IncludeUsage: true}, // usage nel chunk finale
	}
	raw, _ := json.Marshal(body)
	req, _ := http.NewRequestWithContext(ctx, "POST", c.cfg.BaseURL+"/chat/completions", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	for k, v := range c.cfg.Headers {
		req.Header.Set(k, v)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return core.LLMResponse{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return core.LLMResponse{}, fmt.Errorf("openai: HTTP %d: %s", resp.StatusCode, b)
	}
	return c.readSSE(resp.Body, h)
}

func (c *Client) ListModels(ctx context.Context) ([]string, error) {
	token, err := c.cfg.AuthFn(ctx)
	if err != nil {
		return nil, fmt.Errorf("openai: auth: %w", err)
	}

	req, _ := http.NewRequestWithContext(ctx, "GET", c.cfg.BaseURL+"/models", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	for k, v := range c.cfg.Headers {
		req.Header.Set(k, v)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("openai: list models: HTTP %d: %s", resp.StatusCode, b)
	}

	var mr oaiModelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&mr); err != nil {
		return nil, fmt.Errorf("openai: list models: %w", err)
	}

	out := make([]string, 0, len(mr.Data))
	for _, m := range mr.Data {
		out = append(out, m.ID)
	}

	sort.Strings(out)
	return out, nil
}

func (c *Client) readSSE(body io.Reader, h core.TokenHandler) (core.LLMResponse, error) {
	var (
		text  strings.Builder
		calls = map[int]*oaiToolCall{} // index -> call in costruzione
		stop  string
		usage core.TokenUsage
	)
	sc := bufio.NewScanner(body)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024) // chunk grandi
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(line[len("data:"):])
		if data == "[DONE]" {
			break
		}
		var chunk oaiStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return core.LLMResponse{}, fmt.Errorf("openai: chunk: %w", err)
		}
		if chunk.Usage != nil {
			usage = core.TokenUsage{InputTokens: chunk.Usage.PromptTokens, OutputTokens: chunk.Usage.CompletionTokens}
		}
		if len(chunk.Choices) == 0 {
			continue
		}
		ch := chunk.Choices[0]
		if ch.FinishReason != "" {
			stop = ch.FinishReason
		}
		if ch.Delta.Content != "" {
			h(ch.Delta.Content, false) // OpenAI non separa il reasoning qui
			text.WriteString(ch.Delta.Content)
		}
		for _, tc := range ch.Delta.ToolCalls {
			cur := calls[tc.Index]
			if cur == nil {
				cur = &oaiToolCall{Function: oaiFunction{}}
				calls[tc.Index] = cur
			}
			if tc.ID != "" {
				cur.ID = tc.ID
			}
			if tc.Function.Name != "" {
				cur.Function.Name = tc.Function.Name
			}
			cur.Function.Arguments += tc.Function.Arguments // concat frammenti
		}
	}
	if err := sc.Err(); err != nil {
		return core.LLMResponse{}, err
	}

	return assemble(text.String(), calls, stop, usage), nil
}

func mapMessages(msgs []core.Message) []oaiMessage {
	result := make([]oaiMessage, 0, len(msgs))
	for _, msg := range msgs {
		switch msg.Role {

		case core.RoleSystem:
			var text string
			for _, b := range msg.Content {
				if tb, ok := b.(core.TextBlock); ok {
					text += tb.Text
				}
			}
			result = append(result, oaiMessage{Role: "system", Content: text})

		case core.RoleUser:
			var text string
			for _, b := range msg.Content {
				if tb, ok := b.(core.TextBlock); ok {
					text += tb.Text
				}
			}
			result = append(result, oaiMessage{Role: "user", Content: text})

		case core.RoleAssistant:
			// testo e tool_calls vanno nello stesso messaggio
			var text string
			var calls []oaiToolCall
			for _, b := range msg.Content {
				switch bc := b.(type) {
				case core.TextBlock:
					text += bc.Text
				case core.ToolUseBlock:
					args, _ := json.Marshal(bc.Input) // map → JSON string (OpenAI vuole stringa)
					calls = append(calls, oaiToolCall{
						ID:       bc.ID,
						Type:     "function",
						Function: oaiFunction{Name: bc.Name, Arguments: string(args)},
					})
				}
			}
			result = append(result, oaiMessage{
				Role:      "assistant",
				Content:   text,
				ToolCalls: calls,
			})

		case core.RoleTool:
			// ogni ToolResultBlock → messaggio separato con role "tool"
			for _, b := range msg.Content {
				if tr, ok := b.(core.ToolResultBlock); ok {
					content := tr.Content
					if tr.IsError {
						content = "[ERROR] " + content
					}
					result = append(result, oaiMessage{
						Role:       "tool",
						Content:    content,
						ToolCallID: tr.ToolUseID,
					})
				}
			}
		}
	}
	return result
}

func mapTools(tools []core.ToolDefinition) []oaiTool {
	if len(tools) == 0 {
		return nil
	}
	result := make([]oaiTool, len(tools))
	for i, t := range tools {
		props := make(map[string]oaiProperty, len(t.InputSchema.Properties))
		for name, p := range t.InputSchema.Properties {
			props[name] = toOaiProp(p)
		}
		result[i] = oaiTool{
			Type: "function",
			Function: oaiToolFn{
				Name:        t.Name,
				Description: t.Description,
				Parameters: oaiParameters{
					Type:       t.InputSchema.Type,
					Properties: props,
					Required:   t.InputSchema.Required,
				},
			},
		}
	}
	return result
}

func assemble(text string, calls map[int]*oaiToolCall, stop string, usage core.TokenUsage) core.LLMResponse {
	var blocks []core.ContentBlock
	if text != "" {
		blocks = append(blocks, core.TextBlock{Text: text})
	}

	// ordina per index: i tool_call arrivano in ordine ma la mappa non garantisce nulla
	indices := make([]int, 0, len(calls))
	for i := range calls {
		indices = append(indices, i)
	}
	sort.Ints(indices)

	for _, i := range indices {
		tc := calls[i]
		var input map[string]any
		_ = json.Unmarshal([]byte(tc.Function.Arguments), &input)
		blocks = append(blocks, core.ToolUseBlock{
			ID:    tc.ID,
			Name:  tc.Function.Name,
			Input: input,
		})
	}

	stopReason := core.StopReasonEndTurn
	switch stop {
	case "tool_calls":
		stopReason = core.StopReasonToolUse
	case "length":
		stopReason = core.StopReasonMaxTokens
	}

	return core.LLMResponse{Content: blocks, StopReason: stopReason, Usage: usage}
}

func toOaiProp(p core.ToolProperty) oaiProperty {
	op := oaiProperty{Type: p.Type, Description: p.Description, Required: p.Required, Enum: p.Enum}
	if p.Items != nil {
		it := toOaiProp(*p.Items)
		op.Items = &it
	}
	if len(p.Properties) > 0 {
		op.Properties = make(map[string]oaiProperty, len(p.Properties))
		for k, v := range p.Properties {
			op.Properties[k] = toOaiProp(v)
		}
	}
	return op
}
