---
title: Flow file
description: Every key of a flow file — parameters, filters and joins included — the record format that travels between steps, the resume rules, and the flags of mani run on a flow.
weight: 3
---

A flow chains manifests into a pipeline. A file whose top-level key is `flow:` is a flow, and
`mani run` and `mani validate` tell it apart from a manifest by that key rather than by its name.

```yaml
flow: idea_letters                 # a name: lowercase letters, digits, underscore
about: "Rebuilds the timeline of a busta from its transcribed letters"
params:                            # optional; values come from --set
  busta: "The busta to rebuild, as numbered in the archive"
  since: { about: "Earliest year to keep", default: "1490" }

steps:
  - step: fetch_letters
    does: "Downloads the transcribed letters of the busta"
    run: python fetch.py --busta ${params.busta} --since ${params.since}

  - step: fetch_people
    does: "Downloads the register of people named in the busta"
    run: python people.py --busta ${params.busta}

  - step: extract_facts
    does: "Reads one letter and extracts sender, recipient, place and date"
    agent: extract.yaml
    for_each: fetch_letters
    jobs: 4

  - step: review_doubtful
    does: "Gives a second reading to the letters the extractor was unsure about"
    agent: review.yaml
    for_each: extract_facts
    where: { confidence: low }

  - step: build_timeline
    does: "Resolves people and places and orders the letters in time"
    run: python merge.py
    from_all: [extract_facts, review_doubtful, fetch_people]

result: build_timeline
limits: { tokens: 2000000 }        # optional, for the whole flow
```

## Keys

