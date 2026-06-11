package app

type EventType string

const (
	EventToken      EventType = "token"
	EventThinking   EventType = "thinking"
	EventToolCall   EventType = "tool_call"
	EventToolResult EventType = "tool_result"
	EventDone       EventType = "done"
	EventError      EventType = "error"
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
