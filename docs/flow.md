# Flows

A flow wires manifests into a pipeline. It is a second kind of file, run by the same command:

```bash
mani validate --config letters.flow.yaml        # the flow read aloud
mani run --config letters.flow.yaml --out runs/ # execute it
```

`mani run` and `mani validate` tell a flow from an agent by what the file **says**, not by its
name: a file whose first key is `flow:` is a flow. An extension would be a convention that a
rename breaks in silence.

A flow adds no behaviour of its own. Every step is an agent manifest or a command that already
runs by itself, which is the point: each piece stays testable alone, and the flow only says who
reads whose output.

## The whole grammar

```yaml
flow: idea_letters                 # a name: lowercase letters, digits, _
about: "Rebuilds the timeline of a busta from its transcribed letters"
input: "One letter per line"       # optional: the records given with --in

steps:
  - step: fetch_letters
    does: "Downloads the transcribed letters of the busta"
    run: python fetch.py --busta ${BUSTA}

  - step: extract_facts
    does: "Reads one letter and extracts sender, recipient, place and date"
    agent: extract.yaml
    for_each: fetch_letters        # one run per record
    jobs: 4                        # …four at a time

  - step: build_timeline
    does: "Resolves people and places and orders the letters in time"
    run: python merge.py
    from_all: extract_facts        # one run, over every record

  - step: write_synthesis
    does: "Writes the narrative of the busta"
    agent: synthesize.yaml
    for_each: build_timeline

result: write_synthesis            # whose records go to stdout
limits: { tokens: 2000000 }        # optional: a ceiling for the whole flow
```

| Key | Meaning |
|---|---|
| `flow` | the name; it names the state directories and prefixes every log line |
| `about` | one sentence, **required**: what the pipeline is for |
| `input` | one sentence describing what `--in` must contain; present ⇒ `--in` is required |
| `steps[].step` | the step's name; unique, `snake_case`, and `input` is reserved |
| `steps[].does` | one sentence, **required**: what this step does |
| `steps[].agent` | a manifest, relative to the flow file |
| `steps[].run` | a command; exactly one of `agent` or `run` |
| `steps[].for_each` | run once per record of that step |
| `steps[].from_all` | run once, over every record of that step |
| `steps[].jobs` | how many runs at a time; only with `agent` + `for_each` (default 1) |
| `result` | the step whose records are printed on stdout |
| `limits.tokens` | a cap for every run of every step together; 0 or absent = none |

Unknown keys are a hard error with their line number, exactly as in a manifest, and `${VAR}`
expands from the environment in the same way — an unset variable fails the load rather than
becoming an empty string.

## Order, and why there is no cycle

The steps run **in the order written**, one at a time, and a step may only read a step written
**above** it. So the file reads top to bottom like a recipe, the order on the page is an order
that works, and a cycle cannot be written at all — there is nothing to detect.

```
fetch_letters ──► extract_facts ──► build_timeline ──► write_synthesis
   (code)           (agent × N)        (code)             (agent)
```

A step reads at most one other step. Branching out is fine — two steps can read the same step —
but joining two branches back together is not supported yet.

## Records: what travels between steps

The edge is a record: a JSON object with an `id`, a payload, and anything else you attach.

```json
{"id": "doc_1904", "task": "…the letter text…", "shelfmark": "b.12 f.3"}
```

- `id` is required, must be a string, unique within the step, and usable as a file name — it
  becomes one.
- The extra fields **travel untouched** from step to step. That is the lineage: the shelfmark that
  went in comes back out next to the synthesis.
- An agent step writes `{"id": …, "result": …, "run": …}` plus the extras it received. `run` is
  the provenance envelope: run id, source, provider, model, manifest, timestamps and tokens.

What an agent receives as its task:

| The step it reads | The task is |
|---|---|
| `--in`, or a `run:` step | the record's `task` |
| an `agent:` step | that agent's `result` |

If the payload is not a string it is passed as JSON, so one agent's structured answer becomes the
next one's task with no glue in between. There is deliberately **no template language**: when data
needs reshaping, that is what a `run:` step is for.

## `run:` steps

A `run:` step is argv, with **no shell** — no quoting, no pipes, no globs — so the same flow works
on Linux, macOS and Windows:

```yaml
run: python merge.py --strict        # split on spaces
run: [python, "my script.py"]        # a list, when an argument contains spaces
```

- The working directory is the flow's directory, so `run: ./merge.py` and relative data paths
  behave the same from anywhere.
- With `from_all` (or no input at all) the command runs once; with `for_each` it runs once per
  record, in order.
- Records arrive on **stdin** as JSONL, one object per line; the command prints its own records on
  **stdout** as JSONL. Each printed record needs an `id`.
- Its **stderr passes through** to yours, so a script's own progress is visible, and mani's stdout
  stays a format.
- A non-zero exit stops the flow, with the command line and the step in the error.

A first step typically has no input at all: it is the one that fetches.

## State on disk, and resuming

