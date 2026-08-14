package session

import (
	"fmt"
	"time"
)

// Store è la porta di persistenza per le sessioni
type Store interface {
	Save(s *Session) error
	Load(id string) (*Session, error)
	List() ([]Meta, error)
	Delete(id string) error
}

// Meta è il riassunto per elencare senza caricare tutti i messaggi
type Meta struct {
	ID        string
	Title     string
	Model     string
	UpdatedAt time.Time
}

// adapter in memory

type InMemoryStore struct {
	sessions map[string]*Session
}

func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{sessions: make(map[string]*Session)}
}

func (s *InMemoryStore) Save(session *Session) error {
	session.touch()
	s.sessions[session.ID] = session
	return nil
}

func (s *InMemoryStore) Load(id string) (*Session, error) {
	session, ok := s.sessions[id]
	if !ok {
		return nil, fmt.Errorf("session not found: %s", id)
	}
	return session, nil
}

func (s *InMemoryStore) List() ([]Meta, error) {
	out := make([]Meta, 0, len(s.sessions))

	for _, session := range s.sessions {
		out = append(out, Meta{ID: session.ID, Title: session.Title, Model: session.Model, UpdatedAt: session.UpdatedAt})
	}

	return out, nil
}

func (s *InMemoryStore) Delete(id string) error {
	delete(s.sessions, id)
	return nil
}
