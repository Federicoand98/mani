# mani

**A Declarative Agent Runtime in Go.** Define governable AI agents as configuration, run them
headless, as a service, or on triggers — with permissions, guardrails, budgets and an audit trail
built in.

> Learning project (`github.com/Federicoand98/mani`): every piece exists for a reason. Minimal,
> hexagonal, `core/` has zero external dependencies — readable end to end.

## Why mani

The thesis is **agents as configuration, not code**. Where it aims to differ from LangChain / LangGraph:

- **Declarative / manifest-first** — an agent (provider, tools, permissions, guardrails, budget,
  triggers, MCP servers, subagents, output schema) is one YAML file, not glue code.
- **Governance-first** — permissions, risk levels, guardrails and per-run budgets live in the
  runtime, so a manifest-defined agent can run **unattended** safely by construction — and the
  **run journal** records what it actually did.
- **Minimal & legible** — hexagonal architecture, no framework sprawl, a single Go binary.

The classic coding-agent tools (`read`, `edit`, `write`, `bash`) are **batteries included and the
canonical example, not the identity** — a coding agent is just one manifest.

---

# Part 1 — For everyone

*No Go, no programming. You write a YAML file describing what the agent may do, and run one command.*

### What is an "agent" here?

A model (Claude, GPT, a local Llama…) that can **use tools** — read a file, run a command, call
your own script — in a loop, until the task is done. mani is the thing that runs that loop *under
rules you write*.

### The three questions a manifest answers

```yaml
provider: anthropic                       # 1. WHO thinks?
model: claude-sonnet-5

system_prompt: "You are a nightly maintenance agent."

tools:                                    # 2. WHAT can it do?
  - read
  - bash

permissions:                              # 3. WHAT is it NOT allowed to do?
  bash: ask                               # allow | ask | deny
  read: allow
```

Save it as `agent.yaml` and run:

```bash
mani run --config agent.yaml --task "Summarize the errors in today's log"
```

### Safety nets you get for free

You don't have to trust the model. You constrain it:

```yaml
guardrails:
  deny:                                   # never, no matter what the model wants
    - { tool: bash, pattern: 'rm\s+-rf', label: "recursive delete" }
    - { tool: bash, pattern: 'curl.*\|\s*(sh|bash)', label: "pipe-to-shell" }
  mask:                                   # scrub secrets out of tool output
    - { pattern: 'sk-[A-Za-z0-9]{20,}', with: "***REDACTED***" }

budget:                                   # a runaway agent can't drain your account
  max_tokens: 50000
  max_tool_calls: 20
  max_duration: 2m
```

### Let it run by itself, at night

```yaml
triggers:
  - type: daily
    at: "02:00"
    prompt: "Check the logs and summarize anomalies."
```

```bash
mani run --config agent.yaml        # no --task → starts the trigger scheduler, stays running
```

### Know what it did while you slept

Every run is recorded — which tools ran, tokens spent, what got blocked or masked:

```yaml
observability:
  journal:
    enabled: true
    path: ./runs         # one file per run, plain JSON lines
    retention: 200
```

```bash
cat runs/*.jsonl         # human-readable audit trail
```

### Teach it a new skill without writing Go

Any executable that reads JSON on stdin and writes text on stdout becomes a tool — Python, Node,
a shell script, anything:

```yaml
tools:
  - name: fetch_stock
    description: "fetch stock data"
    command: ./tools/.venv/bin/python
    args: ["tools/stock.py"]
    schema:
      type: object
      properties:
        symbol: { type: string, description: "stock symbol to fetch" }
      required: ["symbol"]
```

### Get structured data back instead of prose

Declare the shape of the answer and the agent must return exactly that:

```yaml
output_schema:
  type: object
  properties:
    sentiment: { type: string, enum: [positive, negative, neutral] }
    score:     { type: number }
  required: [sentiment, score]
```

```bash
mani run --config sentiment.yaml --task "The delivery was late but support was great"
# {"sentiment": "neutral", "score": 0.5}
```

That makes the agent a **typed function** you can pipe into other programs.

---

# Part 2 — Technical

## Status

