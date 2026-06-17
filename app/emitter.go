package app

type channelEmitter struct {
	ch       chan<- Event
	thinking bool
}

func (e *channelEmitter) Token(text string) {
	e.ch <- Event{Type: EventToken, Payload: TokenPayload{Text: text}}
}

func (e *channelEmitter) Thinking(text string) {
	if !e.thinking {
		return
	}

	e.ch <- Event{Type: EventThinking, Payload: TokenPayload{Text: text}}
}

func (e *channelEmitter) ToolCall(name string, input map[string]any) {
	e.ch <- Event{Type: EventToolCall, Payload: ToolCallPayload{Name: name, Input: input}}
}

func (e *channelEmitter) ToolResult(name string, result string, isError bool) {
	e.ch <- Event{Type: EventToolResult, Payload: ToolCallResultPayload{Name: name, Result: result, IsError: isError}}
}
