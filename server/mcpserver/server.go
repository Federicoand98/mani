// Package mcpserver exposes a mani agent as an MCP server.
//
// The whole agent becomes a single MCP tool: the manifest's identity.name is the
// tool name, identity.description is what a calling model reads, and output.schema
// — when declared — becomes the tool's output schema, so the agent is a typed
// function over MCP just as it is over HTTP.
//
// Transport is stdio, which means stdout carries the JSON-RPC stream: nothing else
// may ever be written there.
package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strings"

	"github.com/Federicoand98/mani/app"
	"github.com/Federicoand98/mani/session"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var toolNamePattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

func validToolName(name string) error {
	if !toolNamePattern.MatchString(name) {
		return fmt.Errorf("must be 1-64 characters from a-z, A-Z, 0-9, '_' and '-'")
	}
	return nil
}

type Server struct {
	rt   *app.Runtime
	spec app.RuntimeSpec
	srv  *mcp.Server
}

func New(ctx context.Context, spec app.RuntimeSpec, version string) (*Server, error) {
	name := spec.Identity.Name
	if name == "" {
		return nil, fmt.Errorf("mcp: identity.name is required to expose the agent as an MCP tool")
	}

	if err := validToolName(name); err != nil {
		return nil, fmt.Errorf("mcp: identity.name %q: %w", name, err)
	}

	rt, err := app.Build(ctx, spec)
	if err != nil {
		return nil, err
	}

	s := &Server{rt: rt, spec: spec}

	s.srv = mcp.NewServer(
		&mcp.Implementation{Name: "mani", Version: version},
		&mcp.ServerOptions{
			Instructions: spec.Identity.Description,
			Logger: slog.Default(),
		},
	)

	tool := &mcp.Tool{
		Name:        name,
		Description: spec.Identity.Description,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"task": map[string]any{
					"type":        "string",
					"description": "the task to give the agent",
				},
			},
			"required": []string{"task"},
		},
	}

	if spec.Output.Schema.Type != "" {
		tool.OutputSchema = spec.Output.Schema
	}

	s.srv.AddTool(tool, s.handleCall)
	return s, nil
}

func (s *Server) Close() {
	s.rt.Close()
}

func (s *Server) Run(ctx context.Context) error {
	return s.srv.Run(ctx, &mcp.StdioTransport{})
}

func (s *Server) handleCall(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args struct {
		Task string `json:"task"`
	}

	if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
		return toolError(fmt.Errorf("invalid arguments, want an object with a string \"task\": %w", err)), nil
	}
	if strings.TrimSpace(args.Task) == "" {
		return toolError(fmt.Errorf("argument \"task\" is required")), nil
	}

	sess := session.New(s.rt.ModelName())

	ch, cancel := s.rt.ExecuteIn(app.WithSource(ctx, "mcp"), sess, args.Task)
	defer cancel()

	var (
		text       string
		structured map[string]any
		runErr     error
	)

	for ev := range ch {
		switch ev.Type {
		case app.EventPermissionRequest:
			ev.Payload.(app.PermissionRequestPayload).Respond <- app.Deny

		case app.EventDone:
			p := ev.Payload.(app.DonePayload)
			text, structured = p.Text, p.Result

		case app.EventError:
			if p, ok := ev.Payload.(app.ErrorPayload); ok {
				runErr = p.Err
			}
		}
	}

	if runErr != nil {
		return toolError(runErr), nil
	}

	if structured == nil {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}, nil
	}

	b, err := json.Marshal(structured)
	if err != nil {
		return toolError(fmt.Errorf("encode structured result: %w", err)), nil
	}
	return &mcp.CallToolResult{
		StructuredContent: structured,
		Content:           []mcp.Content{&mcp.TextContent{Text: string(b)}},
	}, nil
}

func toolError(err error) *mcp.CallToolResult {
	res := &mcp.CallToolResult{}
	res.SetError(err)
	return res
}
