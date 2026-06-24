package openai

type oaiMessage struct {
	Role       string        `json:"role"`
	Content    string        `json:"content,omitempty"`
	ToolCalls  []oaiToolCall `json:"tool_calls,omitempty"`
	ToolCallID string        `json:"tool_call_id,omitempty"`
}

type oaiToolCall struct {
	ID       string      `json:"id"`
	Type     string      `json:"type"`
	Function oaiFunction `json:"function"`
}

type oaiFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type oaiTool struct {
	Type     string    `json:"type"` // "function"
	Function oaiToolFn `json:"function"`
}

type oaiToolFn struct {
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Parameters  oaiParameters `json:"parameters"`
}

type oaiParameters struct {
	Type       string                 `json:"type"`
	Properties map[string]oaiProperty `json:"properties"`
	Required   []string               `json:"required,omitempty"`
}

type oaiProperty struct {
	Type        string `json:"type"`
	Description string `json:"description"`
}

type oaiRequest struct {
	Model         string       `json:"model"`
	Messages      []oaiMessage `json:"messages"`
	Tools         []oaiTool    `json:"tools,omitempty"`
	Stream        bool         `json:"stream,omitempty"`
	StreamOptions *streamOpts  `json:"stream_options,omitempty"`
}

type streamOpts struct {
	IncludeUsage bool `json:"include_usage,omitempty"`
}

// SSE chunks

type oaiStreamChunk struct {
	Choices []oaiChoice `json:"choices"`
	Usage   *oaiUsage   `json:"usage,omitempty"` // presente solo nell'ultimo chunk
}

type oaiChoice struct {
	Delta        oaiDelta `json:"delta"`
	FinishReason string   `json:"finish_reason"` // "stop", "tool_calls", "length"
}

type oaiDelta struct {
	Content   string             `json:"content"`
	ToolCalls []oaiToolCallDelta `json:"tool_calls"`
}

type oaiToolCallDelta struct {
	Index    int    `json:"index"` // identifica quale tool_call accumulare
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"` // frammenti da concatenare
	} `json:"function"`
}

type oaiUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

type oaiModelsResponse struct {
	Data []struct {
		ID string `json:"id"`
	} `json:"data"`
}
