# Usage

Every surface runs the *same* manifest. The transport is a command, not a rewrite.

## The CLI

```
mani                      interactive terminal chat (the default)
mani init                 scaffold a commented agent.yaml
mani validate --config    check a manifest without running anything
mani run --config         one task, the trigger daemon, or a flow
mani batch --config       one agent over a JSONL file of tasks
mani serve --config       expose the agent over HTTP/WebSocket
mani mcp   --config       expose the agent to MCP clients over stdio
mani runs   --config      list past runs, or replay one as a timeline
mani tui                  the interactive chat, named explicitly
mani --help  --version
```

Exit codes are meaningful for scripts: `0` success, `1` a run failed, `2` you invoked it wrong
(bad flag, missing or invalid manifest).

## 1. Interactive TUI

```bash
mani
tail -f ~/.config/mani/mani.log     # the TUI keeps stdout clean; logs go to a file
```

Slash commands: `/help` `/model` `/provider` `/login <p>` `/logout <p>` `/session` `/memory`
`/clear` `/config` `/thinking` `/quit`

## 2. Headless single task

```bash
mani run --config agent.yaml --task "Summarize the errors in today's log"
mani run --config agent.yaml --task "..." --verbose      # logs to stderr (default: silent)
mani run --config agent.yaml --task "what is wrong here?" --image screenshot.png
```

Prints the final text, or pretty-printed JSON when the manifest declares an `output.schema`.
Permission requests are **fail-closed** (auto-denied) in headless mode — design manifests for
unattended use with `allow`/`deny`, not `ask`.

`--provenance` wraps the result with the run that produced it, instead of returning it bare:

```bash
mani run --config classify.yaml --task "the parcel never arrived" --provenance
```

```json
{
  "result": {"sentiment": "negative"},
  "run": {"id": "8f2a1c4b7e90", "source": "cli", "provider": "ollama",
          "model": "qwen3.5:9b", "manifest": "classify.yaml",
          "in_tokens": 412, "out_tokens": 23}
}
```

The bare result stays the default, because `output.schema` declares what a run returns: wrapping
it by default would make every manifest describe a sub-object, and break every `| jq '.sentiment'`
ever written. The journal holds the provenance either way — the flag is for when the result
travels somewhere else and has to carry it along. `run.id` is what `mani runs <id>` takes.

If the manifest names a provider that cannot be used — missing credentials, no base URL — the
run **fails**. It never falls back to another model behind your back: cost and privacy are the
opposite of what you declared, and finding out afterwards is worse than not running.

## 3. Many tasks at once: `mani batch`

One agent over a JSONL file: one line in, one record out.

```bash
mani batch --config classify.yaml --in reviews.jsonl --out out/ --jobs 4
mani batch --config classify.yaml --in - < reviews.jsonl | jq -c '.result'
```

```json
{"id": "r01", "task": "Boils fast and looks great on the counter.", "product": "kettle"}
{"id": "r02", "task": "The lid broke after two weeks.", "product": "kettle"}
```

`id` is required, must be unique and usable as a file name — it becomes one under `--out`. Any
other field travels through to the result untouched, so a shelfmark or a ticket number comes back
next to the answer. A `task` that is not a string is sent to the model as JSON.

```json
{"id": "r02", "product": "kettle", "result": {"sentiment": "negative"},
 "run": {"id": "8f2a1c4b7e90", "source": "batch", "model": "qwen3.5:9b", "in_tokens": 412}}
```

| Flag | Meaning |
|---|---|
| `--in FILE` | the tasks; `-` reads stdin |
| `--out DIR` | one `<id>.json` per task, which is what makes it resumable; without it, records stream to stdout as they finish |
| `--jobs N` | tasks at a time (default 1) |
| `--limit N` | stop after this many new tasks |

What it does when things go wrong, which is the whole reason to use it on a long list:

- the **entire input is checked before the first model call** — a malformed line, a missing or
  repeated `id`, an `id` that could escape the directory — so a bad file fails now, not two
  thousand runs later;
- a failed task writes **no result file** and appends the reason to `<out>/errors.jsonl`; the exit
  code is 1;
- rerunning **skips** what is already in `--out` and retries only the failures, so an interrupted
  batch is resumed by the same command that started it;
- `Ctrl-C` cancels the tasks in flight, starts none of the pending ones, and keeps what was
  already written.

For twenty items a single agent with subagents is simpler, and worth preferring. `mani batch`
earns its place when the list is long or the work has to be resumable: a master agent pays the
context of every item on **every** call, and when compaction trims it forgets what it already
did — without failing.

A batch is a flow of one step. When one agent is not enough, the next section is the same
machinery with more.

