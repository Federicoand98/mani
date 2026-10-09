---
title: Build a pipeline
description: Chain manifests and scripts into a flow that runs with one command and resumes like make.
weight: 4
---

Goal: several stages — fetch, classify, aggregate, write up — running as one command, where
rerunning it after a change costs only the work that actually changed.

Prerequisites: two manifests that each work on their own, and a script or two for the parts that
are not model work.

## The worked example

`_examples/flow/` in the repository is a complete pipeline: customer reviews in, a labelled
record per review, a tally computed in Python, and a note for the team written by a second agent.

{{< repofile path="flow/reviews.flow.yaml" lang="yaml" >}}

```bash
mani validate --config reviews.flow.yaml
mani run --config reviews.flow.yaml --in reviews.jsonl --out /tmp/reviews
```

`validate` reads the flow back to you, which is the fastest way to catch a wrong wire:

```
reviews.flow.yaml: ok
  reviews — Classifies customer reviews and writes a note for the product team
  input: One review per line: {id, product, task} (--in)
  1. classify           agent classify.yaml, for each record of input (3 at a time)
                        Reads one review and labels its sentiment and problem
  2. tally              run python3 tally.py, on all the records of classify
                        Counts the labels and collects the complaints by product
  3. note               agent summarize.yaml, for each record of tally
                        Writes a short note for the team from the tally
  result: note
  limits: 50000 tokens for the whole flow
```

## How the pieces connect

**`for_each`** runs the step once per record of the step it names, `jobs` at a time.
**`from_all`** runs it once, over every record. A step may only read a step written above it, so
the file reads top to bottom and a cycle cannot be expressed.

**What travels between steps is a record**: `{"id": …, "task": …}` plus any fields you attach,
which ride along untouched. An agent receives the upstream `task`, or the `result` of the agent
before it, serialised as JSON when it is not a string. One agent's structured answer is therefore
the next one's task with nothing in between.

**A `run:` step is argv with no shell.** Records arrive as JSONL on stdin, the script prints its
own records as JSONL on stdout, and its stderr passes through to yours. That is where reshaping
belongs: a flow has no template language on purpose. The tally step of the example, in full:

{{< repofile path="flow/tally.py" lang="python" >}}

## Rerunning is the point

With `--out`, every step keeps its records, and a record newer than what it was made from is not
made again.

```bash
mani run --config reviews.flow.yaml --in reviews.jsonl --out /tmp/reviews
```

```
flow: classify: 0 to run, 5 already done
flow: tally: already done
flow: note: 0 to run, 1 already done
flow: tokens: 0 in, 0 out
```

Add a review to the input and rerun: one classification, then the tally and the note again,
because their input changed. Delete one result and rerun: that review alone is redone, and the
note follows only if the tally came out different. A step that reruns and produces identical
records does not move their timestamps, so nothing downstream reruns.

To make a first step fetch again, delete its `.done` marker, not its directory:

```bash
rm /tmp/reviews/classify/.done     # the step reruns; identical records stay put
```

Deleting the whole directory also works, but it throws away the timestamps and redoes everything
that depended on it.

## Failures stop the flow

A failed record writes no result and lands in `<out>/<step>/errors.jsonl`, and the flow stops
before the next step rather than computing on half its input. The exit code is `1`. Rerunning
retries only what failed.

A `run:` step that exits non-zero stops the flow too, with its own stderr in the error.

## Try a sample end to end

```bash
mani run --config reviews.flow.yaml --in reviews.jsonl --out /tmp/reviews --limit 1
```

`--limit` caps the new agent runs per step and lets the rest of the flow proceed, so you see the
whole chain working on one record before paying for the full list. Rerun without the flag and
only the missing records are computed.

## Keep a ceiling on it

```yaml
limits: { tokens: 50000 }
```

Checked before each run starts, across every step. Runs already in flight finish, so the total
can pass the cap by at most `jobs` runs. The totals print at the end either way, which is how to
pick a number: run once without a cap and read it.

## What you should see

- `mani validate` prints the flow with its steps, its wiring and its budget.
- `<out>/<step>/<id>.json` for every record of every step, plus a `.done` marker per whole-step
  run.
- stdout carrying only the records of the `result:` step, as JSONL.
- A rerun that makes no model calls.

Next: [add a tool in any language](../custom-tools/), or the [flow file reference](../../reference/flow/).
