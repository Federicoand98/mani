package app

import "strings"

func renderPreview(input map[string]any) string {
	if oldC, ok := input["old_content"].(string); ok {
		newC, _ := input["new_content"].(string)
		return diffBlock(oldC, newC)
	}

	if content, ok := input["content"].(string); ok {
		return diffBlock("", content)
	}

	return ""
}

func diffBlock(oldC, newC string) string {
	var b strings.Builder
	if oldC != "" {
		for _, line := range strings.Split(oldC, "\n") {
			b.WriteString("- " + line + "\n")
		}
	}

	if newC != "" {
		for _, line := range strings.Split(newC, "\n") {
			b.WriteString("+ " + line + "\n")
		}
	}

	return b.String()
}
