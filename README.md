# mani

**A Declarative Agent Runtime in Go.** Define governable AI agents as configuration, run them
headless, as a service, or on triggers — with policy, limits and an audit trail built in.

> Learning project (`github.com/Federicoand98/mani`): every piece exists for a reason. Minimal,
> hexagonal, `core/` has zero external dependencies — readable end to end.

## Why mani

The thesis is **agents as configuration, not code**.

- **Declarative / manifest-first** — an agent (model, tools, policy, limits, triggers, MCP
  servers, subagents, output schema) is one YAML file, not glue code: eight blocks, one question
  each.
- **Governance-first** — per-tool policy, risk levels, pattern rules and per-run limits live in
  the runtime, so a manifest-defined agent can run **unattended** safely by construction — and
  the **run journal** records what it actually did.
- **Minimal & legible** — hexagonal architecture, no framework sprawl, a single Go binary.

The classic coding-agent tools (`read`, `edit`, `write`, `bash`) are **batteries included and the
canonical example, not the identity** — a coding agent is just one manifest.

## See it

The manifest declares the shape of the answer, the runtime validates it, and the run drops into
a unix pipeline like any other command:

![An agent returning typed JSON, piped into jq](_examples/demo/triage.gif)

Two more in [`_examples/demo/`](_examples/demo/), with the tapes they were recorded from:

- [`unattended.gif`](_examples/demo/unattended.gif) — no `--task`: the agent starts itself on a
  trigger, gets `SIGKILL`ed mid-work, and **resumes the same task** on restart. The journal shows
  the policy blocking `rm -rf` on every pass, while nobody was watching.
- [`polyglot.gif`](_examples/demo/polyglot.gif) — eight lines of Python become a governed tool:
  JSON in on stdin, JSON out on stdout, no plugin SDK and no Go.

## Install

Needs Go 1.25+ (see `go.mod`). For the default provider, a local [Ollama](https://ollama.com).

```bash
go install github.com/Federicoand98/mani/cmd/mani@latest

mani init                             # scaffold a commented agent.yaml
mani validate --config agent.yaml     # check it without running anything
mani run --config agent.yaml --task "hello"
mani                                  # or just start the interactive chat
```

Config lives in `~/.config/mani/config.json`; credentials never touch a manifest — they go in
`$XDG_DATA_HOME/mani/auth.json` (mode 0600), managed with `/login` in the TUI.

## One block, one question

A manifest has eight top-level blocks, and each answers exactly one question. That is the whole
mental model — and it tells you where anything new belongs.

| Block | Question | |
|---|---|---|
| `identity` | who thinks? | provider, model, prompt |
| `capabilities` | what can it do? | tools, MCP, subagents, workspace |
| `context` | what does it see and remember? | window, compaction, injection |
| `output` | what does it return? | response schema |
| `policy` | what is it allowed to do? | per-tool permissions, rules, redaction, network |
| `limits` | how much may it consume? | tokens, calls, duration, timeouts |
| `run` | when does it start, and how? | triggers, scheduler |
| `observability` | what does it leave behind? | tracing, journal |

```yaml
identity:
  provider: anthropic
  model: claude-sonnet-5
  prompt: !include ./prompts/maintainer.md

capabilities:
  tools: [read, grep, bash]

policy:
  tools:
    bash: allow
  rules:
    - { tool: bash, pattern: 'rm\s+-rf', action: deny, label: "recursive delete" }

run:
  triggers:
    - { type: daily, at: "02:00", prompt: "Summarize anomalies in today's logs." }
  scheduler:
    path: ./queue          # the queue survives restarts

observability:
  journal: { enabled: true, path: ./runs }
```

Unknown keys are a **hard error**, never a silent no-op.

## Documentation

| | |
|---|---|
| [Introduction](docs/introduction.md) | for everyone — no Go, no programming |
| [Manifest reference](docs/manifest.md) | every block, every key, the built-in tools |
| [Usage](docs/usage.md) | CLI, trigger daemon, agent server, subprocess tools, library |
| [Agent server](docs/agent-server.md) | the REST + WebSocket protocol in full |
| [Agentic loop](docs/agentic-loop.md) | where hooks fire, where permissions gate |
| [`_examples/`](_examples/) | runnable manifests |

## Status

| Area | State |
|---|---|
| Declarative manifest (8 blocks) + headless `run` | ✅ |
| CLI: `init`, `validate`, `run`, `runs`, `serve`, `tui`, `--version` | ✅ |
| Providers: Ollama, OpenAI, Anthropic, GitHub Copilot, OpenRouter | ✅ |
| Tools: `read` `write` `edit` `delete` `glob` `grep` `bash` `fetch` `planning` `delegate` | ✅ |
| MCP client, subprocess tools in any language | ✅ |
| Policy: allow/ask/deny, risk levels incl. `network`, rules, redaction, per-run limits | ✅ |
| Triggers (every / daily / webhook) + durable queue that survives crashes | ✅ |
| Structured output (typed response schema) | ✅ |
| Run journal / audit trail (`mani runs`, `GET /runs`) | ✅ |
| Agent server (REST + WebSocket, bearer auth) | ✅ |
| Sessions, planning, subagents, hooks, tracing, compaction, image input | ✅ |
| MCP **server** mode (expose an agent as a tool) | 🚧 next |
| Python SDK · container images | 🗺️ roadmap |

## Architecture

Hexagonal (Ports & Adapters). The single invariant: **`core/` has zero external dependencies.**
Dependency arrows always point inward.

```
cmd/mani/      composition root — TUI, run, serve, init, validate
app/           application service — Runtime, events, manifest, policy, limits,
               journal, task queue, subagents, triggers
server/        driving adapter — REST + WebSocket
tui/           driving adapter — terminal UI (BubbleTea)
core/          domain — Agent, Memory, LLMClient port, hooks, types
llm/*/         driven adapters — ollama, openai, anthropic, copilot, openrouter
tool/          Tool interface + registry
tool/fs/       read/write/edit/delete/glob/grep      tool/bash/   shell
tool/fetch/    HTTP GET with SSRF guard              tool/mcp/    MCP client
tool/subprocess/  external-process tools
config/        config + credentials on disk          session/     session storage
```

Interfaces are defined in the **consuming** package (Go idiom): `LLMClient`, `Memory`,
`PreToolUseHook` live in `core/`; `Tool` lives in `tool/`.

Governance and observability are **pure composition over hooks** — policy rules, limits and the
journal add zero lines to `core/`. The journal writes append-only JSONL (one file per run) behind
a `Journal` port, so SQLite/Redis are drop-in adapters later.

```bash
go build ./... && go test ./...
```

## Roadmap

1. **MCP server mode** — expose a manifest-defined agent as a tool to any MCP client, so a
   governed agent can live inside an IDE and still leave an audit trail.
2. **Manifest composition** — reference a manifest as a tool, composing independently governed units.
3. **Python SDK** — drive the runtime over the agent server.

Feature filter: does it deepen manifest expressiveness, safe autonomy, or operability as a
service? If not, it's out of scope.

## Stability

**This is a learning project.** It works, it's tested, and it's honest about what it isn't: the
public API is **unstable until 1.0** — packages, manifest keys and tool names may change between
minor versions. Pin a version if you depend on it.

## License

[Apache License 2.0](LICENSE).
