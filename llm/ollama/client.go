package ollama

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

type OllamaClient struct {
	BaseURL    string
	Model      string
	HTTPClient *http.Client
}

var (
	_ core.LLMClient   = (*OllamaClient)(nil)
	_ core.ModelLister = (*OllamaClient)(nil)
)

func NewOllamaClient(baseURL, model string) *OllamaClient {
	return &OllamaClient{
		BaseURL:    baseURL,
		Model:      model,
		HTTPClient: &http.Client{Timeout: 120 * time.Second},
	}
}

// ListModels interroga /api/tags per i modelli installati localmente su Ollama.
func (c *OllamaClient) ListModels(ctx context.Context) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.BaseURL+"/api/tags", nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ollama: tags HTTP %d: %s", resp.StatusCode, string(body))
	}

	var tags struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tags); err != nil {
		return nil, fmt.Errorf("ollama: decode tags: %w", err)
	}

	out := make([]string, 0, len(tags.Models))
	for _, m := range tags.Models {
		out = append(out, m.Name)
	}
	sort.Strings(out)
	return out, nil
}

func (c *OllamaClient) Send(ctx context.Context, messages []core.Message, tools []core.ToolDefinition, tokenHandler core.TokenHandler) (core.LLMResponse, error) {
	// 1. cstruire richiesta ollamaRequest mappando messages e tools
	ollamaReq := ollamaRequest{
		Model:    c.Model,
		Messages: mapMessagesToOllama(messages),
		Tools:    mapToolsToOllama(tools),
		Stream:   tokenHandler != nil,
	}

	// 2. serializzare json
	ollamaReqBytes, err := json.Marshal(ollamaReq)
	if err != nil {
		return core.LLMResponse{}, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.BaseURL+"/api/chat", bytes.NewReader(ollamaReqBytes))
	if err != nil {
		return core.LLMResponse{}, err
	}

	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return core.LLMResponse{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return core.LLMResponse{}, fmt.Errorf("ollama: HTTP %d: %s", resp.StatusCode, string(body))
	}

	if tokenHandler != nil {
		return c.readStream(resp.Body, tokenHandler)
	}
	return c.readBlocking(resp.Body)
}

func (c *OllamaClient) readBlocking(body io.Reader) (core.LLMResponse, error) {
	respBytes, err := io.ReadAll(body)
	if err != nil {
		return core.LLMResponse{}, err
	}

	var ollamaResp ollamaResponse
	if err := json.Unmarshal(respBytes, &ollamaResp); err != nil {
		return core.LLMResponse{}, fmt.Errorf("ollama: unmarshal risposta: %w", err)
	}

	return mapOllamaResponseToLLM(ollamaResp), nil
}

func (c *OllamaClient) readStream(body io.Reader, handler core.TokenHandler) (core.LLMResponse, error) {
	var (
		fullContent  strings.Builder
		allToolCalls []ollamaToolCall
		lastChunk    ollamaStreamChunk
	)

	scanner := bufio.NewScanner(body)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var chunk ollamaStreamChunk
		if err := json.Unmarshal(line, &chunk); err != nil {
			return core.LLMResponse{}, fmt.Errorf("ollama: unmarshal chunk: %w", err)
		}

		if chunk.Message.Thinking != "" {
			handler(chunk.Message.Thinking, true)
		}

		if chunk.Message.Content != "" {
			handler(chunk.Message.Content, false)
			fullContent.WriteString(chunk.Message.Content)
		}

		allToolCalls = append(allToolCalls, chunk.Message.ToolCalls...)

		if chunk.Done {
			lastChunk = chunk
			break
		}
	}

	if err := scanner.Err(); err != nil {
		return core.LLMResponse{}, fmt.Errorf("ollama: read stream: %w", err)
	}

	synthetic := ollamaResponse{
		Message: ollamaMessage{
			Role:      "assistant",
			Content:   fullContent.String(),
			ToolCalls: allToolCalls,
		},
		DoneReason:      lastChunk.DoneReason,
		PromptEvalCount: lastChunk.PromptEvalCount,
		EvalCount:       lastChunk.EvalCount,
	}

	return mapOllamaResponseToLLM(synthetic), nil
}

// ---------------------------------------------------------
// -------------------- Utility mapping --------------------
// ---------------------------------------------------------

func mapOllamaResponseToLLM(resp ollamaResponse) core.LLMResponse {
	var blocks []core.ContentBlock

	if resp.Message.Content != "" {
		blocks = append(blocks, core.TextBlock{Text: resp.Message.Content})
	}

	for i, tc := range resp.Message.ToolCalls {
		blocks = append(blocks, core.ToolUseBlock{
			ID:    fmt.Sprintf("call_%d", i),
			Name:  tc.Function.Name,
			Input: tc.Function.Arguments,
		})
	}

	stopReason := core.StopReasonEndTurn
	if len(resp.Message.ToolCalls) > 0 {
		stopReason = core.StopReasonToolUse
	}

	return core.LLMResponse{
		Content:    blocks,
		StopReason: stopReason,
		Usage: core.TokenUsage{
			InputTokens:  resp.PromptEvalCount,
			OutputTokens: resp.EvalCount,
		},
	}
}

func mapToolsToOllama(tools []core.ToolDefinition) []ollamaTool {
	if len(tools) == 0 {
		return nil
	}
	result := make([]ollamaTool, len(tools))
	for i, t := range tools {
		props := make(map[string]ollamaProperty, len(t.InputSchema.Properties))
		for name, p := range t.InputSchema.Properties {
			props[name] = toOllamaProp(p)
		}
		result[i] = ollamaTool{
			Type: "function",
			Function: ollamaToolFunction{
				Name:        t.Name,
				Description: t.Description,
				Parameters: ollamaParameters{
					Type:       t.InputSchema.Type,
					Properties: props,
					Required:   t.InputSchema.Required,
				},
			},
		}
	}
	return result
}

func mapMessagesToOllama(messages []core.Message) []ollamaMessage {
	result := make([]ollamaMessage, 0, len(messages))
	for _, msg := range messages {
		om := ollamaMessage{Role: string(msg.Role)}

		for _, block := range msg.Content {
			switch b := block.(type) {
			case core.TextBlock:
				om.Content += b.Text

			case core.ToolUseBlock:
				om.ToolCalls = append(om.ToolCalls, ollamaToolCall{
					Function: ollamaToolCallFunction{
						Name:      b.Name,
						Arguments: b.Input,
					},
				})

			case core.ToolResultBlock:
				if b.IsError {
					om.Content = "[ERROR] " + b.Content
				} else {
					om.Content = b.Content
				}
			}
		}

		result = append(result, om)
	}
	return result
}

func toOllamaProp(p core.ToolProperty) ollamaProperty {
	op := ollamaProperty{Type: p.Type, Description: p.Description, Required: p.Required, Enum: p.Enum}
	if p.Items != nil {
		it := toOllamaProp(*p.Items)
		op.Items = &it
	}

	if len(p.Properties) > 0 {
		op.Properties = make(map[string]ollamaProperty, len(p.Properties))
		for k, v := range p.Properties {
			op.Properties[k] = toOllamaProp(v)
		}
	}
	return op
}
