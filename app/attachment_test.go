package app

import (
	"context"
	"testing"

	"github.com/Federicoand98/mani/core"
	"github.com/Federicoand98/mani/session"
)

// ExecuteIn took the attachments and dropped them: `mani run --image` sent the
// text alone, and the model never saw the picture.
func TestExecuteIn_SendsTheAttachments(t *testing.T) {
	var seen []core.Message
	client := core.NewMockFunc(func(msgs []core.Message, _ []core.ToolDefinition) core.LLMResponse {
		seen = msgs
		return core.RespText("a dot")
	})
	rt := testRuntime(t, client)

	img := core.ImageBlock{MediaType: "image/png", Data: []byte{1, 2, 3}}
	ch, _ := rt.ExecuteIn(context.Background(), session.New("m"), "describe", img)
	if drain(ch) {
		t.Fatal("run failed")
	}

	var found bool
	for _, m := range seen {
		for _, b := range m.Content {
			if ib, ok := b.(core.ImageBlock); ok && ib.MediaType == "image/png" {
				found = true
			}
		}
	}
	if !found {
		t.Errorf("the model never received the image: %+v", seen)
	}
}
