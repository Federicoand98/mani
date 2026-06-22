package core

import "fmt"

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
			}
		}
	}

	return chars / 4
}
