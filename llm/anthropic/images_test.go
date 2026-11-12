package anthropic

import (
	"testing"

	"github.com/Federicoand98/mani/core"
)

// Image blocks used to be dropped: the model received the text alone.
func TestMapMessages_SendsImages(t *testing.T) {
	_, out := mapMessages([]core.Message{{Role: core.RoleUser, Content: []core.ContentBlock{
		core.TextBlock{Text: "what is this?"},
		core.ImageBlock{MediaType: "image/jpeg", Data: []byte("jpg")},
	}}})
	if len(out) != 1 || len(out[0].Content) != 2 {
		t.Fatalf("mapped = %+v", out)
	}
	img := out[0].Content[1]
	if img.Type != "image" || img.Source == nil || img.Source.Type != "base64" || img.Source.MediaType != "image/jpeg" || img.Source.Data != "anBn" {
		t.Errorf("image block = %+v %+v", img, img.Source)
	}
}
