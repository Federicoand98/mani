package core

type Message struct {
	Role    string // "user", "assistant", "system", "tool"
	Content string
	ToolID  string
}

type ToolCall struct {
	ID        string
	Name      string
	Arguments map[string]interface{}
}
