package openai

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Federicoand98/mani/core"
)

// Image blocks used to be dropped; a user message with an image is now a list
// of parts, and one without keeps the plain string the API has always received.
func TestMapMessages_SendsImages(t *testing.T) {
	out := mapMessages([]core.Message{
		{Role: core.RoleUser, Content: []core.ContentBlock{core.TextBlock{Text: "what is this?"}, core.ImageBlock{MediaType: "image/png", Data: []byte("png")}}},
		{Role: core.RoleUser, Content: []core.ContentBlock{core.TextBlock{Text: "plain"}}},
		{Role: core.RoleAssistant, Content: []core.ContentBlock{core.ToolUseBlock{ID: "1", Name: "read", Input: map[string]any{}}}},
	})
	b, _ := json.Marshal(out)
	got := string(b)
	for _, want := range []string{
		`{"role":"user","content":[{"type":"text","text":"what is this?"},{"type":"image_url","image_url":{"url":"data:image/png;base64,cG5n"}}]}`,
		`{"role":"user","content":"plain"}`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %s in\n%s", want, got)
		}
	}
	if strings.Contains(got, `"role":"assistant","content"`) {
		t.Errorf("an assistant message with only tool calls must have no content:\n%s", got)
	}
}