## 4. Pipelines: flows

A file that declares `flow:` wires manifests into a pipeline, and the same `mani run` executes it:

```yaml
# letters.flow.yaml
flow: idea_letters
about: "Rebuilds the timeline of a busta from its transcribed letters"

steps:
  - step: fetch_letters
    does: "Downloads the transcribed letters of the busta"
    run: python fetch.py --busta ${BUSTA}

  - step: extract_facts
    does: "Reads one letter and extracts sender, recipient, place and date"
    agent: extract.yaml
    for_each: fetch_letters
    jobs: 4

  - step: build_timeline
    does: "Resolves people and places and orders the letters in time"
    run: python merge.py
    from_all: extract_facts

result: build_timeline
limits: { tokens: 2000000 }
```

```bash
mani validate --config letters.flow.yaml        # the flow read aloud: steps, data, order
mani run --config letters.flow.yaml --out runs/
```

Steps run in the order written and may only read the steps **above** them, so the graph is acyclic
by construction. `for_each` runs one agent per record, `jobs` at a time; `from_all` runs once over
every record; a `run:` step is argv with no shell, reading JSONL on stdin and printing JSONL on
stdout. Every step keeps its records under `--out`, and a record newer than what it was made from
is not made again — so rerunning a finished flow calls no model at all, and one more input costs
one more run.

The flags are `--out`, `--in`, `--limit` and `--verbose`; `--task`, `--image` and `--provenance`
belong to an agent and are refused on a flow. Full reference in [flow.md](flow.md).

## 5. Trigger daemon (long-lived)

Omit `--task` and the manifest's triggers drive the runtime. The scheduler is **in-process**, so
the same binary and the same manifest work on Linux, macOS and Windows — no systemd/cron needed.

```yaml
run:
  triggers:
    - { type: every,   every: 30m, name: disk-watch, prompt: "Report partitions above 85%." }
    - { type: daily,   at: "02:00", name: nightly, catch_up: true, prompt: "Summarize anomalies." }
    - { type: webhook, addr: "127.0.0.1:8787", prompt: "Handle this event: {{body}}" }
  scheduler:
    path: ./queue          # durable: tasks survive restarts and crashes
    concurrency: 2         # runs executed in parallel
    retry: { max_attempts: 3, backoff: 30s }
```

### Webhooks

Several webhook triggers share **one listener** and get **one route each**:

```yaml
run:
  triggers:
    - type: webhook
      addr: 127.0.0.1:8787          # declared once; the listener is one
      path: /deploy                  # default: /hook
      token: ${DEPLOY_HOOK_TOKEN}
      name: deploy
      prompt: "Deploy requested: {{body}}"
    - type: webhook
      path: /alert
      token: ${ALERT_HOOK_TOKEN}
      memory: persistent
      prompt: "Triage this alert: {{body}}"
```

```bash
curl -XPOST -H "Authorization: Bearer $DEPLOY_HOOK_TOKEN" \
     -d '{"version":"1.2.3"}' 127.0.0.1:8787/deploy
```

Each route carries its own token, prompt and memory, so revoking one secret leaves the others
working. Two triggers may not share a `path`, and they may not declare different `addr` values —
`mani validate` rejects both.

Every webhook needs a token; the daemon refuses to start without one:

```bash
mani run --config agent.yaml
mani run --config agent.yaml --insecure   # dev only: no authentication
```

`token` is resolved per trigger, falling back to `MANI_WEBHOOK_TOKEN` when the field is absent —
so a manifest written before 0.1.4, with neither `path` nor `token`, keeps answering on `/hook`
with the token from the environment. Secrets stay in the environment either way: `${VAR}` is
what the manifest declares, and an undefined variable is a load-time error.

The request body flows into the prompt, so an unauthenticated endpoint is prompt injection with
tool access. The body is capped at 64 KB. An `addr` without a host (`:8787`) listens on **every**
interface — use `127.0.0.1:8787` to keep it local; mani warns when it doesn't.

Each trigger firing becomes one **task**. With `scheduler.path` set, the queue is a directory
(`pending/ running/ done/ failed/`) you can inspect with `ls`: a task's state *is* the directory
it sits in, so recovery is a `mv`. Tasks survive a crash, failed ones are retried with backoff,
and a `daily` trigger with `catch_up: true` recovers a firing missed while the process was down.
Workers start on their own — there is nothing to declare to consume the queue.

See [`_examples/demo/unattended.tape`](../_examples/demo/) for this happening under a `SIGKILL`.

## 6. Inspecting the journal

Every run leaves a record; `mani runs` reads it without a server running.

