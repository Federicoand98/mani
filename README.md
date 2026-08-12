# mani

**A Declarative Agent Runtime in Go.** Define governable AI agents as configuration, run them
headless, as a service, or on triggers — with policy, limits and an audit trail built in.

> Learning project (`github.com/Federicoand98/mani`): every piece exists for a reason. Minimal,
> hexagonal, `core/` has zero external dependencies — readable end to end.

## Why mani

The thesis is **agents as configuration, not code**. Where it aims to differ from LangChain / LangGraph:

- **Declarative / manifest-first** — an agent (model, tools, policy, limits, triggers, MCP
  servers, subagents, output schema) is one YAML file, not glue code: eight blocks, one
  question each.
- **Governance-first** — per-tool policy, risk levels, pattern rules and per-run limits live in
  the runtime, so a manifest-defined agent can run **unattended** safely by construction — and the
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

### One block, one question

A manifest has **8 top-level blocks**, and each answers exactly **one** question about the
agent. That's the whole mental model:

```yaml
identity:                                 # WHO thinks?
  provider: anthropic
  model: claude-sonnet-5
  prompt: "You are a nightly maintenance agent."

capabilities:                             # WHAT can it do?
  tools: [read, bash]

policy:                                   # WHAT is it allowed to do?
  tools:
    bash: ask                             # allow | ask | deny
    read: allow
```

Save it as `agent.yaml` and run:

```bash
mani run --config agent.yaml --task "Summarize the errors in today's log"
```

The other five blocks — `context`, `output`, `limits`, `run`, `observability` — are shown
below. Adding something new? Walk the list top to bottom and the first match wins, so you
always know where a setting belongs (full table in [CONTEXT.md](CONTEXT.md)).

### Safety nets you get for free

You don't have to trust the model. You constrain it — `policy` works at three levels of
granularity, from the coarsest to the finest:

```yaml
policy:
  tools:                                  # 1. THE TOOL: may it be used at all?
    bash: allow
  rules:                                  # 2. THE CALL: inspect the input, block this one
    - { tool: bash, pattern: 'rm\s+-rf', action: deny, label: "recursive delete" }
    - { tool: bash, pattern: 'curl.*\|\s*(sh|bash)', action: deny, label: "pipe-to-shell" }
  redact:                                 # 3. THE OUTPUT: scrub secrets before the model sees them
    - { pattern: 'sk-[A-Za-z0-9]{20,}', with: "***REDACTED***" }

limits:                                   # a runaway agent can't drain your account
  max_tokens: 50000
  max_tool_calls: 20
  max_duration: 2m
```

### Let it run by itself, at night

```yaml
run:
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
capabilities:
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
output:
  schema:
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
| Policy: per-tool allow/ask/deny, pattern rules, output redaction + per-run limits | ✅ |
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

Prints the final text, or pretty-printed JSON when the manifest declares an `output.schema`.
Permission requests are **fail-closed** (auto-denied) in headless mode — design manifests for
unattended use with `allow`/`deny`, not `ask`.

### 3. Trigger daemon (long-lived)

Omit `--task` and the manifest's triggers drive the runtime. The scheduler is **in-process**, so
the same binary and the same manifest work on Linux, macOS and Windows — no systemd/cron needed.

```yaml
run:
  triggers:
    - { type: every,   every: 30m, name: disk-watch, prompt: "Report partitions above 85%." }
    - { type: daily,   at: "02:00", name: nightly, catch_up: true, prompt: "Summarize anomalies." }
    - { type: webhook, addr: ":8787", prompt: "Handle this event: {{body}}" }
  scheduler:
    path: ./queue          # durable: tasks survive restarts and crashes
    concurrency: 2         # runs executed in parallel
    retry: { max_attempts: 3, backoff: 30s }
