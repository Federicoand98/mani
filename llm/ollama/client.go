package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/Federicoand98/mani/core"
)

type OllamaClient struct {
	BaseURL    string
	Model      string
	HTTPClient *http.Client
}

func NewOllamaClient(baseURL, model string) *OllamaClient {
	return &OllamaClient{
		BaseURL:    baseURL,
		Model:      model,
		HTTPClient: &http.Client{Timeout: 120 * time.Second},
	}
}

func (c *OllamaClient) Send(ctx context.Context, messages []core.Message, tools []core.ToolDefinition) (core.LLMResponse, error) {
	// 1. cstruire richiesta ollamaRequest mappando messages e tools
	ollamaReq := ollamaRequest{
		Model:    c.Model,
		Messages: mapMessagesToOllama(messages),
		Tools:    mapToolsToOllama(tools),
		Stream:   false,
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

	// 4. leggere body
	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return core.LLMResponse{}, err
	}

	if resp.StatusCode != http.StatusOK {
		return core.LLMResponse{}, fmt.Errorf("ollama: HTTP %d: %s", resp.StatusCode, string(respBytes))
	}

	// 5. deserializza
	var ollamaResp ollamaResponse
	if err := json.Unmarshal(respBytes, &ollamaResp); err != nil {
		return core.LLMResponse{}, fmt.Errorf("ollama: unmarshal risposta: %w", err)
	}

	// 6. mappa su llm.Response
	return mapOllamaResponseToLLM(ollamaResp), nil
}

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
			props[name] = ollamaProperty{
				Type:        p.Type,
				Description: p.Description,
			}
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
				om.Role = "tool"
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
