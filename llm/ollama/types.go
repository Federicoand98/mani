package ollama

/*
RICHIESTA:
{
  "model": "qwen2.5-coder",
  "stream": false,
  "messages": [
    { "role": "user", "content": "Leggi il file main.go" }
  ],
  "tools": [
    {
      "type": "function",
      "function": {
        "name": "read_file",
        "description": "Legge un file",
        "parameters": {
          "type": "object",
          "properties": {
            "path": { "type": "string", "description": "Il percorso del file" }
          },
          "required": ["path"]
        }
      }
    }
  ]
}

RISPOSTA:
text:
{
  "message": {
    "role": "assistant",
    "content": "Ecco il contenuto del file..."
  },
  "done_reason": "stop"
}

tool use:
{
  "message": {
    "role": "assistant",
    "content": "",
    "tool_calls": [
      {
        "function": {
          "name": "read_file",
          "arguments": { "path": "main.go" }
        }
      }
    ]
  },
  "done_reason": "stop"
}
*/

// --- Tipi per la richiesta ---

type ollamaRequest struct {
	Model    string          `json:"model"`
	Messages []ollamaMessage `json:"messages"`
	Tools    []ollamaTool    `json:"tools,omitempty"`
	Stream   bool            `json:"stream"`
}

type ollamaMessage struct {
	Role      string           `json:"role"`
	Content   string           `json:"content"`
	ToolCalls []ollamaToolCall `json:"tool_calls,omitempty"`
}

// ollamaToolCall è il formato di una tool call dentro un messaggio dell'assistente.
type ollamaToolCall struct {
	Function ollamaToolCallFunction `json:"function"`
}

type ollamaToolCallFunction struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

// ollamaTool è la definizione di un tool inviata al modello nella richiesta.
type ollamaTool struct {
	Type     string             `json:"type"`
	Function ollamaToolFunction `json:"function"`
}

type ollamaToolFunction struct {
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Parameters  ollamaParameters `json:"parameters"`
}

// ollamaParameters rispecchia tool.InputSchema ma con il tag "parameters"
// che Ollama si aspetta (a differenza di Anthropic che usa "input_schema").
type ollamaParameters struct {
	Type       string                    `json:"type"`
	Properties map[string]ollamaProperty `json:"properties"`
	Required   []string                  `json:"required"`
}

type ollamaProperty struct {
	Type        string `json:"type"`
	Description string `json:"description"`
}

// --- Tipi per la risposta ---

type ollamaResponse struct {
	Model      string        `json:"model"`
	Message    ollamaMessage `json:"message"`
	DoneReason string        `json:"done_reason"`
	Done       bool          `json:"done"`

	// Contatori token — corrispondono a llm.TokenUsage
	PromptEvalCount int `json:"prompt_eval_count"`
	EvalCount       int `json:"eval_count"`
}
