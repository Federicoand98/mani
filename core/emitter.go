package core

import (
	"fmt"
	"io"
)

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

// WriterEmitter is an emitter that writes token output to an io.Writer.
type WriterEmitter struct {
	w io.Writer
}

func NewWriterEmitter(w io.Writer) *WriterEmitter {
	return &WriterEmitter{w: w}
}

func (e *WriterEmitter) Token(text string)    { fmt.Fprint(e.w, text) }
func (e *WriterEmitter) Thinking(text string) {}
func (e *WriterEmitter) ToolCall(name string, input map[string]any) {
	fmt.Fprintf(e.w, "\n[tool: %s %v]\n", name, input)
}

func (e *WriterEmitter) ToolResult(name string, result string, isError bool) {
	if isError {
		fmt.Fprintf(e.w, "[error: %s %s]\n", name, result)
	}
}
func (e *WriterEmitter) Usage(input int, output int) {}
