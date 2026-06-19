package session

import (
	"time"

	"github.com/Federicoand98/mani/core"
)

// unico posto che conosce il formato json dei messaggi

type sessionDTO struct {
	ID        string       `json:"id"`
	Title     string       `json:"title"`
	Model     string       `json:"model"`
	CreatedAt time.Time    `json:"created_at"`
	UpdatedAt time.Time    `json:"updated_at"`
	Messages  []messageDTO `json:"messages"`
}

type messageDTO struct {
	Role    string     `json:"role"`
	Content []blockDTO `json:"content"`
}

// è volutamente piatto -> tiene i campi di tutti i tipi di plocco con un discrimatore. Omitempty tiene il blocco pulito
type blockDTO struct {
	Type      string         `json:"type"` // "text" | "tool_use"
	Text      string         `json:"text,omitempty"`
	ID        string         `json:"id,omitempty"`
	Name      string         `json:"name,omitempty"`
	Input     map[string]any `json:"input,omitempty"`
	ToolUseID string         `json:"tool_use_id,omitempty"`
	Content   string         `json:"content,omitempty"`
	IsError   bool           `json:"is_error,omitempty"`
}

// ---- core -> DTO (scrittura)

func toSessionDTO(s *Session) sessionDTO {
	msgs := s.memory.Messages()
	out := make([]messageDTO, len(msgs))

	for i, m := range msgs {
		out[i] = toMessageDTO(m)
	}

	return sessionDTO{
		ID:        s.ID,
		Title:     s.Title,
		Model:     s.Model,
		CreatedAt: s.CreatedAt,
		UpdatedAt: s.UpdatedAt,
		Messages:  out,
	}
}

func toMessageDTO(m core.Message) messageDTO {
	blocks := make([]blockDTO, len(m.Content))

	for i, b := range m.Content {
		blocks[i] = toBlockDTO(b)
	}

	return messageDTO{Role: string(m.Role), Content: blocks}
}

func toBlockDTO(b core.ContentBlock) blockDTO {
	switch v := b.(type) {
	case core.TextBlock:
		return blockDTO{Type: v.BlockType(), Text: v.Text}
	case core.ToolUseBlock:
		return blockDTO{Type: v.BlockType(), ID: v.ID, Name: v.Name, Input: v.Input}
	case core.ToolResultBlock:
		return blockDTO{Type: v.BlockType(), ToolUseID: v.ToolUseID, Content: v.Content, IsError: v.IsError}
	default:
		return blockDTO{Type: "unknown"}
	}
}

// ---- DTO -> core (lettura)

func (d sessionDTO) toSession() *Session {
	msgs := make([]core.Message, len(d.Messages))

	for i, m := range d.Messages {
		msgs[i] = m.toCore()
	}

	return &Session{
		ID:        d.ID,
		Title:     d.Title,
		Model:     d.Model,
		CreatedAt: d.CreatedAt,
		UpdatedAt: d.UpdatedAt,
		memory:    core.NewInMemoryFrom(msgs),
	}
}

func (d messageDTO) toCore() core.Message {
	blocks := make([]core.ContentBlock, len(d.Content))

	for _, b := range d.Content {
		if cb := b.toCore(); cb != nil {
			blocks = append(blocks, cb)
		}
	}

	return core.Message{Role: core.Role(d.Role), Content: blocks}
}

func (d blockDTO) toCore() core.ContentBlock {
	switch d.Type {
	case "text":
		return core.TextBlock{Text: d.Text}
	case "tool_use":
		return core.ToolUseBlock{ID: d.ID, Name: d.Name, Input: d.Input}
	case "tool_result":
		return core.ToolResultBlock{ToolUseID: d.ToolUseID, Content: d.Content, IsError: d.IsError}
	default:
		return nil
	}
}
