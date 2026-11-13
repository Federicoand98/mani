---
title: Manifest
description: Every block and key of an agent manifest, with defaults, the built-in tools, risk levels, environment variables and file includes.
weight: 2
---

A manifest has eight top-level blocks, and each one answers a single question. Only `identity` is
required.

| Block | Question | Holds |
|---|---|---|
| `identity` | who thinks? | provider, model, prompt |
| `capabilities` | what can it do? | tools, MCP servers, subagents, workspace |
| `context` | what does it see and remember? | window, compaction, injection |
| `output` | what does it return? | the response schema |
| `policy` | what is it allowed to do? | per-tool permissions, rules, redaction, network |
| `limits` | how much may it consume? | tokens, calls, duration, timeouts |
| `run` | when does it start, and how? | triggers, scheduler |
| `observability` | what does it leave behind? | tracing, journal |

When you are unsure where a key belongs, ask what it does. Something that can block an action
belongs in `policy`. Something that constrains the answer belongs in `output`. Something that
changes what the model sees belongs in `context`.

An unknown key is an error with its line number. A manifest that loads is a manifest where every
line does something.

## `identity`

```yaml
identity:
  name: nightly-maintainer       # the tool name under `mani mcp`
  description: "..."             # what an MCP client's model reads
  provider: anthropic            # ollama | openai | anthropic | copilot | openrouter
  model: claude-sonnet-5
  prompt: "..."                  # the system prompt
  # prompt: !include ./prompts/maintainer.md
```

`name` and `description` are optional everywhere except under `mani mcp`, where they become the
agent's public contract: the name identifies the tool, and the description is what another model
reads to decide whether to call it. Write the description for a model, not for a changelog.

If the provider has no credentials or no base URL, the run fails. mani never switches to another
model on your behalf.

`provider` can also name an endpoint you declared yourself: any server that speaks the OpenAI,
Anthropic or Ollama API, such as LM Studio, vLLM, llama.cpp or a company gateway. Add it from the
Providers page of `mani web`, or by hand in `~/.config/mani/config.json`:

```json
"providers": {
  "lmstudio": {
    "base_url": "http://localhost:1234/v1",
    "type": "openai",
    "label": "LM Studio",
    "models": ["qwen2.5-coder-7b"],
    "enabled": true
  }
}
```

`type` says which API the server speaks: `openai`, `anthropic` or `ollama`. `models` is optional
and wins over what the server lists, which helps with endpoints that list nothing. A key, when the
server wants one, goes in `auth.json` like any other; an OpenAI-compatible server without a key gets
no `Authorization` header at all. Then the manifest writes `provider: lmstudio`.

## `capabilities`

```yaml
capabilities:
  workspace: .                   # default: the working directory
  tools: [read, edit, write, delete, glob, grep, bash, fetch, planning, delegate]
  mcp:
    - { name: deepwiki, url: https://mcp.deepwiki.com/sse }       # over HTTP
    - { name: local, command: ./mcp-server, args: ["--stdio"] }   # over stdio
  subagents:
    - name: researcher
      description: "read-only code exploration, reports file:line"
      prompt: "..."
      tools: [read, grep]
      model: ""                  # "" inherits the parent's model
```

`workspace` confines every filesystem tool with a path check, so a path outside it is refused
however the model phrases the request.

A tool entry can be a bare name or an object. The object form is how you declare a tool that is
not built in, as an external process:

```yaml
capabilities:
  tools:
    - read
    - name: lookup_person
      description: "Resolves a name to a person record"
      command: ./lookup.py
      args: ["--strict"]
      risk: none
      schema:
        type: object
        properties:
          name: { type: string }
        required: [name]
```

