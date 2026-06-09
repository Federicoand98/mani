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
