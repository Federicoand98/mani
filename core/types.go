package core

// Gli LLM vogliono i dati nel seguente modo:
/*
Richiesta:
{
	"role": "user",
	"content": [
		{ "type": "text", "text": "Leggi questo file" }
	]
}

Risposta:
{
	"role": "assistant",
	"content": [
		{ "type": "text", "text": "Certo, lo leggo" },
		{ "type": "tool_use", "id": "abc123", "name": "read_file", "input": {"path": "main.go"} }
	]
}
*/

// Role rappresenta il ruolo di un messaggio.
type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleSystem    Role = "system"
	RoleTool      Role = "tool"
)

// ContentBlock rappresenta un blocco di contenuto generico.
type ContentBlock interface {
	BlockType() string
}

// TextBlock rappresenta un blocco di testo generico.
type TextBlock struct {
	Text string
}

func (t TextBlock) BlockType() string {
	return "text"
}

// ToolUseBlock rappresenta un blocco di utilizzo di un tool.
type ToolUseBlock struct {
	ID    string
	Name  string
	Input map[string]any
}

func (t ToolUseBlock) BlockType() string {
	return "tool_use"
}

// ToolResultBlock rappresenta un blocco di risultato di un tool.
type ToolResultBlock struct {
	ToolUseID string
	Content   string
	IsError   bool
}

func (t ToolResultBlock) BlockType() string {
	return "tool_result"
}

type ImageBlock struct {
	MediaType string // image/png, image/jpeg, image/gif, image/webp
	Data      []byte
}

func (t ImageBlock) BlockType() string {
	return "image"
}

// Message rappresenta un messaggio generico.
type Message struct {
	Role    Role // "user", "assistant", "system", "tool"
	Content []ContentBlock
}