| Area | State |
|---|---|
| Interactive TUI coding agent | ✅ |
| Providers: Ollama, OpenAI, Anthropic, GitHub Copilot, OpenRouter | ✅ |
| Tools: `read`, `edit`, `write`, `bash`, MCP client, subprocess tools | ✅ |
| Permission manager (allow / ask / deny), risk levels | ✅ |
| Sessions, planning, subagents, hooks, tracing, compaction | ✅ |
| Declarative manifest + headless `run` | ✅ |
| Triggers (every / daily / webhook) as a long-lived daemon | ✅ |
| Agent server (REST + WebSocket, bearer auth) | ✅ |
| Structured output (typed response schema) | ✅ |
| Guardrails (deny / mask) + per-run budget | ✅ |
| Run journal / audit trail (`GET /runs`) | ✅ |
| Python SDK | 🗺️ roadmap |
| Deploy & ship tooling (release binaries, containers, service templates) | 🗺️ roadmap |

## Install & quick start

Needs Go 1.26+. For the default provider, a local [Ollama](https://ollama.com).

```bash
git clone https://github.com/Federicoand98/mani
cd mani
go build ./...
go run ./cmd/mani          # interactive TUI
```

Defaults: provider `ollama` at `http://localhost:11434`, model `qwen3.5:9b`. Config lives in
`~/.config/mani/config.json`, overridable by env:

```
MANI_PROVIDER   MANI_MODEL   MANI_UI   MANI_THINKING   MANI_DEBUG
MANI_CONTEXT_WINDOW   MANI_LOG_LEVEL   MANI_MAX_ITERATIONS
```

Cloud providers need credentials — in the TUI: `/login anthropic`, then `/provider anthropic`.

## Usage

### 1. Interactive TUI

```bash
go run ./cmd/mani
tail -f ~/.config/mani/mani.log     # TUI keeps stdout clean; logs go to a file
```

Slash commands: `/help` `/model` `/provider` `/login <p>` `/logout <p>` `/session` `/memory`
`/clear` `/config` `/thinking` `/quit`

### 2. Headless single task

```bash
mani run --config agent.yaml --task "Summarize the errors in today's log"
mani run --config agent.yaml --task "..." --verbose      # logs to stderr (default: silent)
```

Prints `LastResponse()`, or pretty-printed JSON when the manifest declares an `output_schema`.
Permission requests are **fail-closed** (auto-denied) in headless mode — design manifests for
unattended use with `allow`/`deny`, not `ask`.

### 3. Trigger daemon (long-lived)

Omit `--task` and the manifest's triggers drive the runtime. The scheduler is **in-process**, so
the same binary and the same manifest work on Linux, macOS and Windows — no systemd/cron needed.

```yaml
triggers:
  - { type: every,   every: 30m, prompt: "Poll the queue and process pending items." }
  - { type: daily,   at: "02:00", prompt: "Check the logs and summarize anomalies." }
  - { type: webhook, addr: ":8787", prompt: "Handle this event: {{body}}" }
```

```bash
mani run --config agent.yaml
```

### 4. Agent server (REST + WebSocket)

```bash
export MANI_SERVER_TOKEN=secret
mani serve --config agent.yaml --addr :9000
mani serve --config agent.yaml --insecure        # dev only, no auth
```

All routes sit behind bearer auth:

| Method | Route | Purpose |
|---|---|---|
| `POST` | `/sessions` | create a session (its own Runtime) |
| `GET` | `/sessions` | list sessions |
| `DELETE` | `/sessions/{id}` | drop a session |
| `POST` | `/sessions/{id}/chat` | one turn on a session |
| `POST` | `/chat` | stateless one-shot turn |
| `GET` | `/sessions/{id}/turn` | WebSocket: streaming multi-turn + permission back-channel |
| `GET` | `/runs` | list runs (`?session=`, `?limit=`) |
| `GET` | `/runs/{id}` | full run record with events |

```bash
curl -H "Authorization: Bearer $MANI_SERVER_TOKEN" \
     -H 'Content-Type: application/json' \
     -d '{"input":"summarize README.md"}' \
     localhost:9000/chat

curl -H "Authorization: Bearer $MANI_SERVER_TOKEN" localhost:9000/runs?limit=5
```

The WebSocket carries `token` / `thinking` / `tool_call` / `tool_result` / `usage` / `done`
frames, plus `permission_request` — the client answers with a `request_id` and a decision, so
approvals work over the wire. See [docs/agent-server.md](docs/agent-server.md).

### 5. Subprocess tools

A tool is any executable: mani writes the JSON input on **stdin**, reads the result from
**stdout** (stderr on a non-zero exit becomes the error the model sees). Declare `risk`
(`none` / `write` / `execute`, default `execute`) so the permission layer can gate it.

```yaml
tools:
  - name: fetch_stock
    description: "fetch stock data"
    command: ./tools/.venv/bin/python
    args: ["tools/stock.py"]
    risk: none
    schema:
      type: object
      properties:
        symbol: { type: string }
      required: ["symbol"]
```

### 6. Library usage

`mani` is importable — skip the CLI and wire a `Runtime` yourself:

```go
cfg, _ := config.Load()
ws, _ := os.Getwd()

rt := app.NewFromConfig(cfg).
    WithTool(fs.NewReadFileTool(ws)).
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

Or build straight from a manifest:

```go
spec, _ := app.LoadManifest("agent.yaml")
rt, _ := app.Build(ctx, spec)
defer rt.Close()
```

`Runtime.Execute` returns a `<-chan Event`, decoupling execution from rendering (TUI today,
HTTP/WebSocket tomorrow — same channel).

## Manifest reference

```yaml
provider: anthropic              # ollama | openai | anthropic | copilot | openrouter
model: claude-sonnet-5
system_prompt: "..."
workspace: .                     # default: cwd; every fs tool is confined to it

tools: [read, edit, write, bash] # or subprocess objects (see above)

permissions:                     # allow | ask | deny; `default` sets the fallback
  bash: deny
  default: allow

features:                        # all on by default
  planning: true
  context_injection: true
  tracing: true
  compaction: { enabled: true, keep: 20 }
  subagents:  { enabled: true, depth: 5 }

subagents:                       # named delegates with their own prompt/tools/model
  - name: researcher
    description: "read-only code exploration, reports file:line"
    system_prompt: "..."
    tools: [read]
    model: ""                    # "" = inherit

mcpservers:
  - { name: deepwiki, url: https://mcp.deepwiki.com/sse }

output_schema: { type: object, properties: {...}, required: [...] }

guardrails:
  deny: [{ tool: bash, pattern: 'rm\s+-rf', label: "..." }]
  mask: [{ pattern: 'sk-[A-Za-z0-9]{20,}', with: "***REDACTED***" }]

budget:
  max_tokens: 50000
  max_tool_calls: 20
  max_duration: 2m
  per_tool_timeout: 15s

observability:
  journal: { enabled: true, path: ./runs, retention: 200 }

triggers: [...]
context_window: 0                # 0 = inherit
max_iterations: 15
```

Runnable examples in [`_examples/`](_examples/): `manifest-minimal.yaml`, `manifest.yaml`,
`guarded.yaml` (guardrails + budget), `sentiment.yaml` (structured output),
`observability.yaml` (journal + trigger).

## Architecture

Hexagonal (Ports & Adapters). The single invariant: **`core/` has zero external dependencies.**
Dependency arrows always point inward.

```
cmd/mani/      composition root — TUI, run, serve
app/           application service — Runtime, events, manifest, permissions,
               guardrails, budget, journal, subagents, triggers
server/        driving adapter — REST + WebSocket
tui/           driving adapter — terminal UI (BubbleTea)
core/          domain — Agent, Memory, LLMClient port, hooks, types
llm/*/         driven adapters — ollama, openai, anthropic, copilot, openrouter
tool/          Tool interface + registry
tool/fs/       read/edit/write     tool/bash/    bash
tool/mcp/      MCP client          tool/subprocess/  external-process tools
config/        config + credentials on disk       session/  session storage
```

Interfaces are defined in the **consuming** package (Go idiom): `LLMClient`, `Memory`,
`PreToolUseHook` live in `core/`; `Tool` lives in `tool/`.

Governance and observability are **pure composition over hooks** — guardrails, budget and the
journal add zero lines to `core/`. The journal writes append-only JSONL (one file per run) behind
a `Journal` port, so SQLite/Redis are drop-in adapters later.

```bash
go build ./...
go test ./...
```

## Roadmap

1. **Manifest composition** — reference a manifest as a tool, composing independently governed units.
2. **Deploy & ship** — release binaries, container images and cross-platform service templates so
   an agent can be shipped to a VM and run unattended.
3. **Python SDK** — drive the runtime over the agent server.

Feature filter: does it deepen manifest expressiveness, safe autonomy, or operability as a service?
If not, it's out of scope.

## License

See [LICENSE](LICENSE).
