// Package mcpserver exposes a mani agent as an MCP server.
//
// The whole agent becomes a single MCP tool: the manifest's identity.name is the
// tool name, identity.description is what a calling model reads, and output.schema
// — when declared — becomes the tool's output schema, so the agent is a typed
// function over MCP just as it is over HTTP.
//
// Transport is stdio, which means stdout carries the JSON-RPC stream: nothing else
package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Federicoand98/mani/app"
	"github.com/Federicoand98/mani/session"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

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

	// if err := validToolName(name); err != nil {
	// 	return nil, fmt.Errorf("mcp: identity.name: %q %w", name, err)
	// }

	rt, err := app.Build(ctx, spec)
	if err != nil {
		return nil, err
	}

	s := &Server{rt: rt, spec: spec}

	s.srv = mcp.NewServer(
		&mcp.Implementation{Name: "mani", Version: version},
		&mcp.ServerOptions{
			Instructions: spec.Identity.Description,
			// Logger:       std,
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
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}
	if strings.TrimSpace(args.Task) == "" {
		return nil, fmt.Errorf("argument 'task' is required")
	}

	sess := session.New(s.rt.ModelName())

	ch, cancel := s.rt.ExecuteIn(ctx, sess, args.Task)
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
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{&mcp.TextContent{Text: runErr.Error()}},
		}, nil
	}

	res := &mcp.CallToolResult{}
	if structured != nil {
		res.StructuredContent = structured
	} else {
		res.Content = []mcp.Content{&mcp.TextContent{Text: text}}
	}
	return res, nil
}
