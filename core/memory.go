package core

type Memory interface {
	Messages() []Message
	Add(msg Message)
	Clear()
}

type InMemory struct {
	messages []Message
}

func NewInMemory() *InMemory {
	return &InMemory{
		messages: []Message{},
	}
}

func NewInMemoryFrom(messages []Message) *InMemory {
	if messages == nil {
		messages = []Message{}
	}

	return &InMemory{messages: messages}
}

func (m *InMemory) Messages() []Message {
	return m.messages
}

func (m *InMemory) Add(msg Message) {
	if m.messages == nil {
		m.messages = []Message{}
	}

	// TODO: qui potremmo voler fare qualcosa di più intelligente, tipo rimuovere messaggi vecchi o simili
	m.messages = append(m.messages, msg)
}

func (m *InMemory) Clear() {
	m.messages = []Message{}
}
