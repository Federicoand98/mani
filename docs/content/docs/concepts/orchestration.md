---
title: Orchestration
description: "One run, subagents, a batch or a flow: four ways to do more than one thing, and how to choose between them."
weight: 5
---

Four ways to run more than one thing in mani. They differ on one question: **who decides what
happens next.**

| | Who decides | Memory | Resumable |
|---|---|---|---|
| One run | the model, inside one context | one conversation | no |
| Subagents (`delegate`) | the model, at run time | one per subagent, empty at the start | no |
| `mani batch` | you, over a list | one per record | yes |
| A flow | you, over a graph | one per record per step | yes |

## One run

The default. A task, a model, its tools, until it answers. Everything shares one context, so the
agent remembers what it read two tool calls ago.

The ceiling is that same context. A long job fills the window, compaction starts trimming, and
the agent keeps going with less of the history than it needs. It does not fail; it gets quietly
worse. That is the point at which one of the other three starts paying off.

## Subagents

`delegate` hands a sub-task to a named agent with its own prompt, tools and model:

```yaml
capabilities:
  tools: [read, grep, delegate]
  subagents:
    - name: researcher
      description: "read-only code exploration, reports file:line"
      prompt: "Explore and report. Never edit."
      tools: [read, grep]
```

The model decides when to delegate. The subagent starts with **empty memory** and returns only
its final answer, so a long exploration costs the parent one tool result instead of fifty
messages. Its own policy applies: a read-only researcher stays read-only even when the parent can
write.

Use it when the work needs judgement that you cannot script, and when the sub-task's transcript
is noise for the caller.

## Batch

One agent over a file of tasks, one line in and one record out:

```bash
mani batch --config classify.yaml --in reviews.jsonl --out out/ --jobs 4
```

You decide the list, so nothing depends on the model's judgement about what to process next. Each
record is an independent run with a fresh session, which means `--jobs` can run several at once
and a thousand records cost a thousand small contexts instead of one enormous one.

It resumes. A record already in `--out` is skipped, and a failed one is retried by rerunning the
same command.

The alternative people try first is a master agent with subagents over the same list. For twenty
items that is simpler and fine. For a thousand it fails in a specific way: the master pays the
context of every item on every call, and when compaction trims, it forgets which items it already
handled, without failing. A batch cannot forget, because the list is a file and the progress is
on disk.

## Flows

A flow chains manifests into a pipeline:

```yaml
flow: idea_letters
about: "Rebuilds the timeline of a busta from its transcribed letters"

steps:
  - step: fetch_letters
    does: "Downloads the transcribed letters"
    run: python fetch.py --busta ${BUSTA}

  - step: extract_facts
    does: "Reads one letter, extracts sender, recipient, place and date"
    agent: extract.yaml
    for_each: fetch_letters
    jobs: 4

  - step: build_timeline
    does: "Resolves people and places, orders the letters in time"
    run: python merge.py
    from_all: extract_facts

result: build_timeline
```

The shape is yours, written down and checkable with `mani validate`. Steps run in the order
written and may only read the steps above them, so the graph is acyclic by construction. A step
is either an agent or a plain command, which is how data reshaping stays in a real language
instead of becoming a template syntax in YAML.

Resuming follows make's rule: a record newer than what it was made from is not made again. A
finished flow rerun calls no model; one new input costs one new run; a step that produces
identical output leaves everything downstream alone.

A flow is deliberately not a graph framework. No shared mutable state, no cycles, no conditional
edges. Where the branch depends on judgement, use an agent; where the shape is known in advance,
use a flow.

## Choosing

- The work is one task → **one run**.
- The work needs a focused sub-task whose transcript the caller does not need → **subagents**.
- The work is the same task over a known list → **batch**.
- The work is several stages, where one stage's output feeds the next → **a flow**.

Next: the [glossary](../glossary/), or the guide for [building a pipeline](../../guides/flows/).