```bash
mani runs --config agent.yaml                  # the last 20 runs
mani runs --config agent.yaml --status error --since 24h
mani runs --config agent.yaml --json | jq '.[].summary.blocked'
mani runs --path ./runs                        # a JSONL journal directory
mani runs --path ./runs.db                     # or a SQLite journal file
```

The manifest journal uses JSONL by default. Set `observability.journal.backend: sqlite` and
point `path` at a database file when run history should be queried from one compact, indexed store;
the same backend is used by the server and by `mani runs --config`. `retention` limits stored runs
for either backend.

```
ID            STATUS  STARTED              DURATION  TOKENS   TOOLS  BLOCKED
82fdb6ffaa1b  ok      2026-08-31 18:06:10  3.6s      681/96   2      1
a159f37dccb6  error   2026-08-31 18:05:02  1.2s      412/18   1      0
```

One run in full, as a timeline. A unique id prefix is enough, as with `git`:

```bash
mani runs --config agent.yaml 82fdb6
```

```
run 82fdb6ffaa1b  ok  2026-08-31 18:06:10 → 18:06:13 (3.6s)
source: trigger:every   tokens: 681 in / 96 out   tools: 2   blocked: 1

18:06:11.8  tool_call      read       {"path":"incident.log"}
18:06:11.8  tool_result    read       ok  559 bytes
18:06:12.1  tool_call      delegate   {"agent":"researcher"}
18:06:12.4     |- llm_call     messages=4 tools=2
18:06:12.9     |- tool_result  read  ok  1204 bytes
18:06:13.1  guardrail      bash       deny  "recursive delete"
18:06:13.6  run_end        ok
```

Subagent events are indented: the journal is a flat log *read* as a tree, which is why
every event carries a depth.

## 7. Agent server (REST + WebSocket)

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
approvals work over the wire. Full protocol in [agent-server.md](agent-server.md).

## 8. MCP server

```bash
mani mcp --config agent.yaml
```

Serves the manifest to an MCP client over stdio. You rarely run this by hand: the client
launches it. In Claude Desktop's `claude_desktop_config.json`:

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

In Claude Code:

```bash
claude mcp add reviewer -- mani mcp --config /absolute/path/to/reviewer.yaml
```

**What the client sees.** One tool, and it is the whole agent — not the agent's own tools, which
would hand the client the capabilities while leaving the policy behind.

| Manifest | MCP |
|---|---|
| `identity.name` | the tool name — 1-64 characters from `a-z A-Z 0-9 _ -` |
| `identity.description` | the tool description, and the server instructions: this is what the calling model reads to decide whether to use the agent, so write it for a model |
| — | input: `{"task": "..."}` |
| `output.schema` | the tool's output schema; the result comes back as structured content *and* as its JSON in text, for clients that only read text |

**How a call runs.**

- **Every call is a fresh run.** No memory carries over between calls: a tool is a function,
  not a conversation.
- **Permissions are fail-closed.** A client has no way to answer an `ask`, so `ask` resolves to
  deny and the run continues with the tool refused. Design manifests for MCP with
  `allow`/`deny`.
- **A failed run is a tool error, not a protocol error.** The calling model sees the reason and
  can react. The same goes for a missing or malformed `task`.
- **Policy, limits and the journal apply unchanged.** Runs are journaled with source `mcp`, so
  `mani runs --config agent.yaml` shows what was done from inside the editor.

**Things that bite.**

- **Use absolute paths.** The client decides the working directory, not you. That applies to
  `--config` and, more importantly, to `capabilities.workspace`: left empty it defaults to the
  working directory, which under a client may be anywhere — set it explicitly.
- **Environment variables come from the client.** `${VAR}` in the manifest resolves against the
  environment the client launches mani with, which is usually not your shell's. Most clients
  accept an `env` map next to `args`.
- **stdout is the protocol.** Logs go to stderr, which is where clients collect them. A tool that
  prints to stdout cannot break the stream: tool output is captured, never inherited.

Only stdio is implemented. MCP over HTTP, subagents exposed as separate tools, and answering
`ask` through MCP elicitation are deliberately deferred.

## 9. Subprocess tools

A tool is any executable: mani writes the JSON input on **stdin**, reads the result from
**stdout** (stderr on a non-zero exit becomes the error the model sees). Declare `risk`
(`none` / `network` / `write` / `execute`, default `execute`) so the permission layer can gate it.

```yaml
capabilities:
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

A worked example, eight lines of Python: [`_examples/demo/disk.py`](../_examples/demo/disk.py)
with [`_examples/demo-polyglot.yaml`](../_examples/demo-polyglot.yaml).

## 10. Library usage

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
