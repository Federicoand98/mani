---
title: Flow file
description: Every key of a flow file, the record format that travels between steps, the resume rules, and the flags of mani run on a flow.
weight: 3
---

A flow chains manifests into a pipeline. A file whose top-level key is `flow:` is a flow, and
`mani run` and `mani validate` tell it apart from a manifest by that key rather than by its name.

```yaml
flow: idea_letters                 # a name: lowercase letters, digits, underscore
about: "Rebuilds the timeline of a busta from its transcribed letters"
input: "One letter per line"       # optional; its presence makes --in required

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
limits: { tokens: 2000000 }        # optional, for the whole flow
```

## Keys

| Key | Meaning |
|---|---|
| `flow` | the name; names the state directories and prefixes every log line |
| `about` | **required**, one sentence: what the pipeline is for |
| `input` | one sentence describing what `--in` must contain; present means `--in` is required |
| `steps[].step` | the step's name: unique, `snake_case`, and `input` is reserved |
| `steps[].does` | **required**, one sentence: what this step does |
| `steps[].agent` | a manifest, resolved against the flow file |
| `steps[].run` | a command; exactly one of `agent` or `run` |
| `steps[].for_each` | run once per record of that step |
| `steps[].from_all` | run once, over every record of that step |
| `steps[].jobs` | runs at a time; only on an `agent` step with `for_each` (default 1) |
| `result` | the step whose records are printed on stdout |
| `limits.tokens` | a cap across every run of every step; 0 or absent means none |

Unknown keys are an error with their line number, and `${VAR}` expands from the environment, both
exactly as in a [manifest](../manifest/).

## Order, and the absence of cycles

Steps run in the order written, one at a time, and a step may only read a step written **above**
it. The file therefore reads top to bottom, and a cycle cannot be expressed. A step reads at most
one other step: branching out is fine, joining two branches back together is not supported.

```
fetch_letters ──► extract_facts ──► build_timeline
   (run)            (agent × N)       (run)
```

## Records

The edge between steps is a record: a JSON object with an `id`, a payload, and anything else you
attach.

```json
{"id": "doc_1904", "task": "…the letter text…", "shelfmark": "b.12 f.3"}
```

- `id` is required, must be a string, unique within the step, and usable as a file name.
- Extra fields travel untouched from step to step. That is the lineage: the shelfmark that went in
  comes back out next to the synthesis.
- An agent step writes `{"id": …, "result": …, "run": …}` plus the extras it received. `run` is
  the [provenance](../../concepts/runs/) envelope.

What an agent receives as its task:

| The step it reads | The task is |
|---|---|
| `input`, or a `run:` step | the record's `task` |
| an `agent:` step | that agent's `result` |

A payload that is not a string is passed as JSON, which is how one agent's structured answer
becomes the next one's task. There is no template language: reshaping is what a `run:` step is
for.

An agent step with `from_all` receives the JSON array of the upstream records, without their `run`
fields, so it can tell them apart by `id` without paying for the bookkeeping.

## `run:` steps

```yaml
run: python merge.py --strict        # a string, split on spaces
run: [python, "my script.py"]        # a list, when an argument contains spaces
```

argv, with **no shell**: no quoting, no pipes, no globs, and the same flow works on Linux, macOS
and Windows. The working directory is the flow's own, so `run: ./merge.py` and relative data paths
behave the same from anywhere.

With `from_all`, or with no input at all, the command runs once. With `for_each` it runs once per
record, in order. Records arrive as JSONL on stdin; the command prints its records as JSONL on
stdout, each with an `id`. Its stderr passes through to yours. A non-zero exit stops the flow,
with the command line and the step in the error.

## State on disk

```
runs/
├── fetch_letters/
│   ├── doc_1904.json
│   └── .done                 # written when a whole-step run finished
├── extract_facts/
│   ├── doc_1904.json
│   └── errors.jsonl          # one line per failed attempt, appended
└── build_timeline/
```

## Resuming

Make's rule: a record that exists and is **newer** than the record it was made from is not made
again.

- Rerunning a finished flow calls no model.
- One more input costs one more run, plus the steps that depend on it.
- A step that reruns and produces identical records does not move their timestamps, so nothing
  downstream reruns.

To make a code step run again, delete its `.done`, not its directory:

```bash
rm runs/fetch_letters/.done
```

Deleting the directory also works, but it throws away the timestamps and redoes everything
downstream. Records from `--in` carry no timestamps: a line is identified by its `id`, so editing
the text of a line already done does not redo it (delete its result file), while appending lines
is always safe.

Without `--out` the flow runs in a temporary directory that is removed at the end: nothing is
kept and nothing can be resumed.

## Failures

A failed record writes no result file and appends the reason to `errors.jsonl`. The flow then
stops before the next step, so no step computes on half its input, and the exit code is `1`. The
next run retries only what failed.

`Ctrl-C` cancels the runs in flight and starts none of the pending ones. A cancelled run writes
nothing and is not counted as a failure.

## Flags

| Flag | Meaning |
|---|---|
| `--out DIR` | keep every step's records here; without it nothing is kept |
| `--in FILE` | the records for `input`; `-` reads stdin |
| `--limit N` | at most N new agent runs per step; the flow proceeds on what there is |
| `--verbose` | logs to stderr; progress is always on stderr, stdout is the result |

`--task`, `--image`, `--provenance` and `--insecure` belong to an agent and are refused on a flow.

## Budget

`limits.tokens` is checked before a run starts, so runs already in flight finish and the total can
pass the cap by at most `jobs` runs. When it is spent the flow stops with an error; raising the cap
and running again continues from the records on disk. Totals print on stderr either way.

## What a flow is not

No shared mutable state, no cycles, no conditional edges, no scheduler. Where the next step
depends on judgement, use an agent or `delegate`; where the shape is known in advance, use a flow.
Conditional steps, declared parameters and joining two branches are deliberately deferred.

Next: [built-in tools](../tools/).
