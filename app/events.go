package app

type EventType string

const (
	EventToken             EventType = "token"
	EventThinking          EventType = "thinking"
	EventToolCall          EventType = "tool_call"
	EventToolResult        EventType = "tool_result"
	EventPermissionRequest EventType = "permission_request"
	EventDone              EventType = "done"
	EventError             EventType = "error"
	EventCancelled         EventType = "cancelled"
	EventUsage             EventType = "usage"
)

type Event struct {
	Type    EventType
	Payload any
}

type TokenPayload struct {
	Text string
}

type ToolCallPayload struct {
	Name  string
	Input map[string]any
}

type ToolCallResultPayload struct {
	Name    string
	Result  string
	IsError bool
}

type ErrorPayload struct {
	Err error
}

type PermissionRequestPayload struct {
	ToolName  string
	RiskLevel string
	Input     map[string]any
	Respond   chan Decision
	Preview   string
}

type UsagePayload struct {
	Input  int
	Output int
}

type DonePayload struct {
	Result map[string]any
	Text   string
}
