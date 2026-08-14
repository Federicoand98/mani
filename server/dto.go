package server

import "github.com/Federicoand98/mani/app"

// client -> server
type clientMsg struct {
	Type      string `json:"type"`
	Input     string `json:"input,omitempty"`
	RequestID string `json:"request_id,omitempty"`
	Decision  string `json:"decision,omitempty"` // allow_once | allow_always | deny
}

// server -> client
type serverMsg struct {
	Type    string `json:"type"`
	Payload any    `json:"payload,omitempty"`
}

type tokenDTO struct {
	Text string `json:"text"`
}

type toolCallDTO struct {
	Name  string         `json:"name"`
	Input map[string]any `json:"input"`
}

type toolResultDTO struct {
	Name    string `json:"name"`
	Result  string `json:"result"`
	IsError bool   `json:"is_error"`
}

type usageDTO struct {
	Input  int `json:"input"`
	Output int `json:"output"`
}

type permissionRequestDTO struct {
	RequestID string         `json:"request_id"`
	ToolName  string         `json:"tool_name"`
	RiskLevel string         `json:"risk_level"`
	Input     map[string]any `json:"input"`
	Preview   string         `json:"preview"`
}

type errorDTO struct {
	Message string `json:"message"`
}

// toServerMsg traduce un app.Event in un DTO serializzabile. Ritorna ok=false per
// EventPermissionRequest, gestito a parte (serve assegnare un req_id e salvare il chan).
func toServerMsg(ev app.Event) (serverMsg, bool) {
	switch ev.Type {
	case app.EventToken:
		p := ev.Payload.(app.TokenPayload)
		return serverMsg{Type: "token", Payload: tokenDTO{Text: p.Text}}, true
	case app.EventThinking:
		p := ev.Payload.(app.TokenPayload)
		return serverMsg{Type: "thinking", Payload: tokenDTO{Text: p.Text}}, true
	case app.EventToolCall:
		p := ev.Payload.(app.ToolCallPayload)
		return serverMsg{Type: "tool_call", Payload: toolCallDTO{Name: p.Name, Input: p.Input}}, true
	case app.EventToolResult:
		p := ev.Payload.(app.ToolCallResultPayload)
		return serverMsg{Type: "tool_result", Payload: toolResultDTO{Name: p.Name, Result: p.Result, IsError: p.IsError}}, true
	case app.EventUsage:
		p := ev.Payload.(app.UsagePayload)
		return serverMsg{Type: "usage", Payload: usageDTO{Input: p.Input, Output: p.Output}}, true
	case app.EventDone:
		return serverMsg{Type: "done"}, true
	case app.EventCancelled:
		return serverMsg{Type: "cancelled"}, true
	case app.EventError:
		p, _ := ev.Payload.(app.ErrorPayload)
		msg := ""
		if p.Err != nil {
			msg = p.Err.Error()
		}
		return serverMsg{Type: "error", Payload: errorDTO{Message: msg}}, true
	default:
		return serverMsg{}, false
	}
}

func parseDecision(s string) app.Decision {
	switch s {
	case "allow_once":
		return app.AllowOnce
	case "allow_always":
		return app.AllowAlways
	default:
		return app.Deny
	}
}
