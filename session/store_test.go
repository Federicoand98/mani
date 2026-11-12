package session

import (
	"sync"
	"testing"

	"github.com/Federicoand98/mani/core"
)

// Every run of a Runtime saves into the same store, and runs are concurrent:
// triggers, server clients, batch jobs. Under -race this fails without the lock.
func TestInMemoryStore_ConcurrentUse(t *testing.T) {
	st := NewInMemoryStore()
	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() {
			s := New("test-model")
			_ = st.Save(s)
			_, _ = st.Load(s.ID)
			_, _ = st.List()
			_ = st.Delete(s.ID)
		})
	}
	wg.Wait()
}

// A reloaded session must hold exactly the blocks that were saved: no leading
// nils (the slice was once sized and then appended to), and images survive.
func TestFileStore_RoundTripKeepsEveryBlock(t *testing.T) {
	st, err := NewFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s := New("qwen3.5:9b")
	s.Provider = "ollama"
	png := []byte{0x89, 'P', 'N', 'G'}
	s.Memory().Add(core.Message{Role: core.RoleUser, Content: []core.ContentBlock{
		core.TextBlock{Text: "what is in this picture?"},
		core.ImageBlock{MediaType: "image/png", Data: png},
	}})
	s.Memory().Add(core.Message{Role: core.RoleAssistant, Content: []core.ContentBlock{core.TextBlock{Text: "a cat"}}})
	if err := st.Save(s); err != nil {
		t.Fatal(err)
	}

	got, err := st.Load(s.ID)
	if err != nil {
		t.Fatal(err)
	}
	msgs := got.Memory().Messages()
	if len(msgs) != 2 || len(msgs[0].Content) != 2 || len(msgs[1].Content) != 1 {
		t.Fatalf("blocks after reload: %+v", msgs)
	}
	img, ok := msgs[0].Content[1].(core.ImageBlock)
	if !ok || img.MediaType != "image/png" || string(img.Data) != string(png) {
		t.Errorf("image lost: %#v", msgs[0].Content[1])
	}
	if got.Provider != "ollama" {
		t.Errorf("provider = %q", got.Provider)
	}
	if list, _ := st.List(); list[0].Provider != "ollama" {
		t.Errorf("Meta.Provider = %q", list[0].Provider)
	}
}

// A conversation created empty (mani web does this) must still get its title
// from the first message.
func TestTitle_DerivedAfterAnEmptySave(t *testing.T) {
	st := NewInMemoryStore()
	s := New("m")
	_ = st.Save(s)
	s.Memory().Add(core.Message{Role: core.RoleUser, Content: []core.ContentBlock{core.TextBlock{Text: "plan the trip"}}})
	_ = st.Save(s)
	if s.Title != "plan the trip" {
		t.Errorf("title = %q", s.Title)
	}
}
