// Package mcp makes mani a Model Context Protocol client.
//
// It connects to an MCP server (stdio or HTTP/SSE), lists its tools and adapts them
// to tool.Tool, so tools written in any language become available to the agent.
package mcp

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/Federicoand98/mani/core"
	"github.com/Federicoand98/mani/tool"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// ServerSpec represents an MCP server specification: stdio (Command) or remote (URL).
type ServerSpec struct {
	Name    string
	Command string
	Args    []string
	URL     string
	Risk    core.RiskLevel
}

type Session struct {
	sess  *mcpsdk.ClientSession
	tools []tool.Tool
}

func (s *Session) Tools() []tool.Tool { return s.tools }
func (s *Session) Close() error       { return s.sess.Close() }

func Connect(ctx context.Context, spec ServerSpec) (*Session, error) {
	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "mani", Version: "0.1"}, nil)

	var transport mcpsdk.Transport
	switch {
	case spec.URL != "":
		transport = &mcpsdk.SSEClientTransport{Endpoint: spec.URL}
	case spec.Command != "":
		transport = &mcpsdk.CommandTransport{Command: exec.Command(spec.Command, spec.Args...)}
	default:
		return nil, fmt.Errorf("[mcp %q]: no command and url", spec.Name)
	}

	sess, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return nil, fmt.Errorf("[mcp %q]: connect error: %w", spec.Name, err)
	}

	list, err := sess.ListTools(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("[mcp %q]: list tools error: %w", spec.Name, err)
	}

	risk := spec.Risk
	if risk == core.RiskNone { // zero value = non impostato → default prudente
		risk = core.RiskExecute
	}

	var tools []tool.Tool
	for _, t := range list.Tools {
		tools = append(tools, newMcpTool(sess, t, risk))
	}

	return &Session{sess: sess, tools: tools}, nil
}

// ------------------- MCP Tool Adapter ----------------------------
type mcpTool struct {
	sess   *mcpsdk.ClientSession
	name   string
	desc   string
	risk   core.RiskLevel
	schema tool.ToolSchema
}

func newMcpTool(sess *mcpsdk.ClientSession, t *mcpsdk.Tool, risk core.RiskLevel) *mcpTool {
	return &mcpTool{
		sess: sess, name: t.Name, desc: t.Description, risk: risk,
		schema: tool.ToolSchema{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: convertSchema(t.InputSchema),
		},
	}
}

func (m *mcpTool) Name() string              { return m.name }
func (m *mcpTool) Description() string       { return m.desc }
func (m *mcpTool) RiskLevel() core.RiskLevel { return m.risk }
func (m *mcpTool) Schema() tool.ToolSchema   { return m.schema }

func (m *mcpTool) Execute(ctx context.Context, input map[string]any) (string, error) {
	res, err := m.sess.CallTool(ctx, &mcpsdk.CallToolParams{Name: m.name, Arguments: input})
	if err != nil {
		return "", fmt.Errorf("[mcp %q]: execute error: %w", m.name, err)
	}

	var b strings.Builder
	for _, r := range res.Content {
		if tc, ok := r.(*mcpsdk.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}

	out := b.String()
	if res.IsError {
		return "", fmt.Errorf("[mcp %q]: error: %s", m.name, out)
	}

	return out, nil
}