```

```bash
mani run --config agent.yaml
```

Each trigger firing becomes one **task**. With `scheduler.path` set, the queue is a directory
(`pending/ running/ done/ failed/`) you can inspect with `ls`: tasks survive a crash, failed
ones are retried with backoff, and a `daily` trigger with `catch_up: true` recovers a firing
missed while the process was down. Its workers start on their own — there is nothing to declare
to consume the queue.

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

Eight blocks, one question each. Unknown keys are a **hard error**, never a silent no-op.

| Block | Question it answers |
|---|---|
| `identity` | who thinks? |
| `capabilities` | what can it do? |
| `context` | what does it see and remember? |
| `output` | what does it return? |
| `policy` | what is it allowed to do? |
| `limits` | how much may it consume? |
| `run` | when does it start, and how is it executed? |
| `observability` | what does it leave behind? |

```yaml
identity:
  name: nightly-maintainer       # identifies the agent
  description: "..."
  provider: anthropic            # ollama | openai | anthropic | copilot | openrouter
  model: claude-sonnet-5
  prompt: "..."                  # the system prompt

capabilities:
  workspace: .                   # default: cwd; every fs tool is confined to it
  tools: [read, edit, write, bash, planning, delegate]   # or subprocess objects (see above)
  mcp:
    - { name: deepwiki, url: https://mcp.deepwiki.com/sse }
  subagents:                     # named delegates; reachable via the `delegate` tool
    - name: researcher
      description: "read-only code exploration, reports file:line"
      prompt: "..."
      tools: [read]
      model: ""                  # "" = inherit

context:
  window: 0                      # 0 = inherit from config
  inject: true                   # pull AGENTS.md into the system prompt
  compaction: { enabled: true, keep: 20 }

output:
  schema: { type: object, properties: {...}, required: [...] }

policy:
  tools:                         # allow | ask | deny; `default` sets the fallback
    bash: deny
    default: allow
  rules: [{ tool: bash, pattern: 'rm\s+-rf', action: deny, label: "..." }]
  redact: [{ pattern: 'sk-[A-Za-z0-9]{20,}', with: "***REDACTED***" }]

limits:
  max_tokens: 50000
  max_tool_calls: 20
  max_duration: 2m
  max_iterations: 15
  tool_timeout: 15s
  subagent_depth: 5

run:
  triggers: [...]
  scheduler:                     # how triggers are executed — NOT a list of tasks
    path: ./queue                # present ⇒ the queue survives restarts
    concurrency: 1
    max_pending: 64
    retry: { max_attempts: 3, backoff: 30s }

observability:
  tracing: true
  journal: { enabled: true, path: ./runs, retention: 200 }
```

**Built-in tools:** `read` · `edit` · `write` · `bash` · `planning` · `delegate`. The last two
are ordinary tools, not flags — declare them to enable them. A tool's manifest key is always
its runtime name, so `policy` and subagent references resolve against the same vocabulary.

Runnable examples in [`_examples/`](_examples/): `manifest-minimal.yaml`, `manifest.yaml`
(subagents + MCP), `guarded.yaml` (all three policy levels + limits), `sentiment.yaml`
(structured output), `observability.yaml` (journal + trigger), `queue.yaml` (durable scheduler).

## Architecture

Hexagonal (Ports & Adapters). The single invariant: **`core/` has zero external dependencies.**
Dependency arrows always point inward.

```
cmd/mani/      composition root — TUI, run, serve
app/           application service — Runtime, events, manifest, policy, limits,
               journal, task queue, subagents, triggers
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

Governance and observability are **pure composition over hooks** — policy rules, limits and the
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

## Status & stability

**This is a learning project.** It works, it's tested, and it's honest about what it isn't:
the public API is **unstable until 1.0** — packages, manifest keys and tool names may change
between minor versions (the manifest grammar was reorganised wholesale in 0.1.0). Pin a version
if you depend on it.

Credentials never live in a manifest: they're stored separately in `auth.json`
(`$XDG_DATA_HOME/mani/auth.json`, mode 0600) and managed with `/login`. Keep manifests
committable — they are meant to be.

## License

[Apache License 2.0](LICENSE).
