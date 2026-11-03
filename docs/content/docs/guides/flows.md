---
title: Build a pipeline
description: Chain manifests and scripts into a flow that runs with one command and resumes like make.
weight: 4
---

Goal: several stages — fetch, classify, aggregate, write up — running as one command, where
rerunning it after a change costs only the work that actually changed.

Prerequisites: manifests that each work on their own, and a script or two for the parts that are
not model work.

## The worked example

`_examples/flow/` in the repository is a complete pipeline, and it uses every part of the flow
grammar: customer reviews in, a product catalog fetched by a script, a label per review, a draft
reply to every unhappy customer, a tally that joins all three, and a note for a product team.

{{< repofile path="flow/reviews.flow.yaml" lang="yaml" >}}

| Step | What it shows |
|---|---|
| `catalog` | a first step: a `run:` that reads nothing and fetches |
| `classify` | `for_each: input`, one agent run per line of `--in`, `jobs: 3` at a time |
| `reply` | `where: { sentiment: negative }`, checked against `classify.yaml`'s enum |
| `tally` | a join with `from_all: [classify, reply, catalog]`, and `${params.team}` in a command |
| `note` | an agent over a script's output: the tally object arrives as its task, in JSON |

Around them: `input:` makes `--in` required, `params:` declares one required parameter and one with
a default, `result:` picks what goes to stdout, and `limits.tokens` caps the whole pipeline.

```bash
mani validate --config reviews.flow.yaml --set team="Small appliances"
mani run --config reviews.flow.yaml --in reviews.jsonl --out /tmp/reviews --set team="Small appliances"
```

`validate` reads the flow back to you, which is the fastest way to catch a wrong wire:

```
reviews.flow.yaml: ok
  reviews — Classifies customer reviews, drafts replies to the unhappy ones, and writes a note for a product team
  input: One review per line: {id, product, task} (--in)
  params:
    team           Small appliances --set     The product team the note is addressed to
    top            2                default   How many products the note ranks
  1. catalog            run python3 catalog.py
                        Reads the product catalog, one record per product
  2. classify           agent classify.yaml, for each record of input (3 at a time)
                        Reads one review and labels its sentiment and problem
  3. reply              agent reply.yaml, for each record of classify where sentiment is negative
                        Drafts a short reply to each unhappy customer
  4. tally              run python3 tally.py --team "Small appliances" --top 2, on all the records of classify, reply and catalog
                        Counts the labels, collects complaints and drafted replies, and ranks the products
  5. note               agent summarize.yaml, for each record of tally
                        Writes a short note for the team from the tally
  result: note
  limits: 80000 tokens for the whole flow
```

The first run, with five reviews:

```
flow: catalog: running python3 catalog.py
catalog: 2 products
flow: classify: 5 to run, 0 already done
flow: reply: 2 to run, 0 already done, 3 not matching where
flow: tally: running python3 tally.py --team "Small appliances" --top 2
tally: 5 reviews, 2 negative, 2 replies
flow: note: 1 to run, 0 already done
flow: tokens: 320 in, 80 out
```

Eight model calls: five classifications, two replies — the three positive and neutral reviews do
not get one — and the note. The `catalog:` and `tally:` lines are the scripts' own stderr, passed
through.

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
belongs: a flow has no template language on purpose. The tally step of the example, in full — it
tells its three sources apart by the `from` field the join adds:

{{< repofile path="flow/tally.py" lang="python" >}}

## Rerunning is the point

With `--out`, every step keeps its records, and a record newer than what it was made from is not
made again.

```bash
mani run --config reviews.flow.yaml --in reviews.jsonl --out /tmp/reviews --set team="Small appliances"
```

```
flow: catalog: already done
flow: classify: 0 to run, 5 already done
flow: reply: 0 to run, 2 already done, 3 not matching where
flow: tally: already done
flow: note: 0 to run, 1 already done
flow: tokens: 0 in, 0 out
```

Add a review to the input and rerun: one classification, a reply only if it is negative, then the
tally and the note again, because their input changed. Delete one result and rerun: that review
alone is redone, and everything after it follows only if its output came out different. A step
that reruns and produces identical records does not move their timestamps, so nothing downstream
reruns.

The directory also remembers the parameters it was built with. The same command with
`--set team=Kitchen` is refused rather than handing the kitchen team a note built for small
appliances:

```
mani run: /tmp/reviews was built with team=Small appliances, and this run has team=Kitchen; use
another --out, or remove the directory to rebuild it
```

To make the first step fetch again, delete its `.done` marker, not its directory:

```bash
rm /tmp/reviews/catalog/.done      # the step reruns; identical records stay put
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
mani run --config reviews.flow.yaml --in reviews.jsonl --out /tmp/reviews --set team="Small appliances" --limit 1
```

`--limit` caps the new agent runs per step and lets the rest of the flow proceed, so you see the
whole chain working on one record before paying for the full list. Rerun without the flag and
only the missing records are computed.

## Parameters, filters and joins

Three additions turn the example into the shape most real pipelines have. All three are in the
[flow file reference](../../reference/flow/); this is the short version.

**Parameters**, for the values that change from one run to the next:

```yaml
params:
  busta: "The busta to rebuild"
  since: { about: "Earliest year to keep", default: "1490" }

steps:
  - step: fetch_letters
    does: "Downloads the transcribed letters of the busta"
    run: python fetch.py --busta ${params.busta} --since ${params.since}
```

```bash
mani run --config letters.flow.yaml --set busta=12 --out runs/12/
```

A missing required parameter stops the run before anything starts. The state directory remembers
the values it was built with, so pointing busta 13 at `runs/12/` is refused instead of quietly
reusing busta 12's records — give each value its own `--out`.

**A filter**, to send only some records down a step:

```yaml
  - step: review_doubtful
    does: "Gives a second reading to the letters the extractor was unsure about"
    agent: review.yaml
    for_each: extract_facts
    where: { confidence: low }
```

The extractor's schema declares `confidence: { enum: [low, medium, high] }`, so the agent decides
which letters are doubtful and the flow only routes them. `mani validate` checks the filter against
that schema, so `where: { confidence: lw }` fails at load time instead of filtering out every
letter.

**A join**, to bring branches back together:

```yaml
  - step: build_timeline
    does: "Resolves people and places and orders the letters in time"
    run: python merge.py
    from_all: [extract_facts, review_doubtful, fetch_people]
```

Every record arrives with a `from` field naming its step, which is how `merge.py` tells the second
reading of a letter from the first, and a person from a letter with the same id.

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