mani writes the tool input as JSON on the process's stdin and reads the result from stdout, so any
language will do. A non-zero exit turns stderr into the error the model sees, and `env` values can
reference environment variables. Worked examples are in
[`_examples/tools/`](https://github.com/Federicoand98/mani/tree/master/_examples/tools).

### Built-in tools

| Tool | Risk | What it does |
|---|---|---|
| `read`, `glob`, `grep` | none | read a file, list paths by pattern, search a regex |
| `write`, `edit`, `delete` | write | change files inside the workspace |
| `fetch` | network | HTTP GET, with HTML reduced to text |
| `bash` | execute | run a shell command |
| `planning` | none | the agent keeps its own checklist |
| `delegate` | none | hand a sub-task to a subagent |

`planning` and `delegate` are ordinary tools rather than switches: declare them to enable them. A
tool's manifest key is its runtime name, so `policy` and subagent entries refer to the same
vocabulary.

### Subagents

A subagent is a named delegate with its own prompt, tools and model. It starts with **empty
memory** and returns only its final answer to the caller, which is what keeps a long sub-task from
filling the parent's context. `limits.subagent_depth` caps how deep delegation can nest.

## `context`

```yaml
context:
  window: 0                      # no additional manifest ceiling
  inject: true                   # pull AGENTS.md into the system prompt
  compaction: { enabled: true, keep: 20 }
```

Without a `mode`, this is the legacy trim: before each call it keeps the last `keep` messages.
It does not add model calls and does not offer token-budgeted summarization. Existing manifests
keep this behavior; `keep` still counts messages, not turns.

Opt in to managed context explicitly:

```yaml
context:
  window: 131072                 # shared input + output ceiling, optional
  inject: true
  compaction:
    enabled: true
    mode: summary
    keep_turns: 2
limits:
  max_output_tokens: 4096
```

The runtime discovers model limits where the provider exposes them, or uses manual limits
saved for that provider and model. Unknown limits stop the call with a configuration error.
`context.window` can tighten a known shared window, never enlarge it. A separate model input
ceiling is not treated as a shared window: output is subtracted only from the shared ceiling.
`limits.max_output_tokens` is the per-request generation limit, distinct from the cumulative
run budget `limits.max_tokens`.

Before every model request, the runtime estimates messages, system instructions and tool
schemas. Above 80% of the effective input budget it summarizes older complete turns using the
same model without tools, targeting 60%. System instructions, the last two completed turns
and the current turn stay intact. `keep_turns` changes the number of completed turns retained.
If protected content cannot fit, it reports an error instead of silently dropping it.

Compaction takes at most four summary calls, counted in usage and run limits. The full stored
message history remains available; a separate summary checkpoint is saved in the session and
reused after restart. Failed or incomplete summaries do not replace the checkpoint.
Previously clipped tool output is still clipped in that history; this is not an archive of
the original raw tool output.

`mode: summary` with `enabled: false` keeps the preventive budget check but refuses oversized
requests instead of summarizing. Model-only console and terminal chats enable managed context
by default. Agents in those interfaces still obey their manifest.

A recognized context overflow may trigger one additional compaction and one new model request.
This never restarts the turn or reruns tools. No automatic resend happens after response text
or thinking has already been emitted; unknown HTTP 400 errors are not assumed to be overflows.

In the console, **Providers - Model limits** configures the model's capacity, while
**Settings - Reserved output tokens** configures the requested generation limit. These are
different values. The chat meter shows estimated input over effective input budget, not the
sum of input tokens consumed by all calls. Its detail distinguishes the last provider-reported
input from the estimate, and excludes unsent draft text.
Legacy manifest chats also report the actual post-trim request size, without enabling summaries
or adding a preventive input guard. If metadata is unavailable or cannot yield a valid input
budget, the meter shows an unknown budget and the runtime logs the reason. Saving model limits
or the output reserve refreshes cached agent runtimes for the next turn.

For headless use, add `model_limits` to the existing provider entry in `config.json`, keeping
its base URL and credentials unchanged:

```json
{
  "max_output_tokens": 4096,
  "providers": {
    "example-provider": {
      "type": "openai",
      "model": "example-model",
      "model_limits": {
        "example-model": {
          "max_input_tokens": 245000,
          "max_output_tokens": 11000
        }
      }
    }
  }
}
```

This is a partial configuration example, not a replacement for the provider's existing entry.
If the known limit is shared instead, use `context_window`. `MANI_CONTEXT_WINDOW` overrides
the active model's shared ceiling; `MANI_MAX_OUTPUT_TOKENS` overrides requested output.
Ollama managed calls set `options.num_ctx` and `options.num_predict` explicitly; an input-only
manual cap is insufficient unless a shared window can also be discovered.

## `output`

```yaml
output:
  schema:
    type: object
    properties:
      severity: { type: string, enum: [low, medium, high] }
      places:
        type: array
        items: { type: string }
    required: [severity]
```

With a schema, the agent answers through a synthetic `respond` tool and the run returns JSON.
Validation covers array items and nested objects, checking `type`, `required` and `enum` at every
level, and the error names the element it refused, such as `places[1] must be a string`. An
invalid answer goes back to the model with the reason, so what reaches stdout fits the schema.

### Vocabularies from a file

An `enum` can come from a file instead of the manifest, which is what you want for a list of
people, places or product codes:

```yaml
person: { type: string, enum: !include ./people.txt }
```

| File | Read as |
|---|---|
| any other extension | one value per line; blank lines and `#` comments are skipped |
| `.json`, `.yaml`, `.yml` | a list of strings |

It works anywhere in a schema: on a property, on the `items` of an array, inside a nested object,
and in a subprocess tool's own schema. An empty file, a duplicate value or a malformed list is an
error that names the field.

{{< callout type="info" >}}
  Above 200 values `mani validate` warns, without failing. The schema travels with every request,
  so a long list is a cost on each call. A lookup tool over the same list costs one extra call,
  and only when the model uses it.
{{< /callout >}}

## `policy`

```yaml
policy:
  tools:                         # allow | ask | deny
    bash: deny
    default: allow
  rules:
    - { tool: bash, pattern: 'rm\s+-rf', action: deny, label: "recursive delete" }
  redact:
    - { pattern: 'sk-[A-Za-z0-9]{20,}', with: "***REDACTED***" }
  network:
    allow: ["api.github.com", "*.wikipedia.org"]
    deny: ["*.internal"]
```

`ask` stops the run and waits for a decision. In the chat you answer it; over the WebSocket the
client answers it; headless and under `mani mcp` there is nobody to ask, so the call is denied.
Design an unattended manifest with `allow` and `deny`.

`rules` match the tool's arguments before the call happens, so a denied pattern never runs.
`redact` rewrites tool output before the model sees it. Both are recorded in the journal, which is
how you find out that a nightly run kept trying something you had blocked.

### Risk levels

`none`, `network`, `write`, `execute`. Only `none` runs in parallel and ungated. Reaching the
network is its own level because a read-only GET can carry out whatever the agent has just read.
Network tools are further confined by `policy.network`, and refuse private, loopback and
link-local addresses at dial time, so an allowed name that resolves to `127.0.0.1` cannot reach
the agent's own server.

## `limits`

```yaml
limits:
  max_tokens: 50000
  max_tool_calls: 20
  max_duration: 2m
  max_iterations: 15
  tool_timeout: 15s
  subagent_depth: 5              # default 5
```

These are per run. When one is hit, the run stops and the journal records why. They are the
difference between a bug that costs a few cents and a bug that runs all night.

## `run`

```yaml
run:
  triggers:
    - { type: every, every: 30m, name: disk-watch, prompt: "Report partitions above 85%." }
    - { type: daily, at: "02:00", name: nightly, catch_up: true, prompt: "Summarize anomalies." }
    - type: webhook
      addr: 127.0.0.1:8787       # one listener for every webhook trigger
      path: /deploy              # one route each; default /hook
      token: ${DEPLOY_TOKEN}     # per route; falls back to MANI_WEBHOOK_TOKEN
      prompt: "Handle this event: {{body}}"
  scheduler:
    path: ./queue                # present means the queue survives restarts
    concurrency: 1
    max_pending: 64
    retry: { max_attempts: 3, backoff: 30s }
```

`mani run` without `--task` starts these triggers and stays in the foreground. The scheduler runs
in-process, so the same binary and the same manifest work on Linux, macOS and Windows with no cron
and no systemd unit.

`catch_up: true` runs a daily trigger that was missed while the process was down. With
`scheduler.path` set, queued tasks survive a crash and are picked up on restart.

Each trigger starts with fresh memory. Add `memory: persistent` to a trigger that should remember
its previous runs, and give it a `name` so that memory has a stable identity.

## `observability`

```yaml
observability:
  tracing: true
  journal:
    enabled: true
    backend: jsonl               # jsonl (default) or sqlite
    path: ./runs                 # a directory for jsonl, a file for sqlite
    retention: 200
```

The journal records each run: the tools it called, the arguments, the results, the permissions
denied, the tokens spent and the answer it returned. `mani runs` reads it without a server.

Use `jsonl` for a handful of runs you want to read with `cat`. Use `sqlite` when there are
thousands and you want listing to stay fast: run headers live in an indexed table instead of being
recomputed from the events.

## Environment variables

A manifest is meant to be committed, so anything secret is referenced rather than written:

```yaml
capabilities:
  tools:
    - name: deploy
      command: ./deploy.sh
      env: { API_TOKEN: ${DEPLOY_TOKEN} }
```

The rules are narrow on purpose:

- Only `${VAR}` with braces. A bare `$VAR` would match inside every bash command and regex.
- Only in values, never in keys.
- Never inside block scalars (`|`, `>`), because that is where prose lives.
- In string fields only.
- An undefined variable is an error, not an empty string. A blank token would silently mean
  "authentication disabled".
- Not expanded inside `!include`d files.

`mani validate` resolves them, so a missing variable fails in CI instead of at three in the
morning.

## `!include`

A real system prompt is a hundred lines, and YAML is a bad place to keep it:

```yaml
identity:
  prompt: !include ./prompts/reviewer.md
```

Paths are relative to the manifest, not to the working directory, so `mani run --config
~/agents/x.yaml` finds `~/agents/prompts/` from anywhere. Absolute paths are refused and files are
capped at 256 KB. The other place `!include` works is an `enum`, described above.

## Examples

{{< repofile path="manifest.yaml" lang="yaml" >}}

More runnable manifests are in
[`_examples/`](https://github.com/Federicoand98/mani/tree/master/_examples).
