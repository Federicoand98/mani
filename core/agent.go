package core

import (
	"context"
	"fmt"
)

type Agent struct {
	Client        LLMClient
	streamHandler TokenHandler
}

func NewAgent(client LLMClient) *Agent {
	return &Agent{Client: client}
}

func (a *Agent) Run(ctx context.Context, memory Memory, userInput string) error {
	memory.Add(Message{
		Role:    RoleUser,
		Content: []ContentBlock{TextBlock{Text: userInput}},
	})

	resp, err := a.Client.Send(ctx, memory.Messages(), nil, a.streamHandler)
	if err != nil {
		return fmt.Errorf("agent: %w", err)
	}

	memory.Add(Message{
		Role:    RoleAssistant,
		Content: resp.Content,
	})

	if resp.StopReason == StopReasonMaxTokens {
		return fmt.Errorf("agent: max_token reached")
	}

	return nil
}

func (a *Agent) SetStreamHandler(handler TokenHandler) {
	a.streamHandler = handler
}
