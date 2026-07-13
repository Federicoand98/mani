package anthropic

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Federicoand98/mani/core"
)

// Config: parametri dell'adapter Anthropic (indipendente da config.Config globale,
// come openai.Config). Il factory lo popola da provider/model/credenziali.
type Config struct {
	BaseURL string
	Model   string
	APIKey  string
}

type Client struct {
	cfg  Config
	http *http.Client
}

func New(cfg Config) *Client {
	return &Client{cfg: cfg, http: &http.Client{Timeout: 120 * time.Second}}
}

func (c *Client) Send(ctx context.Context, msgs []core.Message, tools []core.ToolDefinition, h core.TokenHandler) (core.LLMResponse, error) {
	system, messages := mapMessages(msgs)
	body := antRequest{
		Model:     c.cfg.Model,
		System:    system,
		Messages:  messages,
		Tools:     mapTools(tools),
		MaxTokens: defaultMaxTokens,
		Stream:    true,
	}
	raw, _ := json.Marshal(body)

	req, _ := http.NewRequestWithContext(ctx, "POST", c.cfg.BaseURL+"/v1/messages", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.cfg.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := c.http.Do(req)
	if err != nil {
		return core.LLMResponse{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return core.LLMResponse{}, fmt.Errorf("anthropic: HTTP %d: %s", resp.StatusCode, b)
	}
	return c.readSSE(resp.Body, h)
}

func mapMessages(msgs []core.Message) (string, []antMessage) {
	var system string
	var out []antMessage

	for _, msg := range msgs {
		switch msg.Role {

		case core.RoleSystem:
			for _, b := range msg.Content {
				if tb, ok := b.(core.TextBlock); ok {
					system += tb.Text
				}
			}

		case core.RoleUser:
			var blocks []antContent
			for _, b := range msg.Content {
				if tb, ok := b.(core.TextBlock); ok {
					blocks = append(blocks, antContent{Type: "text", Text: tb.Text})
				}
			}
			out = append(out, antMessage{Role: "user", Content: blocks})

		case core.RoleAssistant:
			var blocks []antContent
			for _, b := range msg.Content {
				switch bc := b.(type) {
				case core.TextBlock:
					blocks = append(blocks, antContent{Type: "text", Text: bc.Text})
				case core.ToolUseBlock:
					blocks = append(blocks, antContent{
						Type: "tool_use", ID: bc.ID, Name: bc.Name, Input: bc.Input,
					})
				}
			}
			out = append(out, antMessage{Role: "assistant", Content: blocks})

		case core.RoleTool:
			// tool_result è un blocco in un messaggio "user"
			var blocks []antContent
			for _, b := range msg.Content {
				if tr, ok := b.(core.ToolResultBlock); ok {
					blocks = append(blocks, antContent{
						Type:      "tool_result",
						ToolUseID: tr.ToolUseID,
						Content:   []antContent{{Type: "text", Text: tr.Content}},
						IsError:   tr.IsError,
					})
				}
			}
			out = append(out, antMessage{Role: "user", Content: blocks})
		}
	}
	return system, out
}

func mapTools(tools []core.ToolDefinition) []antTool {
	if len(tools) == 0 {
		return nil
	}
	result := make([]antTool, len(tools))
	for i, t := range tools {
		result[i] = antTool{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: antToolSchema{
				Type:       t.InputSchema.Type,
				Properties: mapProps(t.InputSchema.Properties),
				Required:   t.InputSchema.Required,
			},
		}
	}
	return result
}

func mapProps(props map[string]core.ToolProperty) map[string]antToolProperty {
	if len(props) == 0 {
		return nil
	}
	out := make(map[string]antToolProperty, len(props))
	for name, p := range props {
		out[name] = mapProp(p)
	}
	return out
}

// mapProp converte ricorsivamente una ToolProperty (array → Items, object → Properties/Required, enum).
func mapProp(p core.ToolProperty) antToolProperty {
	ap := antToolProperty{
		Type:        p.Type,
		Description: p.Description,
		Enum:        p.Enum,
		Required:    p.Required,
	}
	if p.Items != nil {
		it := mapProp(*p.Items)
		ap.Items = &it
	}
	if len(p.Properties) > 0 {
		ap.Properties = mapProps(p.Properties)
	}
	return ap
}

type antStreamEvent struct {
	Type         string `json:"type"`
	Index        int    `json:"index"`
	ContentBlock *struct {
		Type string `json:"type"` // "text" | "tool_use"
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"content_block,omitempty"`
	Delta *struct {
		Type        string `json:"type"` // "text_delta" | "input_json_delta"
		Text        string `json:"text"`
		PartialJSON string `json:"partial_json"`
		StopReason  string `json:"stop_reason"` // su message_delta
	} `json:"delta,omitempty"`
	Message *struct {
		Usage antUsage `json:"usage"`
	} `json:"message,omitempty"` // su message_start
	Usage *antUsage `json:"usage,omitempty"` // su message_delta
}

type antUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

func (c *Client) readSSE(body io.Reader, h core.TokenHandler) (core.LLMResponse, error) {
	var (
		text   strings.Builder
		blocks []core.ContentBlock
		// tool_use in costruzione
		curToolID   string
		curToolName string
		curToolJSON strings.Builder
		inTool      bool
		stop        string
		usage       core.TokenUsage
	)

	flushTool := func() {
		if !inTool {
			return
		}
		var input map[string]any
		_ = json.Unmarshal([]byte(curToolJSON.String()), &input)
		blocks = append(blocks, core.ToolUseBlock{ID: curToolID, Name: curToolName, Input: input})
		inTool = false
		curToolJSON.Reset()
	}

	sc := bufio.NewScanner(body)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if !strings.HasPrefix(line, "data:") {
			continue // ignora le righe "event:" e quelle vuote
		}
		data := strings.TrimSpace(line[len("data:"):])

		var ev antStreamEvent
		if err := json.Unmarshal([]byte(data), &ev); err != nil {
			return core.LLMResponse{}, fmt.Errorf("anthropic: event: %w", err)
		}

		switch ev.Type {
		case "message_start":
			if ev.Message != nil {
				usage.InputTokens = ev.Message.Usage.InputTokens
			}
		case "content_block_start":
			if ev.ContentBlock != nil && ev.ContentBlock.Type == "tool_use" {
				flushTool() // sicurezza
				inTool = true
				curToolID = ev.ContentBlock.ID
				curToolName = ev.ContentBlock.Name
			}
		case "content_block_delta":
			if ev.Delta == nil {
				break
			}
			switch ev.Delta.Type {
			case "text_delta":
				h(ev.Delta.Text, false)
				text.WriteString(ev.Delta.Text)
			case "input_json_delta":
				curToolJSON.WriteString(ev.Delta.PartialJSON)
			}
		case "content_block_stop":
			flushTool()
		case "message_delta":
			if ev.Delta != nil && ev.Delta.StopReason != "" {
				stop = ev.Delta.StopReason
			}
			if ev.Usage != nil {
				usage.OutputTokens = ev.Usage.OutputTokens
			}
		case "message_stop":
			// fine
		}
	}
	if err := sc.Err(); err != nil {
		return core.LLMResponse{}, err
	}
	flushTool() // se lo stream finisce senza content_block_stop

	// il testo va messo davanti ai tool_use, nell'ordine d'arrivo basta anteporlo
	if text.Len() > 0 {
		blocks = append([]core.ContentBlock{core.TextBlock{Text: text.String()}}, blocks...)
	}

	stopReason := core.StopReasonEndTurn
	switch stop {
	case "tool_use":
		stopReason = core.StopReasonToolUse
	case "max_tokens":
		stopReason = core.StopReasonMaxTokens
	}

	return core.LLMResponse{Content: blocks, StopReason: stopReason, Usage: usage}, nil
}
