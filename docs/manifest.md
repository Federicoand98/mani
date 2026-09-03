# Manifest reference

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

**Where does a new setting go?** Walk the list top to bottom; the first "yes" wins. Only records
something → `observability`. Decides when it starts → `run`. Is a numeric ceiling → `limits`.
Can block → `policy`. Constrains the answer → `output`. Changes what the model sees → `context`.
Adds an ability → `capabilities`. Changes who reasons → `identity`.

```yaml
identity:
  name: nightly-maintainer       # identifies the agent
  description: "..."             # what it is for
  provider: anthropic            # ollama | openai | anthropic | copilot | openrouter
  model: claude-sonnet-5
  prompt: "..."                  # the system prompt
  # prompt: !include ./prompts/maintainer.md    # …or keep it in its own markdown file

capabilities:
  workspace: .                   # default: cwd; every fs tool is confined to it
  tools: [read, edit, write, delete, glob, grep, bash, fetch, planning, delegate]
  mcp:
    - { name: deepwiki, url: https://mcp.deepwiki.com/sse }
  subagents:                     # named delegates; reachable via the `delegate` tool
    - name: researcher
      description: "read-only code exploration, reports file:line"
      prompt: "..."
      tools: [read, grep]
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
  network:                       # applies to every tool with risk `network`
    allow: ["api.github.com", "*.wikipedia.org"]
    deny:  ["*.internal"]

limits:
  max_tokens: 50000
  max_tool_calls: 20
  max_duration: 2m
  max_iterations: 15
  tool_timeout: 15s
  subagent_depth: 5

run:
  triggers:                      # see docs/usage.md
    - { type: every, every: 30m, prompt: "..." }
    - { type: daily, at: "02:00", catch_up: true, prompt: "..." }
    - type: webhook
      addr: 127.0.0.1:8787       # one listener for every webhook trigger
      path: /deploy              # one route each; default /hook
      token: ${DEPLOY_TOKEN}     # per route; falls back to MANI_WEBHOOK_TOKEN
  scheduler:                     # how triggers are executed — NOT a list of tasks
    path: ./queue                # present ⇒ the queue survives restarts
    concurrency: 1
    max_pending: 64
    retry: { max_attempts: 3, backoff: 30s }

observability:
  tracing: true
  journal: { enabled: true, path: ./runs, retention: 200 }
```

## Built-in tools

| Tool | Risk | What it does |
|---|---|---|
| `read` `glob` `grep` | none | read a file, list paths by pattern, search a regex |
| `write` `edit` `delete` | write | change files inside the workspace |
| `fetch` | network | HTTP GET, HTML reduced to text |
| `bash` | execute | run a shell command; the shell is detected and named in the tool description |
| `planning` | none | the agent keeps its own checklist |
| `delegate` | none | hand work to a named subagent |

`planning` and `delegate` are ordinary tools, not flags — declare them to enable them. A tool's
manifest key is always its runtime name, so `policy` and subagent references resolve against the
same vocabulary.

## Risk levels

`none` · `network` · `write` · `execute`. Only `none` runs in parallel and ungated, which is why
reaching the network is its own level: a "read-only" GET marked `none` would be a free
exfiltration channel for a file the agent just read. Network tools are additionally confined by
`policy.network`, and refuse private, loopback and link-local addresses at dial time — a
allowlisted name that resolves to `127.0.0.1` must not reach the agent's own server.

## Environment variables

A manifest is meant to be committed, so anything secret is *referenced*, never written:

```yaml
capabilities:
  tools:
    - name: deploy
      command: ./deploy.sh
      env: { API_TOKEN: ${DEPLOY_TOKEN} }
```

The rules are deliberately narrow:

| | |
|---|---|
| Only `${VAR}` with braces | a bare `$VAR` would false-positive in every bash command and regex pattern |
| Only values, never keys | a key is structure, not data |
| Never inside block scalars (`\|`, `>`) | `identity.prompt` is prose; eating its `${}` would be a bug that surfaces much later |
| String fields only | interpolating into a number or a boolean is not supported |
| An undefined variable is an **error** | not an empty string: a blank token would silently mean "authentication disabled" |
| Not expanded inside `!include`d files | those are prose too |

`mani validate` resolves them, so a missing variable fails in CI rather than at 3am.

## `!include`

A real system prompt is a hundred lines, and YAML is a bad place for it:

```yaml
identity:
  prompt: !include ./prompts/reviewer.md
```

Paths are relative **to the manifest**, not to the working directory, so
`mani run --config ~/agents/x.yaml` from anywhere still finds `~/agents/prompts/`. Absolute paths
are refused and the file is capped at 256 KB. `mani validate` checks that includes resolve, so a
missing file fails in CI rather than at 3am.

## Examples

Runnable manifests in [`_examples/`](../_examples/):

| File | Shows |
|---|---|
| `manifest-minimal.yaml` | the smallest useful agent |
| `manifest.yaml` | all eight blocks, with subagents and MCP |
| `guarded.yaml` | the three policy levels plus limits |
| `sentiment.yaml` | structured output |
| `observability.yaml` | journal + trigger |
| `queue.yaml` | durable scheduler |
| `demo-triage.yaml` `demo-unattended.yaml` `demo-polyglot.yaml` | the three recorded demos |
