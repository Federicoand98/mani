package llm

import (
	"context"

	"github.com/Federicoand98/mani/core"
	"github.com/Federicoand98/mani/tool"
)

type LLMClient interface {
	Send(ctx context.Context, messages []core.Message, tools []tool.Tool) (string, error)
}
