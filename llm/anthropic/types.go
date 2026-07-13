package anthropic

const defaultMaxTokens = 4096

type antRequest struct {
	Model     string       `json:"model"`
	System    string       `json:"system"`
	Messages  []antMessage `json:"messages"`
	Tools     []antTool    `json:"tools,omitempty"`
	MaxTokens int          `json:"max_tokens"`
	Stream    bool         `json:"stream"`
}

type antMessage struct {
	Role    string       `json:"role"` // "user", "assistant"
	Content []antContent `json:"content"`
}

type antContent struct {
	Type      string         `json:"type"` // "text", "tool_use", "tool_result"
	Text      string         `json:"text,omitempty"`
	ID        string         `json:"id,omitempty"`
	Name      string         `json:"name,omitempty"`
	Input     map[string]any `json:"input,omitempty"`
	ToolUseID string         `json:"tool_use_id,omitempty"`
	Content   []antContent   `json:"content,omitempty"`
	IsError   bool           `json:"is_error,omitempty"`
}

type antTool struct {
	Name        string        `json:"name"`
	Description string        `json:"description"`
	InputSchema antToolSchema `json:"input_schema"`
}

type antToolSchema struct {
	Type       string                     `json:"type"` // "object"
	Properties map[string]antToolProperty `json:"properties"`
	Required   []string                   `json:"required,omitempty"`
}

type antToolProperty struct {
	Type        string                     `json:"type"`
	Description string                     `json:"description,omitempty"`
	Enum        []string                   `json:"enum,omitempty"`
	Items       *antToolProperty           `json:"items,omitempty"`      // se Type == "array"
	Properties  map[string]antToolProperty `json:"properties,omitempty"` // se Type == "object"
	Required    []string                   `json:"required,omitempty"`   // se Type == "object"
}
