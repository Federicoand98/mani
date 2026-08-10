package core

import "strings"

func TextFrom(blocks []ContentBlock) string {
	var sb strings.Builder

	for _, block := range blocks {
		if tb, ok := block.(TextBlock); ok {
			sb.WriteString(tb.Text)
		}
	}

	return sb.String()
}

func lastAssistantText(memory Memory) string {
	msgs := memory.Messages()
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role != RoleAssistant {
			continue
		}
		for _, b := range msgs[i].Content {
			if tb, ok := b.(TextBlock); ok {
				return tb.Text
			}
		}
	}
	return ""
}
