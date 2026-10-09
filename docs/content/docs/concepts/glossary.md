---
title: Glossary
description: "The terms mani uses, defined once: manifest, run, record, session, step, flow, subagent, journal, workspace, port."
weight: 6
---

**Agent** — a model in a loop with tools, governed by a manifest. Not a process: an agent exists
when a run starts and is gone when it ends.

**Manifest** — the YAML file that declares an agent. Eight blocks, one question each. See
[the manifest](../manifest/).

**Flow** — a file that declares `flow:` and chains manifests into a pipeline. Steps are agents or
commands, and each may only read the steps above it. See [reference](../../reference/flow/).

**Step** — one stage of a flow. Either `agent:` (a manifest) or `run:` (a command).

**Run** — one execution of one task. The unit mani records, counts and limits. In a flow, each
record of each step is its own run.

**Session** — the memory a run reads and appends to. The chat keeps one session across many runs;
batch and flow give every record a fresh one.

**Record** — two meanings, kept apart by context. In the journal, a record is the stored history
of a run. In a batch or a flow, a record is the JSON object that travels between steps:
`{"id": …, "task": …}` plus whatever fields you attach.

**Journal** — the audit trail of runs, as JSONL files or a SQLite database. Read it with
`mani runs`. See [runs and the journal](../runs/).

**Provenance** — the `run` object attached to a result: run id, source, provider, model, manifest,
timestamps and tokens. Always present in batch and flow records, and in `mani run --provenance`.

**Source** — where a run came from: `cli`, `tui`, `server`, `mcp`, `batch`, `flow`, or
`trigger:<name>`.

**Subagent** — a named delegate declared in `capabilities.subagents` and reached through the
`delegate` tool. Starts with empty memory, returns only its final answer.

**Tool** — something the model can call. Built-in (`read`, `bash`, `fetch`, …), from an MCP
server, or an external process declared in the manifest.

**Risk level** — what a tool can do to the world: `none`, `network`, `write`, `execute`. Governs
whether it is gated and whether it can run in parallel.

**Workspace** — the directory every filesystem tool is confined to, `capabilities.workspace`. A
path outside it is refused however the model asks.

**Policy** — the block that decides what is allowed: per-tool verdicts, pattern rules, redaction,
network allow and deny lists.

**Guardrail** — a policy rule that fired. Recorded in the journal with its label.

**Trigger** — a declaration in `run.triggers` that starts a run by itself: `every`, `daily` or
`webhook`.

**Scheduler** — how triggered runs are queued and executed. With `scheduler.path` set, the queue
survives a restart.

**Structured output** — an `output.schema` in the manifest. The agent answers through a synthetic
`respond` tool, the payload is validated, and an invalid one is sent back for a retry.

**Vocabulary** — an `enum` loaded from a file with `!include`, used to constrain a field to a
known list of values.

**Hook** — a middleware point in the loop (`PreToolUse`, `ContextFull`, …) that can observe,
mutate or abort. Governance is built out of hooks.

**Event** — what flows outward from a run to whatever renders it: tokens, tool calls, permission
requests, the result. Events never change the loop.

**Port** — an interface the core defines and an adapter implements: `LLMClient`, `Tool`,
`Journal`, `Memory`. The reason a new provider or a new journal backend is a package, not a
rewrite.

**Compaction** — dropping the oldest messages when the context window fills, keeping the most
recent `context.compaction.keep`.

Next: the [guides](../../guides/).
