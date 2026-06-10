package core

import "testing"

func TestInMemory_StartsEmpty(t *testing.T) {
	m := NewInMemory()
	if len(m.Messages()) != 0 {
		t.Errorf("attesi 0 messaggi, ottenuti %d", len(m.Messages()))
	}
}

func TestInMemory_Add_IncreasesCount(t *testing.T) {
	m := NewInMemory()
	m.Add(Message{Role: RoleUser, Content: []ContentBlock{TextBlock{Text: "ciao"}}})
	if len(m.Messages()) != 1 {
		t.Errorf("atteso 1 messaggio, ottenuti %d", len(m.Messages()))
	}
}

func TestInMemory_Add_PreservesOrder(t *testing.T) {
	m := NewInMemory()
	m.Add(Message{Role: RoleUser, Content: []ContentBlock{TextBlock{Text: "primo"}}})
	m.Add(Message{Role: RoleAssistant, Content: []ContentBlock{TextBlock{Text: "secondo"}}})

	msgs := m.Messages()
	if len(msgs) != 2 {
		t.Fatalf("attesi 2 messaggi, ottenuti %d", len(msgs))
	}
	if msgs[0].Role != RoleUser {
		t.Errorf("msgs[0]: role atteso 'user', ottenuto %q", msgs[0].Role)
	}
	if msgs[1].Role != RoleAssistant {
		t.Errorf("msgs[1]: role atteso 'assistant', ottenuto %q", msgs[1].Role)
	}
}

func TestInMemory_Add_PreservesContent(t *testing.T) {
	m := NewInMemory()
	m.Add(Message{Role: RoleUser, Content: []ContentBlock{TextBlock{Text: "test"}}})

	tb, ok := m.Messages()[0].Content[0].(TextBlock)
	if !ok {
		t.Fatal("atteso TextBlock nel content")
	}
	if tb.Text != "test" {
		t.Errorf("text atteso 'test', ottenuto %q", tb.Text)
	}
}

func TestInMemory_Clear_RemovesAll(t *testing.T) {
	m := NewInMemory()
	m.Add(Message{Role: RoleUser, Content: []ContentBlock{TextBlock{Text: "ciao"}}})
	m.Add(Message{Role: RoleAssistant, Content: []ContentBlock{TextBlock{Text: "risposta"}}})
	m.Clear()

	if len(m.Messages()) != 0 {
		t.Errorf("attesi 0 messaggi dopo Clear, ottenuti %d", len(m.Messages()))
	}
}

func TestInMemory_Clear_ThenAddWorks(t *testing.T) {
	m := NewInMemory()
	m.Add(Message{Role: RoleUser, Content: []ContentBlock{TextBlock{Text: "primo"}}})
	m.Clear()
	m.Add(Message{Role: RoleUser, Content: []ContentBlock{TextBlock{Text: "secondo"}}})

	msgs := m.Messages()
	if len(msgs) != 1 {
		t.Fatalf("atteso 1 messaggio dopo Clear+Add, ottenuti %d", len(msgs))
	}
	tb, _ := msgs[0].Content[0].(TextBlock)
	if tb.Text != "secondo" {
		t.Errorf("text atteso 'secondo', ottenuto %q", tb.Text)
	}
}

func TestInMemory_ManyAdds(t *testing.T) {
	m := NewInMemory()
	const n = 50
	for i := range n {
		m.Add(Message{Role: RoleUser, Content: []ContentBlock{TextBlock{Text: "msg"}}})
		_ = i
	}
	if len(m.Messages()) != n {
		t.Errorf("attesi %d messaggi, ottenuti %d", n, len(m.Messages()))
	}
}

func TestInMemory_Add_AllRoles(t *testing.T) {
	roles := []Role{RoleUser, RoleAssistant, RoleSystem}
	m := NewInMemory()
	for _, r := range roles {
		m.Add(Message{Role: r, Content: []ContentBlock{TextBlock{Text: "x"}}})
	}
	msgs := m.Messages()
	for i, r := range roles {
		if msgs[i].Role != r {
			t.Errorf("msgs[%d]: role atteso %q, ottenuto %q", i, r, msgs[i].Role)
		}
	}
}
