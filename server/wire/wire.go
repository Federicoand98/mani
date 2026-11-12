// Package wire is the WebSocket protocol between mani and a client: the agent
// server (mani serve) and the browser console (mani web) speak it alike. Field
// names are part of the protocol; `go generate ./server/workspace/` turns these
// structs into TypeScript for the console.
package wire

import "github.com/Federicoand98/mani/app"

// ClientMsg is what a client sends.
type ClientMsg struct {
	Type      string  `json:"type"` // "input", "cancel" or "permission_response"
	Input     string  `json:"input,omitempty"`
	Images    []Image `json:"images,omitempty"`
	Keep      int     `json:"keep,omitempty"` // messages the model remembers (mani web only)
	RequestID string  `json:"request_id,omitempty"`
	Decision  string  `json:"decision,omitempty"` // allow_once | allow_always | deny
}

// Image travels base64-encoded.
type Image struct {
	MediaType string `json:"media_type"`
	Data      string `json:"data"`
}

// ServerMsg is what mani sends. Type names the payload: token and thinking
// carry Token, tool_call ToolCall, tool_result ToolResult, usage Usage,
// permission_request PermissionRequest, error Error; done, cancelled and busy
// carry none; structured carries the agent's JSON answer as is.
type ServerMsg struct {
	Type    string `json:"type"`
	Payload any    `json:"payload,omitempty"`
}

type Token struct {
	Text string `json:"text"`
}

type ToolCall struct {
	Name  string         `json:"name"`
	Input map[string]any `json:"input"`
}

type ToolResult struct {
	Name    string `json:"name"`
	Result  string `json:"result"`
	IsError bool   `json:"is_error"`
}

type Usage struct {
	Input  int `json:"input"`
	Output int `json:"output"`
}

type PermissionRequest struct {
	RequestID string         `json:"request_id"`
	ToolName  string         `json:"tool_name"`
	RiskLevel string         `json:"risk_level"`
	Input     map[string]any `json:"input"`
	Preview   string         `json:"preview"`
}

type Error struct {
	Message string `json:"message"`
}

// FromEvent turns a runtime event into its message. It reports false for a
// permission request, which needs an id and a reply channel, so the caller
// handles it.
func FromEvent(ev app.Event) (ServerMsg, bool) {
	switch ev.Type {
	case app.EventToken:
		p := ev.Payload.(app.TokenPayload)
		return ServerMsg{Type: "token", Payload: Token{Text: p.Text}}, true
	case app.EventThinking:
		p := ev.Payload.(app.TokenPayload)
		return ServerMsg{Type: "thinking", Payload: Token{Text: p.Text}}, true
	case app.EventToolCall:
		p := ev.Payload.(app.ToolCallPayload)
		return ServerMsg{Type: "tool_call", Payload: ToolCall{Name: p.Name, Input: p.Input}}, true
	case app.EventToolResult:
		p := ev.Payload.(app.ToolCallResultPayload)
		return ServerMsg{Type: "tool_result", Payload: ToolResult{Name: p.Name, Result: p.Result, IsError: p.IsError}}, true
	case app.EventUsage:
		p := ev.Payload.(app.UsagePayload)
		return ServerMsg{Type: "usage", Payload: Usage{Input: p.Input, Output: p.Output}}, true
	case app.EventDone:
		return ServerMsg{Type: "done"}, true
	case app.EventCancelled:
		return ServerMsg{Type: "cancelled"}, true
	case app.EventError:
		p, _ := ev.Payload.(app.ErrorPayload)
		msg := ""
		if p.Err != nil {
			msg = p.Err.Error()
		}
		return ErrorMsg(msg), true
	default:
		return ServerMsg{}, false
	}
}

func ErrorMsg(s string) ServerMsg {
	return ServerMsg{Type: "error", Payload: Error{Message: s}}
}

// ParseDecision maps a permission_response; anything unknown is a deny.
func ParseDecision(s string) app.Decision {
	switch s {
	case "allow_once":
		return app.AllowOnce
	case "allow_always":
		return app.AllowAlways
	default:
		return app.Deny
	}
}
