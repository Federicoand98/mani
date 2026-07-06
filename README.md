# mani

**A Declarative Agent Runtime in Go.** Define governable AI agents as configuration, run them
headless, as a service, or on triggers — with permissioning and observability built into the core.

> Learning project (`github.com/Federicoand98/mani`): every piece exists for a reason. Minimal,
> hexagonal, `core/` has zero external dependencies — readable end to end.

## Why mani

The thesis is **agents as configuration, not code**. Where it aims to differ from LangChain / LangGraph:

- **Declarative / manifest-first** — an agent (provider, tools, permissions, hooks, triggers, MCP
  servers, subagents) is described in a YAML manifest, not wired together in glue code.
- **Governance-first** — permissions, policy and per-tool risk levels live in the core, so a
  manifest-defined agent can run **unattended** (on a cron/webhook trigger) safely by construction.
- **Minimal & legible** — hexagonal architecture, no framework sprawl, deployable as a single Go binary.

The classic coding-agent tools (`read_file`, `edit_file`, `bash`, …) are **batteries included and the
canonical example, not the identity** — a coding agent is just one manifest.

## Status

| Area | State |
|---|---|
| Interactive TUI coding agent | ✅ works |
| Multi-provider (Ollama, OpenAI, GitHub Copilot, OpenRouter) | ✅ works |
| Tools: read/edit/write/delete file, bash, MCP client | ✅ works |
| Permission manager (allow / ask / always), risk levels | ✅ works |
| Sessions + storage, planning, subagents, hooks, tracing | ✅ works |
| Triggers (cron / daily / webhook daemon) | ✅ works |
| Declarative manifest (`agent.yaml`) + headless `run` | 🚧 in progress |
| Agent server (HTTP/WebSocket) + Python SDK | 🚧 in progress |
| Subprocess tools, structured output | 🗺️ roadmap |

## Quick start

Needs Go 1.26+ and, for the default provider, a local [Ollama](https://ollama.com) running.

```bash
git clone https://github.com/Federicoand98/mani
cd mani
go build ./...
go run ./cmd/mani          # start the interactive agent (TUI)
```

Defaults: provider `ollama` at `http://localhost:11434`, model `qwen3.5:9b`. Config lives in
`~/.config/mani/config.json` and can be overridden by env vars:

```bash
MANI_PROVIDER=ollama MANI_MODEL=qwen3.5:9b MANI_LOG_LEVEL=debug go run ./cmd/mani
```

Logs go to `~/.config/mani/mani.log` (the TUI keeps stdout clean):

```bash
tail -f ~/.config/mani/mani.log
```

### Slash commands (in the TUI)

`/help` · `/model` · `/provider` · `/login <provider>` · `/logout <provider>` · `/session` ·
`/memory` · `/clear` · `/config` · `/thinking` · `/quit`

Cloud providers need credentials: `/login openai` (then set the key), then `/provider openai`.

## Library usage

`mani` is importable — skip the CLI and wire a `Runtime` yourself:

```go
cfg, _ := config.Load()
ws, _ := os.Getwd()

rt := app.NewFromConfig(cfg).
    WithTool(fstools.NewReadFileTool(ws)).
    WithTool(bash.NewBashTool(ws)).
    UsePermissionManager()

for ev := range rt.Execute(ctx, "list the Go files and summarize the packages") {
    switch ev.Type {
    case app.EventToken:
        fmt.Print(ev.Payload.(app.TokenPayload).Text)
    case app.EventDone:
        // turn finished
    }
}
```

`Runtime.Execute` returns a `<-chan Event`, decoupling execution from rendering (TUI today,
HTTP/WebSocket tomorrow — same channel).

## Architecture

Hexagonal (Ports & Adapters). The single invariant: **`core/` has zero external dependencies.**
Dependency arrows always point inward.

```
cmd/mani/      composition root — wires everything
app/           application service — Runtime, event channel, permissions, hooks, triggers, build
tui/           driving adapter — terminal UI (BubbleTea)
core/          domain — Agent, Memory, LLMClient port, hooks, types
llm/ollama/    driven adapter — LLM clients (ollama, openai, copilot, openrouter)
tool/          Tool interface + registry
tool/fs/       read/edit/write/delete file      tool/bash/   bash      tool/mcp/   MCP client
config/        config + credentials on disk      session/     session storage
```

Interfaces are defined in the **consuming** package (Go idiom): `LLMClient`, `Memory`,
`PreToolUseHook` live in `core/`; `Tool` lives in `tool/`.

## Roadmap

Toward the declarative thesis, in order:

1. **Subprocess tools** — declare a tool as an external process (JSON over stdin/stdout), so you
   add capabilities in the manifest, in any language, without writing Go. This is what makes the
   runtime genuinely no-code.
2. **Structured output** — a typed response schema in the manifest; the agent becomes a pluggable,
   typed component, not just a chat.
3. **Python SDK** — drive the declarative runtime from outside over the agent server.

Feature filter: does it deepen manifest expressiveness, safe autonomy, or operability as a service?
If not, it's out of scope.

## License

See [LICENSE](LICENSE).