| Key | Meaning |
|---|---|
| `flow` | the name; names the state directories and prefixes every log line |
| `about` | **required**, one sentence: what the pipeline is for |
| `input` | one sentence describing what `--in` must contain; present means `--in` is required |
| `params.<name>` | a parameter: a description, or `{about, default}`; see [parameters](#parameters) |
| `steps[].step` | the step's name: unique, `snake_case`, and `input` is reserved |
| `steps[].does` | **required**, one sentence: what this step does |
| `steps[].agent` | a manifest, resolved against the flow file |
| `steps[].run` | a command; exactly one of `agent` or `run` |
| `steps[].for_each` | run once per record of that step |
| `steps[].from_all` | run once, over every record of that step — or of several, as a list |
| `steps[].where` | keep only the records whose fields equal these values; see [filters](#filters-where) |
| `steps[].jobs` | runs at a time; only on an `agent` step with `for_each` (default 1) |
| `result` | the step whose records are printed on stdout |
| `limits.tokens` | a cap across every run of every step; 0 or absent means none |

Unknown keys are an error with their line number, and `${VAR}` expands from the environment, both
exactly as in a [manifest](../manifest/).

## Parameters

```yaml
params:
  busta: "The busta to rebuild"                          # required
  since: { about: "Earliest year to keep", default: "1490" }
```

```bash
mani run --config letters.flow.yaml --set busta=12 --out runs/
```

A parameter is referenced as `${params.name}`, in any value: a command, an agent path, a `does`.
The rules are the ones of `${VAR}`: values only, never keys, never block scalars (`|`, `>`). The
two do not collide — `${ARCHIVE_URL}` comes from the environment, `${params.busta}` from the
command line — and the file says which is which.

- `--set` wins over `default`. A parameter with neither is **required**: `mani validate` shows the
  flow anyway, marking it, and `mani run` refuses to start, naming it and its description.
- `${params.x}` for a parameter the flow does not declare is a load error with its line, and so is
  `--set x=…`. A typo is never silently ignored.
- Names are `snake_case`, and every parameter needs a description: it is what the error says when
  the value is missing.

**A state directory belongs to its parameters.** The first run with `--out` writes
`<out>/.params.json`; a later run with any different value — a different `--set`, or a default
that changed in the file — is refused:

```
runs/ was built with busta=12, and this run has busta=13; use another --out, or remove the
directory to rebuild it
```

Without that, rerunning busta 13 in the directory of busta 12 would find every step "already done"
and hand back busta 12's letters. Without `--out` nothing is recorded, because nothing is kept.

## Filters: `where`

```yaml
  - step: review_doubtful
    agent: review.yaml
    for_each: extract_facts
    where: { confidence: low }
```

A step with `where` sees only the records whose payload has every listed field equal to its value
— the payload being the `result` of an agent, or the `task` of a script or of `--in`. Several
fields are all required to match. There is nothing else: no comparisons, no `or`, no
expressions.

That is deliberate. A branch that needs a threshold gets it from the agent upstream, which declares
a band in its schema (`confidence: { enum: [low, medium, high] }`) and decides which one a letter
falls in. The judgement stays with the agent; the flow only routes on a value the agent declared.

When the step it reads is an agent with an `output.schema`, the filter is checked at load time:

```
line 22: step review_doubtful: where.confidence: lw is not one of low, medium, high
```

A misspelt field, a value outside the enum or a string where the schema has a number would
otherwise filter out every record, and the flow would finish having done nothing. Records from a
script or from `--in` have no declared shape, so their filters are checked at run time only, and a
payload that is not an object is an error naming the record.

How a filter behaves on a rerun:

- A record left out is **not pending**: it is not run, not retried, not counted as a failure. The
  log says how many: `review_doubtful: 3 to run, 0 already done, 12 not matching where`.
- A record that matched before and no longer does has its old result removed from the step's
  directory, so the directory keeps meaning "this step's records".
- A whole-step run (`from_all`) behind a filter reruns when *any* upstream record changes, kept or
  not: a record that stopped matching changes what the step must produce.

`where` works with `for_each` and with a single-source `from_all`. It is refused on a fan-in,
because each source has its own fields: filter each branch in its own step.

## Joins: `from_all` with a list

```yaml
  - step: build_timeline
    run: python merge.py
    from_all: [extract_facts, review_doubtful, fetch_people]
```

The step receives the records of every listed step, in the order the sources are written, and each
record carries one more field, `"from"`, naming the step it came from. The same `id` can exist in
two sources — document `a` among the letters, person `a` in the register — and `from` is what
tells them apart, so records are unique per `(from, id)` rather than per `id`.

In the example that is what lets `merge.py` prefer the second reading from `review_doubtful` over
the first one from `extract_facts` for the same letter.

- Only `from_all` takes a list. `for_each` over two sources would be ambiguous, and is refused.
- Written as a list, records carry `from` even with one source, so adding a second source later
  does not change what the script reads.
- `from` is set by mani on a join and replaces a field of that name, which the log reports. Pick
  another name for a field of your own — `sender`, not `from`.
- The step is fresh only while it is newer than every source: a change in any of them reruns it.

## Order, and the absence of cycles

Steps run in the order written, one at a time, and a step may only read a step written **above**
it. The file therefore reads top to bottom, and a cycle cannot be expressed. Branching out is
fine, and branches join again through a [`from_all` list](#joins-from_all-with-a-list).

```
fetch_letters ──► extract_facts ──► review_doubtful ─┐
      │                  └──────────────────────────┼──► build_timeline
fetch_people ───────────────────────────────────────┘
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
├── .params.json              # the parameters this directory was built with
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
| `--set name=value` | a parameter; repeatable, and also accepted by `mani validate` |
| `--verbose` | logs to stderr; progress is always on stderr, stdout is the result |

`--task`, `--image`, `--provenance` and `--insecure` belong to an agent and are refused on a flow.

## Budget

`limits.tokens` is checked before a run starts, so runs already in flight finish and the total can
pass the cap by at most `jobs` runs. When it is spent the flow stops with an error; raising the cap
and running again continues from the records on disk. Totals print on stderr either way.

## What a flow is not

No shared mutable state, no cycles, no expressions, no scheduler. A `where` is an equality filter
on a value an agent declared, not a condition language. Where the next step depends on judgement,
use an agent or `delegate`; where the shape is known in advance, use a flow. Running independent
branches at the same time is deliberately deferred.

Next: [built-in tools](../tools/).
