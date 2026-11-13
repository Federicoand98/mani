package core

import (
	"encoding/json"
	"fmt"
)

// EstimateTokens da una stima GREZZA dei token di un insieme di messaggi.
// Euristica: ~4 caratteri = 1 token (stima di imprecisione +- 20%) ma sufficiente per decidere quando si è al limite del contesto.
// Il numero esatto lo da sempre il provider. es ollama: prompt_eval_count
func EstimateTokens(messages []Message) int {
	chars := 0
	for _, m := range messages {
		chars += len(m.Role)
		for _, b := range m.Content {
			switch v := b.(type) {
			case TextBlock:
				chars += len(v.Text)
			case ToolUseBlock:
				chars += len(v.Name)
				for k, val := range v.Input {
					chars += len(k) + len(fmt.Sprint(val))
				}
			case ToolResultBlock:
				chars += len(v.Content)
			case ImageBlock:
				chars += (len(v.Data) / 750) * 4
			}
		}
	}

	return chars / 4
}

// EstimateRequestTokens is an estimate, not a model-specific tokenizer.
// Include tool schemas and protocol overhead; retain a margin for the text heuristic.
func EstimateRequestTokens(messages []Message, tools []ToolDefinition) (int, error) {
	encoded, err := json.Marshal(tools)
	if err != nil {
		return 0, fmt.Errorf("estimate tool schemas: %w", err)
	}
	n := EstimateTokens(messages) + (len(encoded)+3)/4 + len(messages)*12 + 8
	return n + (n+3)/4, nil
}
