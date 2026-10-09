---
title: Built-in tools
description: The tools mani ships with, their arguments, their risk levels and the guards that apply to each one.
weight: 4
---

Declare a tool to enable it. Nothing is implicit, including `planning` and `delegate`:

```yaml
capabilities:
  workspace: .
  tools: [read, edit, write, delete, glob, grep, bash, fetch, planning, delegate]
```

A tool's name in the manifest is its name at run time, so `policy` entries, subagent tool lists
and journal records all use the same word.

| Tool | Risk | Arguments |
|---|---|---|
| [`read`](#read) | none | `path` |
| [`glob`](#glob) | none | `pattern` |
| [`grep`](#grep) | none | `pattern`, `path`, `include` |
| [`write`](#write) | write | `path`, `content` |
| [`edit`](#edit) | write | `path`, `old_content`, `new_content` |
| [`delete`](#delete) | write | `path` |
| [`fetch`](#fetch) | network | `url` |
| [`bash`](#bash) | execute | `command` |
| [`planning`](#planning) | none | the agent's own checklist |
| [`delegate`](#delegate) | none | `task`, and `agent` when subagents are named |

## The workspace

Every filesystem tool is confined to `capabilities.workspace`, which defaults to the working
directory. Paths are relative to it, and a path that would leave it is refused after cleaning, so
neither `../../etc/passwd` nor a symlink pointing outside gets through.

## read

Reads a file.

```json
{ "path": "cmd/mani/main.go" }
```

## glob

Lists paths matching a pattern, relative to the workspace root.

```json
{ "pattern": "**/*_test.go" }
```

## grep

Searches file contents with RE2 regular expressions: no backreferences, no lookahead. `path`
narrows the search to a subdirectory, `include` to a set of files.

```json
{ "pattern": "func Test[A-Z]", "path": "app", "include": "**/*.go" }
```

## write

Writes a file, creating it or replacing its contents.

```json
{ "path": "notes/summary.md", "content": "..." }
```

## edit

Replaces an exact string with another one. The edit fails if `old_content` is missing from the
file, and also if it appears more than once — an ambiguous edit is refused rather than applied to
the first match:

```
edit: old content is ambiguous; found 3 occurrences
```

```json
{ "path": "main.go", "old_content": "log.Println(err)", "new_content": "slog.Error(\"failed\", \"err\", err)" }
```

## delete

Deletes a file inside the workspace.

```json
{ "path": "tmp/scratch.txt" }
```

## fetch

An HTTP or HTTPS GET, with HTML reduced to text. Guards, all of them fixed:

- `policy.network` allow and deny lists decide which hosts are reachable at all.
- Private, loopback and link-local addresses are refused at dial time, so an allowed hostname
  that resolves to `127.0.0.1` cannot reach the agent's own server.
- At most 5 redirects, and each hop is checked again.
- Bodies are read up to 5 MB, and the text handed to the model is truncated at 40 000 characters
  with a line saying how much was dropped.

```json
{ "url": "https://api.github.com/repos/Federicoand98/mani" }
```

```yaml
policy:
  network:
    allow: ["api.github.com", "*.wikipedia.org"]
    deny:  ["*.internal"]
```

## bash

Runs a command with the shell, with the workspace as the working directory. The tool description
names the shell it detected, so the model knows what syntax it may use.

```json
{ "command": "go test ./... 2>&1 | tail -20" }
```

`bash` is `execute`: it is the one tool that can do anything the user running mani can do.
`policy.tools.bash: deny` plus the tools you actually need is the usual shape for an unattended
agent, with pattern rules for the cases where you need it but not all of it:

```yaml
policy:
  tools: { bash: allow }
  rules:
    - { tool: bash, pattern: 'rm\s+-rf', action: deny, label: "recursive delete" }
    - { tool: bash, pattern: 'curl|wget', action: deny, label: "network via shell" }
```

`limits.tool_timeout` caps how long a command may run.

## planning

Gives the agent a checklist it writes and updates itself. Useful on long tasks, where the plan
appearing in the journal is also how you later see what the agent thought it was doing.

## delegate

Hands a sub-task to a subagent, which starts with empty memory and returns only its final answer.

With no `capabilities.subagents` declared, the tool takes `task` and an optional `instructions`
and spawns a child with the parent's tools. With subagents declared, it takes `task` and `agent`,
and only the named ones are reachable:

```yaml
capabilities:
  tools: [read, grep, delegate]
  subagents:
    - name: researcher
      description: "read-only code exploration, reports file:line"
      prompt: "Explore and report. Never edit."
      tools: [read, grep]
```

```json
{ "agent": "researcher", "task": "Where is the permission gate invoked?" }
```

A subagent's own tool list and policy apply, so a read-only researcher stays read-only even when
the parent can write. `limits.subagent_depth` (default 5) caps nesting.

## Adding your own

Two ways, both in the manifest: an external process (`command` plus a `schema`) or an MCP server.
See the [custom tools guide](../../guides/custom-tools/).

Next: the [HTTP and WebSocket API](../http-api/).
