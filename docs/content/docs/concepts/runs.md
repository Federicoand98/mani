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

The record holds:

- the run id, its source, start and end, and its status;
- who ran it: the agent's name, the manifest it was read from, the provider and the model;
- the task it was given and the answer it gave, or the error that stopped it;
- the limits in force when it started (`max_tokens`, `max_tool_calls`, `max_iterations`,
  `max_duration`), so a later change to the manifest does not rewrite what this run was allowed;
- the counters: tokens, tool calls, blocked calls, masked results;
- the structured result, if the manifest declared one;
- the full event list.

```
run 82fdb6ffaa1b  error  2026-08-31 07:00:00 → 07:00:41 (41s)
source: daily   tokens: 9870 in / 612 out   tools: 6   blocked: 2   masked: 0
agent:  monitor (agents/monitor.yaml, ollama / qwen3.5:9b)
task:   Morning check: report partitions above 85% and what fills them.

07:00:01.2  tool_call      bash       {"command":"df -h"}
07:00:01.4  tool_result    bash       ok  612 bytes
07:00:07.0  tool_call      delegate   {"agent":"researcher"}
07:00:08.8     |- tool_result  bash  ok  74 bytes
07:00:14.1  tool_call      bash       {"command":"journalctl --vacuum-size=2G"}
07:00:14.1  permission     bash       unattended
07:00:15.0  guardrail      bash       deny  "no deleting"
07:00:41.3  run_end        error

error  budget: max_tokens reached (limit: 10000)
```

Each tool result keeps a preview of its output, the first 2 KB, written after redaction and
truncation: the journal never holds what the model itself was not shown. Task and answer are kept
up to 16 KB each.

Subagent events are indented. The journal is a flat log read as a tree: every event carries a
depth, and the reader does the nesting.

## Who answered an "ask"

A tool set to `ask` leaves a `permission` event with the answer it got:

| Decision | Meaning |
|---|---|
| `allowed` | a person allowed this call (on a trigger with an allowing policy: the policy did) |
| `allowed_always` | a person allowed the tool for the rest of the session |
| `remembered` | allowed earlier, nobody was asked this time |
| `denied` | a person said no |
| `expired` | someone could have answered, nobody did within 10 minutes |
| `unattended` | nobody to ask: a trigger, `mani run`, an API call |

The last three count as blocked calls, with the calls a rule or the policy denied.

Runs recorded by an older mani have none of the fields above beyond the counters and the events:
they read as runs of an unknown agent.

## The source field

Every run records where it came from: `cli`, `tui`, `web`, `server`, `mcp`, `batch`, `flow`, or
the trigger that started it, `daily`, `cron` or `webhook`. That one field answers the question you will actually have in front of a
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
with the same filters, and `mani web` shows the runs of every agent in its folder on its Runs page.

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