With `--out` every step keeps its records:

```
runs/
├── fetch_letters/
│   ├── doc_1904.json
│   └── .done                 # written when a whole-step run finished
├── extract_facts/
│   ├── doc_1904.json
│   └── errors.jsonl          # one line per failed attempt, appended, never truncated
└── build_timeline/
```

Resuming is **make's rule**: a record that exists and is **newer** than the record it was made
from is not made again. In practice:

- rerunning a finished flow calls no model at all;
- one more input costs one more run, plus the steps that depend on it;
- a step that runs again and produces **identical** records does not move their timestamps, so
  nothing downstream runs again either.

To force a code step to run again, delete its `.done` — not its directory. The step reruns, the
identical records are left alone, and only what really changed is recomputed downstream:

```bash
rm runs/fetch_letters/.done
mani run --config letters.flow.yaml --out runs/
```

Deleting the whole directory also works, but it throws away the timestamps and therefore redoes
everything that depended on it.

Without `--out` the flow still runs, in a temporary directory that is removed at the end: nothing
is kept, and nothing can be resumed.

## When something fails

- A failed record writes **no result file** — that is what makes the retry possible — and appends
  the reason to `errors.jsonl`.
- The flow then **stops before the next step**, so no step ever computes on half of its input. The
  exit code is 1.
- The next run retries only the records that failed.
- `Ctrl-C` cancels the runs in flight and starts none of the pending ones. Nothing is written for
  a cancelled run, and it is not counted as a failure: the next run picks it up.

## Flags

| Flag | Meaning |
|---|---|
| `--out DIR` | keep every step's records here; without it, nothing is kept |
| `--in FILE` | the records for `input:`; `-` reads stdin |
| `--limit N` | at most N **new** agent runs per step; the flow proceeds on what there is |
| `--verbose` | logs to stderr (progress is always on stderr; stdout is the result) |

`--limit` is how you try a pipeline on a sample: run it with `--limit 1`, look at the whole chain
end to end, then run it again without the flag — only the missing records are computed.

## Budget

`limits.tokens` caps every run of every step together. It is checked **before** a run starts, so
runs already in flight finish: the total can pass the cap by at most `jobs` runs. When it is spent
the flow stops with an error, and because the records are on disk, raising the cap and running
again continues where it left off.

The totals are printed on stderr at the end either way, so the way to pick a cap is to run once
without one and read the number.

## `mani validate` on a flow

```
$ mani validate --config letters.flow.yaml
letters.flow.yaml: ok
  idea_letters — Rebuilds the timeline of a busta from its transcribed letters
  1. fetch_letters      run python fetch.py --busta 12
                        Downloads the transcribed letters of the busta
  2. extract_facts      agent extract.yaml, for each record of fetch_letters (4 at a time)
                        Reads one letter and extracts sender, recipient, place and date
  3. build_timeline     run python merge.py, on all the records of extract_facts
                        Resolves people and places and orders the letters in time
  4. write_synthesis    agent synthesize.yaml, for each record of build_timeline
                        Writes the narrative of the busta
  result: write_synthesis
  limits: 2000000 tokens for the whole flow
```

Validating a flow also loads **every manifest it names**, so a flow that validates has agents
that validate too. That is why `about:` and `does:` are required: this output is the flow read
aloud, and it is the first test of whether the file is legible.

## `mani batch`: a flow of one step

One agent over a file of tasks is the same machinery with a single step, so it has its own
command — the common case should not need a file:

```bash
mani batch --config classify.yaml --in reviews.jsonl --out out/ --jobs 4
mani batch --config classify.yaml --in - < reviews.jsonl | jq -c '.result'
```

Same rules: resume by skipping what is already there, `errors.jsonl` for failures, exit 1 if any
task failed, extras carried through. With `--out` each result is a file; without it the records
stream to stdout as they finish, so a batch fits in a pipe. The whole input is checked before the
first model call: a malformed line, a missing or repeated `id`, or an `id` that cannot be a file
name fails immediately rather than two thousand runs later.

## What a flow is not

- **Not LangGraph**: no shared mutable state, no cycles, no conditional edges. Where judgement is
  needed, use an agent — a prompt, or `delegate` with subagents. Where the shape is known in
  advance, use a flow.
- **Not a queue**: it does not survive as a process. It *resumes*, which is cheaper and easier to
  reason about.
- **Not a template language**: data is reshaped by a `run:` step, in a real language.
- **Not a scheduler**: when things start is still `run.triggers` in a manifest.

Deliberately deferred, with the reasoning kept in the backlog: conditional edges (`when:`),
declared parameters (`params:`), joining two branches, and running independent branches in
parallel.

## Example

[`_examples/flow/`](../_examples/flow/) is a complete, runnable pipeline: reviews in, a labelled
record per review, a tally computed by a Python step, and a note for the team written by a second
agent. The header of `reviews.flow.yaml` lists the commands to try, including the ones that show
the resume rules.
