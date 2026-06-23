package core

type Emitter interface {
	Token(text string)
	Thinking(text string)
	ToolCall(name string, input map[string]any)
	ToolResult(name string, result string, isError bool)
	Usage(input int, output int)
}

type nopEmitter struct{}

func (nopEmitter) Token(text string)                                   {}
func (nopEmitter) Thinking(text string)                                {}
func (nopEmitter) ToolCall(name string, input map[string]any)          {}
func (nopEmitter) ToolResult(name string, result string, isError bool) {}
func (nopEmitter) Usage(input int, output int)                         {}
