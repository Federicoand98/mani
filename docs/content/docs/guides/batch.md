---
title: Process a file of tasks
description: Run one agent over a JSONL file with mani batch, in parallel, resumable, with failures kept apart from results.
weight: 3
---

Goal: a thousand inputs through one agent, running several at a time, where an interruption costs
you the records that were in flight and nothing else.

Prerequisites: a manifest that runs headless, ideally with an `output.schema` so the results are
typed.

## Write the input

One JSON object per line:

```json
{"id": "r01", "task": "Boils fast and looks great on the counter.", "product": "kettle"}
{"id": "r02", "task": "The lid broke after two weeks.", "product": "kettle"}
{"id": "r03", "task": "Burns one side of the bread every time.", "product": "toaster"}
```

- `id` is required, must be unique, and must work as a file name: it becomes one.
- `task` is what the agent receives. A `task` that is not a string is passed as JSON, which is
  how a structured input reaches the model.
- Every other field travels through to the result untouched. A shelfmark, a ticket number or a
  product name comes back next to the answer, so you never have to join two files afterwards.

## Run it

```bash
mani batch --config classify.yaml --in reviews.jsonl --out out/ --jobs 4
```

```
batch: tasks: 3 to run, 0 already done
  [1/3] r01  ok
  [2/3] r02  ok
  [3/3] r03  ok
batch: tokens: 1236 in, 84 out
```

Progress is on stderr. With `--out`, each result is a file:

```json
{
  "id": "r02",
  "product": "kettle",
  "result": { "sentiment": "negative", "problem": "lid broke after two weeks" },
  "run": { "id": "8f2a1c4b7e90", "source": "batch", "provider": "ollama",
           "model": "qwen3.5:9b", "in_tokens": 412, "out_tokens": 28 }
}
```

Without `--out` the records stream to stdout as they finish, so a batch fits in a pipe:

```bash
mani batch --config classify.yaml --in reviews.jsonl | jq -c '{id, s: .result.sentiment}'
```

## What happens when things go wrong

This is the reason to use `mani batch` instead of a shell loop.

**The input is checked before the first model call.** A malformed line, a missing or repeated
`id`, an `id` that could climb out of the output directory, a task that is empty: all of them
fail immediately, naming the line. A bad file costs you nothing instead of failing after six
hundred successful runs.

**A failed record writes no result file.** The reason is appended to `<out>/errors.jsonl`, and the
command exits `1`:

```json
{"id":"r02","error":"ollama: HTTP 500: internal error","at":"2026-10-09T18:12:04Z"}
```

**Rerunning retries only what is missing.** A record whose `<id>.json` exists is skipped:

```bash
mani batch --config classify.yaml --in reviews.jsonl --out out/ --jobs 4
```

```
batch: tasks: 1 to run, 2 already done
```

That is also how you handle a provider outage: rerun the same command when it comes back.

**Ctrl-C is safe.** Records in flight are cancelled and write nothing; records not yet started do
not start; everything already written stays. The next run continues.

## Try it on a sample first

```bash
mani batch --config classify.yaml --in reviews.jsonl --out out/ --limit 5
```

Five records, then stop. Read them, fix the prompt if the answers are wrong, delete `out/` and
run the whole file.

## When a batch is the wrong tool

For twenty items, a single agent with subagents is simpler. For several stages, where one stage
feeds the next, use a [flow](../flows/) — a batch is a flow of one step, so you are already
holding the smaller version of it.

## What you should see

- One `<id>.json` per input line in `--out`, each carrying your extra fields and a `run` object.
- `errors.jsonl` only when something failed, with one line per failed attempt.
- A rerun that reports `0 to run, N already done` and makes no model calls.

Next: [build a pipeline](../flows/).
