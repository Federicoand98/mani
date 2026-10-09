

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
`$XDG_DATA_HOME/mani/auth.json` (default `~/.local/share/mani/auth.json`, mode 0600), managed
with `/login` in the TUI.

## Inside your editor

`mani mcp` serves a manifest as an MCP server over stdio, so any MCP client — Claude Desktop,
Claude Code, an IDE, another agent — can call it. The whole agent is **one tool**: its name is
`identity.name`, its description is `identity.description`, and if the manifest declares
`output.schema` the client sees that too.

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

```bash
claude mcp add reviewer -- mani mcp --config /absolute/path/to/reviewer.yaml
```

Policy, limits and the journal still apply, because they live in the runtime and not in the
transport. **An agent called from inside an editor leaves the same audit trail** as one started
by a trigger — `mani runs --config reviewer.yaml` lists its runs with source `mcp`.

## More than one agent

A file that declares `flow:` wires manifests into a pipeline, and the same command runs it. Each
step is an agent or a plain command that already works on its own — the flow adds no behaviour of
its own, it only says who reads whose output.

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
    for_each: fetch_letters      # one run per record, 4 at a time
    jobs: 4

  - step: build_timeline
    does: "Resolves people and places and orders the letters in time"
    run: python merge.py
    from_all: extract_facts      # one run, over every record

result: build_timeline
limits: { tokens: 2000000 }      # a ceiling for the whole pipeline
```

```bash
mani validate --config letters.flow.yaml   # the flow read aloud: what each step does, and reads
mani run --config letters.flow.yaml --out runs/
```

A step may only read the steps **above** it, so the graph is acyclic by construction and the file
reads top to bottom. What travels between steps is a record — `{"id": …, "task": …}` plus any
fields of your own, which ride along untouched — so one agent's structured answer becomes the
next one's task with no glue in between, and there is no template language to learn. A `run:`
step is argv with no shell: JSONL in, JSONL out.

**Resuming is make's rule.** Every step keeps its records under `--out`, and a record newer than
what it was made from is not made again: rerunning a finished pipeline calls no model at all, one
more input costs one more run, and a step that produces identical output leaves everything
downstream alone. A record that fails stops the flow before the next step — so nothing ever
computes on half its input — and the next run retries only what failed.

One agent over a file of tasks is the same machinery with one step, so it gets a shortcut:

```bash
mani batch --config classify.yaml --in reviews.jsonl --out out/ --jobs 4
```

Every record carries the run that produced it — id, model, manifest, tokens — and `mani run
--provenance` adds the same envelope to a single run, so a result that travels somewhere else can
still be traced back to the journal.

It is deliberately **not** LangGraph: no shared mutable state, no cycles, no conditional edges.
Where judgement is needed you use an agent; where the shape is known you use a flow.

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
| [Usage](docs/usage.md) | CLI, batch, flows, trigger daemon, agent server, subprocess tools, library |
| [Flows](docs/flow.md) | the pipeline file: steps, records, resuming, budget |
| [Agent server](docs/agent-server.md) | the REST + WebSocket protocol in full |
| [Agentic loop](docs/agentic-loop.md) | where hooks fire, where permissions gate |
| [`_examples/`](_examples/) | runnable manifests |

## Status

| Area | State |
|---|---|
| Declarative manifest (8 blocks) + headless `run` | ✅ |
| CLI: `init`, `validate`, `run`, `batch`, `runs`, `serve`, `mcp`, `tui`, `--version` | ✅ |
| Providers: Ollama, OpenAI, Anthropic, GitHub Copilot, OpenRouter | ✅ |
| Tools: `read` `write` `edit` `delete` `glob` `grep` `bash` `fetch` `planning` `delegate` | ✅ |
| MCP client, subprocess tools in any language | ✅ |
| MCP **server** mode: the agent as one tool, over stdio | ✅ |
| Policy: allow/ask/deny, risk levels incl. `network`, rules, redaction, per-run limits | ✅ |
| Triggers (every / daily / webhook) + durable queue that survives crashes | ✅ |
| Structured output (typed response schema) | ✅ |
| Run journal / audit trail (`mani runs`, `GET /runs`), JSONL or SQLite | ✅ |
| Agent server (REST + WebSocket, bearer auth) | ✅ |
| Sessions, planning, subagents, hooks, tracing, compaction, image input | ✅ |
| Flows: a pipeline of manifests, resumable, with a budget for the whole run | ✅ |
| `mani batch`: one agent over a JSONL file, resumable, with `--jobs` | ✅ |
| Provenance on results (`--provenance`) + the result in the journal | ✅ |
| Vocabularies from a file (`enum: !include`) + deep schema validation | ✅ |
| Asynchronous human approval · notification channels | 🚧 next |
| Python SDK · container images | 🗺️ roadmap |

## Architecture

Hexagonal (Ports & Adapters). The single invariant: **`core/` has zero external dependencies.**
Dependency arrows always point inward.

```
cmd/mani/      composition root — TUI, run, batch, serve, mcp, init, validate, runs,
               and the flow executor (one engine: a batch is a flow of one step)
app/           application service — Runtime, events, manifest + flow spec, policy,
               limits, journal, task queue, subagents, triggers
server/        driving adapter — REST + WebSocket
server/mcpserver/  driving adapter — MCP server over stdio (the agent as one tool)
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
a `Journal` port; a SQLite adapter is a drop-in alternative behind one manifest key.

```bash
go build ./... && go test ./...
```

## Roadmap

1. **Asynchronous human approval** — an unattended agent pauses on a sensitive action and waits
   for an ok, instead of having to choose `allow` or `deny` up front. The feature that makes
   triggers usable for work that matters.
2. **Manifest composition** — one agent as a tool of another, where the *model* decides. Flows
   cover the case where the shape is known in advance; this covers the case where it is not.
3. **Python SDK** — drive the runtime over the agent server.

Feature filter: does it deepen manifest expressiveness, safe autonomy, or operability as a
service? If not, it's out of scope.

## Stability

**This is a learning project.** It works, it's tested, and it's honest about what it isn't: the
public API is **unstable until 1.0** — packages, manifest keys and tool names may change between
minor versions. Pin a version if you depend on it.

## License

[Apache License 2.0](LICENSE).
