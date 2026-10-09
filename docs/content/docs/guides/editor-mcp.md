---
title: Use it from your editor
description: Serve a manifest over MCP so Claude Desktop, Claude Code, an IDE or another agent can call your agent as a single tool.
weight: 6
---

Goal: your agent appears inside an MCP client as one tool, with the same policy, limits and
journal as every other way of running it.

Prerequisites: a manifest with a `name` and a `description` in `identity`, and an MCP client.

## Serve it

```bash
mani mcp --config reviewer.yaml
```

The process speaks JSON-RPC on stdin and stdout, so you do not run it by hand: the client starts
it. Clients are configured either with a JSON file:

```json
{
  "mcpServers": {
    "reviewer": {
      "command": "mani",
      "args": ["mcp", "--config", "/absolute/path/to/reviewer.yaml"]
    }
  }
}
```

or with a command, for Claude Code:

```bash
claude mcp add reviewer -- mani mcp --config /absolute/path/to/reviewer.yaml
```

Use an absolute path. The client decides the working directory, and a relative `--config` will
eventually be resolved somewhere you did not expect.

## The whole agent is one tool

```yaml
identity:
  name: reviewer                                   # the tool's name in the client
  description: "Reviews a Go diff and reports risky changes with file:line."
  provider: anthropic
  model: claude-sonnet-5
  prompt: !include ./prompts/reviewer.md
```

The calling model reads `description` to decide whether to use the tool, so write it for a model:
what it does, what it returns, when to reach for it. A vague description is an agent nobody calls.

Under `mani mcp` the name is required and must match `^[a-zA-Z0-9_-]{1,64}$`. That is stricter
than MCP itself, because clients pass tool names on to model APIs that refuse anything else, and
the failure would surface in the client, far from your manifest.

Declare an `output.schema` and the client sees it as the tool's output schema. The result comes
back both as structured content and as its JSON in the text, so a client that only renders text
still shows something useful.

## What the client cannot do

**Answer a permission prompt.** There is no channel for it, so a tool whose policy is `ask` is
denied. Write the manifest with `allow` and `deny`.

**Keep a conversation.** Every call is a fresh run with fresh memory. The client keeps the
context on its side; mani keeps the governance on this side.

**See your logs.** stdout carries the protocol and nothing else — a stray line breaks the
connection with an error that says nothing useful. Logs go to stderr, which the client usually
shows in a panel.

## Errors come back as answers

A failed run, a missing `task` argument or a malformed one returns a tool error the calling model
can read, not a protocol error it cannot. The model can then fix the call and retry, which is the
behaviour you want from inside an editor.

## It still leaves a record

Policy, limits and the journal apply, because they live in the runtime rather than in the
transport. Runs started by a client are journaled with source `mcp`:

```bash
mani runs --config reviewer.yaml
```

```
ID            STATUS  STARTED              DURATION  TOKENS    TOOLS  BLOCKED
4c9a1f7e2b80  ok      2026-10-09 11:21:03  6.1s      3140/210  3      0
```

An agent called from inside an editor leaves the same audit trail as one started by a trigger.

## What you should see

- The tool appears in the client under `identity.name`, with your description.
- A call returns the structured result when the manifest declares a schema.
- `mani runs` lists the call with source `mcp`.
- Nothing but JSON-RPC on stdout, even with `MANI_LOG_LEVEL=debug`.

Next: [run it as a service](../service/).
