package app

import (
	"context"
	"fmt"
	"unicode/utf8"

	"github.com/Federicoand98/mani/core"
)

// DefaultToolOutputLimit is the most one tool result may put in the conversation,
// in bytes: about 8k tokens. A file read whole or a long command output would
// otherwise fill a model's context window in a single call, and every request
// after it would be refused.
const DefaultToolOutputLimit = 32 << 10

// RegisterToolOutputTruncation clips every tool result longer than limit bytes
// before it enters the memory, keeping its head and its tail around a marker
// that says how much was cut and how to read the rest. Subagents spawned by rt
// get the same limit. A limit of 0 or less registers nothing.
//
// Register it after the redaction rules: a secret cut in half would no longer
// match its pattern, and its first half would reach the model.
func RegisterToolOutputTruncation(rt *Runtime, limit int) {
	if limit <= 0 {
		return
	}
	rt.toolOutputLimit = limit
	rt.OnPostToolUse(truncationHook(limit))
}

func truncationHook(limit int) func(context.Context, *core.PostToolUsePayload) error {
	return func(_ context.Context, p *core.PostToolUsePayload) error {
		p.Result = truncateOutput(p.Result, limit)
		return nil
	}
}

// truncateOutput keeps the first and the last half of limit bytes of s, cut on
// rune boundaries, with a marker in between. The head is where a file starts,
// the tail is where a command prints its errors: both are worth keeping.
func truncateOutput(s string, limit int) string {
	if len(s) <= limit {
		return s
	}

	headEnd := limit / 2
	for headEnd > 0 && !utf8.RuneStart(s[headEnd]) {
		headEnd--
	}
	tailStart := len(s) - limit/2
	for tailStart < len(s) && !utf8.RuneStart(s[tailStart]) {
		tailStart++
	}

	return fmt.Sprintf("%s\n\n... [output truncated: %d of %d bytes cut. Read less at once: a part of the file, a narrower search, a command that prints less.] ...\n\n%s",
		s[:headEnd], tailStart-headEnd, len(s), s[tailStart:])
}
