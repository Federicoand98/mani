---
title: Runs and the journal
description: What a run is in mani, what the journal records, and how a result can point back to the run that produced it.
weight: 4
---

A **run** is one execution of a task: the input, every model call, every tool call, the decisions
governance made, and the answer. It is the unit mani records, counts and resumes.

One run is not one conversation. In the chat, each message you send is a run on a shared session.
Headless, each `mani run --task` is one run. In a batch or a flow, each record is its own run with
a fresh session, which is why a thousand tasks cannot poison each other's context.

## What the journal holds

With a journal configured, each run leaves a record:

```yaml
observability:
  journal:
    enabled: true
    path: ./runs
```

The record holds the run id, its source, start and end, status, the counters (tokens, tool calls,
blocked calls), the structured result if the manifest declared one, and the full event list:

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

Subagent events are indented. The journal is a flat log read as a tree: every event carries a
depth, and the reader does the nesting.

## The source field

Every run records where it came from: `cli`, `tui`, `server`, `mcp`, `batch`, `flow`,
`trigger:<name>`. That one field answers the question you will actually have in front of a
surprising record, which is *who started this*.

## Two shapes on disk

`jsonl` is the default: a directory, one append-only file per run. You can `cat` it, grep it, and
copy it somewhere else.

`sqlite` stores the same records in one indexed file, and is the right choice once there are
thousands of runs: listing reads run headers from an indexed table instead of folding every
event of every run. The port is the same, so `mani runs`, the HTTP API and the filters do not
change.

```yaml
observability:
  journal: { enabled: true, backend: sqlite, path: ./runs.db, retention: 2000 }
```

`retention` caps how many runs are kept.

## Reading it

```bash
mani runs --config agent.yaml                      # the last 20
mani runs --config agent.yaml --status error --since 24h
mani runs --config agent.yaml 82fdb6               # one run, as a timeline
mani runs --path ./runs.db --json | jq '.[].summary.blocked'
```

A unique id prefix is enough, the way `git` works. The server exposes the same data on `GET /runs`
with the same filters.

## Provenance: a result that knows where it came from

A result that leaves mani can carry the run that produced it:

```bash
mani run --config classify.yaml --task "the parcel never arrived" --provenance
```

```json
{
  "result": { "sentiment": "negative" },
  "run": {
    "id": "8f2a1c4b7e90", "source": "cli", "provider": "ollama",
    "model": "qwen3.5:9b", "manifest": "classify.yaml",
    "started_at": "2026-10-09T18:06:10Z", "ended_at": "2026-10-09T18:06:13Z",
    "in_tokens": 412, "out_tokens": 23
  }
}
```

Without the flag the result is bare, because `output.schema` declares what a run returns and
wrapping it by default would make every manifest describe a sub-object. Batch and flow records
always carry the envelope, since a file of results that cannot be traced is a file nobody can
audit. Either way the journal holds the same facts: `run.id` is what `mani runs <id>` takes.

Next: [orchestration](../orchestration/).
