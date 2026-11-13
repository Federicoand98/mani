package workspace

// The JSON the console reads and writes. `go generate` turns these structs into
// web/src/lib/api/types.ts, so a field added here reaches the frontend typed.

//go:generate go run github.com/gzuidhof/tygo@v0.2.21 generate --config ../../tygo.yaml

// Health is what the console checks first: who is serving it, and on which folder.
type Health struct {
	Version string `json:"version"`
	Dir     string `json:"dir"`
}

// Provider is one model provider mani knows, added by the user or not.
type Provider struct {
	Name    string `json:"name"`
	Label   string `json:"label"`
	Local   bool   `json:"local"`
	Auth    string `json:"auth"` // "none", "key" or "device"
	BaseURL string `json:"baseUrl"`
	Enabled bool   `json:"enabled"`
	KeyHint string `json:"keyHint,omitempty"` // last four characters of the saved key
	Model   string `json:"model,omitempty"`   // the default model for this provider
}

type ProviderList struct {
	Providers []Provider `json:"providers"`
}

// ProviderInput adds or edits a provider. Empty fields keep what is saved.
type ProviderInput struct {
	BaseURL string `json:"baseUrl,omitempty"`
	Key     string `json:"key,omitempty"`
	Model   string `json:"model,omitempty"`
}

// TestResult reports a connection test. A provider that refuses is not a server
// error: OK is false and Error carries the provider's own words.
type TestResult struct {
	OK     bool     `json:"ok"`
	Models []string `json:"models"`
	Error  string   `json:"error,omitempty"`
}

type ModelList struct {
	Models []string `json:"models"`
}

// DeviceStart is the code the user types at the verification URL.
type DeviceStart struct {
	UserCode        string `json:"userCode"`
	VerificationURI string `json:"verificationUri"`
	ExpiresIn       int    `json:"expiresIn"`
}

// DeviceStatus is polled by the console while the user authorizes.
type DeviceStatus struct {
	Status string `json:"status"` // "idle", "pending", "done" or "error"
	Error  string `json:"error,omitempty"`
}

// APIError is the body of every non-2xx answer under /api.
type APIError struct {
	Error string `json:"error"`
}

// ConversationMeta is one row of the conversation list.
type ConversationMeta struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Provider  string `json:"provider"`
	Model     string `json:"model"`
	UpdatedAt string `json:"updatedAt"` // RFC 3339
	Running   bool   `json:"running"`   // a reply is being written right now
}

type ConversationList struct {
	Conversations []ConversationMeta `json:"conversations"`
}

// Conversation is a conversation with its messages, as the page shows them.
type Conversation struct {
	ConversationMeta `tstype:",extends"`
	Messages         []ChatMessage `json:"messages"`
}

type ChatMessage struct {
	Role  string     `json:"role"` // "user" or "assistant"
	Parts []ChatPart `json:"parts"`
}

// ChatPart is text or an image; images come back base64-encoded.
type ChatPart struct {
	Type      string `json:"type"` // "text" or "image"
	Text      string `json:"text,omitempty"`
	MediaType string `json:"mediaType,omitempty"`
	Data      string `json:"data,omitempty"`
}

// ConversationInput creates or edits a conversation. Empty fields keep what is saved.
type ConversationInput struct {
	Title    string `json:"title,omitempty"`
	Provider string `json:"provider,omitempty"`
	Model    string `json:"model,omitempty"`
}

// AgentSummary is one row of the agent list.
type AgentSummary struct {
	Path        string   `json:"path"` // relative to the working directory, with forward slashes
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Provider    string   `json:"provider"`
	Model       string   `json:"model"`
	Tools       int      `json:"tools"`
	Triggers    []string `json:"triggers"` // "every 30m", "daily 07:00", "webhook /hook"
	Status      string   `json:"status"`   // "ok", "problems" or "invalid" (the YAML does not parse)
	Problems    int      `json:"problems"`
	UpdatedAt   string   `json:"updatedAt"`
}

type AgentList struct {
	Agents []AgentSummary `json:"agents"`
}

// AgentFile is a manifest as the editor needs it: its text, the version that
// a save must match, and what is wrong with it.
type AgentFile struct {
	Path     string    `json:"path"`
	Text     string    `json:"text"`
	Version  string    `json:"version"`
	Problems []Problem `json:"problems"`
}

// Problem is a manifest problem with its line (0 when unknown).
type Problem struct {
	Path    string `json:"path"`
	Line    int    `json:"line"`
	Message string `json:"message"`
}

// AgentWrite saves a manifest. Version is the one the editor loaded: a file
// changed since then is refused, unless Force says to overwrite it.
type AgentWrite struct {
	Text    string `json:"text"`
	Version string `json:"version,omitempty"`
	Force   bool   `json:"force,omitempty"`
}

// CheckInput validates a text that is not saved yet. Path places it, so
// !include resolves next to the file.
type CheckInput struct {
	Text string `json:"text"`
	Path string `json:"path"`
}

type CheckResult struct {
	Problems []Problem `json:"problems"`
}

// FolderList is the subfolders of one folder, for the workspace picker.
type FolderList struct {
	Path    string   `json:"path"`
	Folders []string `json:"folders"`
}
