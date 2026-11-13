package core

import (
	"context"
	"testing"
)

func TestEstimateRequestIncludesTools(t *testing.T) {
	messages := []Message{{Role: RoleUser, Content: []ContentBlock{TextBlock{Text: "hello"}}}}
	bare, err := EstimateRequestTokens(messages, nil)
	if err != nil {
		t.Fatal(err)
	}
	withTools, err := EstimateRequestTokens(messages, []ToolDefinition{{Name: "read", Description: "Read a file", InputSchema: ToolInputSchema{Type: "object", Properties: map[string]ToolProperty{"path": {Type: "string"}}}}})
	if err != nil || withTools <= bare {
		t.Fatalf("bare=%d tools=%d err=%v", bare, withTools, err)
	}
}

func TestTokenLimitsAreRequestScoped(t *testing.T) {
	root := context.Background()
	ctx := WithTokenLimits(root, TokenLimits{ContextWindow: 128000, OutputTokens: 4096})
	if TokenLimitsFrom(root).OutputTokens != 0 || TokenLimitsFrom(ctx).OutputTokens != 4096 {
		t.Fatal("limits leaked between requests")
	}
	if err := (ModelLimits{MaxInputTokens: -1}).Validate(); err == nil {
		t.Fatal("negative limit accepted")
	}
}
