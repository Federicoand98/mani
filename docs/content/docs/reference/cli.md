---
title: CLI
description: Every mani command and flag, with exit codes and the rules about stdout and stderr.
weight: 1
---

```
mani                      interactive terminal chat
mani run --config         one task, the trigger daemon, or a flow
mani batch --config       one agent over a JSONL file of tasks
mani serve --config       expose the agent over HTTP and WebSocket
mani mcp --config         expose the agent to MCP clients over stdio
mani init                 scaffold a commented agent.yaml
mani validate --config    check a manifest or a flow without running it
mani runs --config        list past runs, or replay one as a timeline
mani tui                  the chat, named explicitly
mani --help  --version
```

## stdout is a format

Every command keeps the two channels separate. Results go to stdout, everything else to stderr:
progress, warnings, the summary of a batch, logs. A run can be piped into `jq` without filtering
anything out, and `mani mcp` can speak JSON-RPC on stdout without a stray log line breaking the
client.

Logs are quiet by default for `run` and `batch`, go to stderr for `serve` and `mcp`, and go to
`~/.config/mani/mani.log` for the chat. `--verbose` moves them to stderr.

## Exit codes

| Code | Meaning |
|---|---|
| `0` | success |
| `1` | the run failed, or a task in a batch or flow failed |
| `2` | you invoked it wrong: bad flag, missing or invalid manifest, malformed input |

A failed task in a batch of a thousand still exits `1`, so a wrapper script notices. The records
that succeeded are written, and the next run retries only the failures.

## `mani run`

Runs an agent manifest or a flow file. Which one it is comes from the file: a file whose top-level
key is `flow:` is a flow.

| Flag | Default | Meaning |
|---|---|---|
| `--config PATH` | required | the manifest or flow file |
| `--task TEXT` | | one task, headless; without it the manifest's triggers start |
| `--image PATH` | | attach an image to the task, repeatable |
| `--provenance` | `false` | wrap the result with the run that produced it |
| `--insecure` | `false` | start webhook triggers without authentication, for local development |
| `--in PATH` | | flows only: the records for the flow's `input`, or `-` for stdin |
| `--out DIR` | | flows only: keep every step's records here, which makes the flow resumable |
| `--limit N` | `0` | flows only: at most N new agent runs per step |
| `--verbose`, `--debug` | `false` | logs to stderr |

Without `--task` and without triggers in the manifest, the command refuses rather than doing
nothing. Flow flags on an agent, or `--task` on a flow, are usage errors that name the flag.

```bash
mani run --config agent.yaml --task "Summarize today's log"
mani run --config agent.yaml --task "what broke here?" --image screenshot.png
mani run --config classify.yaml --task "$(cat review.txt)" --provenance
mani run --config agent.yaml                      # trigger daemon, stays in the foreground
mani run --config letters.flow.yaml --out runs/   # a pipeline
```

Permission requests are denied automatically in headless mode. Nobody is there to answer, and a
run that hangs waiting for a human is worse than one that stops.

## `mani batch`

One agent over a JSONL file: one line in, one record out.

| Flag | Default | Meaning |
|---|---|---|
| `--config PATH` | required | the manifest |
| `--in PATH` | required | the tasks, one JSON object per line; `-` reads stdin |
| `--out DIR` | | one `<id>.json` per task, which makes the batch resumable |
| `--jobs N` | `1` | tasks at a time |
| `--limit N` | `0` | stop after N new tasks |
| `--verbose` | `false` | logs to stderr |

```bash
mani batch --config classify.yaml --in reviews.jsonl --out out/ --jobs 4
mani batch --config classify.yaml --in - < reviews.jsonl | jq -c '.result'
```

Each line needs an `id` that is unique and usable as a file name. Any other field you add travels
through to the result untouched. Without `--out` the records stream to stdout as they finish; with
it they become files, and rerunning skips what is already there.

## `mani serve`

| Flag | Default | Meaning |
|---|---|---|
| `--config PATH` | required | the manifest |
| `--addr HOST:PORT` | `:9000` | listen address |
| `--token TOKEN` | | bearer token; falls back to `MANI_SERVER_TOKEN` |
| `--insecure` | `false` | run without authentication, for local development |

Without a token and without `--insecure` the command refuses to start. Every route sits behind
bearer auth, including the WebSocket.

```bash
export MANI_SERVER_TOKEN=secret
mani serve --config agent.yaml --addr :9000
```

## `mani mcp`

Serves the manifest to any MCP client over stdio. The whole agent is one tool: `identity.name` is
its name and `identity.description` is what the calling model reads.

```bash
mani mcp --config reviewer.yaml
claude mcp add reviewer -- mani mcp --config /absolute/path/to/reviewer.yaml
```

Under `mani mcp` the name is required and must match `^[a-zA-Z0-9_-]{1,64}$`, which is stricter
than MCP itself: clients pass tool names to model APIs that refuse anything else.

## `mani init`

| Flag | Default | Meaning |
|---|---|---|
| `--template NAME` | `agent` | which template to scaffold |
| `-o PATH` | `agent.yaml` | output file |
| `--force` | `false` | overwrite the output file if it exists |

## `mani validate`

Loads a manifest or a flow and reports what it found, without calling a model.

```bash
mani validate --config agent.yaml
```

```
agent.yaml: ok
  identity:     my-agent (ollama / qwen3.5:9b)
  tools:        2
  subagents:    0
  triggers:     0
  output:       structured
```

It resolves included files and `${VAR}` references, checks tool names, policy rules and journal
settings, and on a flow it also loads every manifest the flow names. Warnings, such as an enum
with more than 200 values, go to stderr and do not fail the command.

## `mani runs`

| Flag | Default | Meaning |
|---|---|---|
| `--config PATH` | | read the journal location from this manifest |
| `--path PATH` | | a JSONL directory or a SQLite file, overriding the manifest |
| `--limit N` | `20` | how many runs to list |
| `--status STATE` | | filter by `ok`, `error` or `cancelled` |
| `--since DUR` | | only runs newer than this, for example `24h` |
| `--json` | `false` | JSON instead of the table |

With an id as a positional argument it prints that single run as a timeline. A unique prefix is
enough:

```bash
mani runs --config agent.yaml --status error --since 24h
mani runs --config agent.yaml 82fdb6
mani runs --path ./runs.db --json | jq '.[].summary.blocked'
```

Next: [the manifest](../manifest/).
