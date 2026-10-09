---
title: Get typed JSON back
description: Declare an output schema so a run returns a validated JSON object, and use it from a shell pipeline or a script.
weight: 1
---

Goal: a run that prints a JSON object matching a shape you declared, so a script can consume it
without parsing prose.

Prerequisites: a working manifest ([your first agent](../../getting-started/first-agent/)).

## Declare the shape

```yaml
output:
  schema:
    type: object
    properties:
      sentiment: { type: string, enum: [positive, negative, neutral] }
      problem:   { type: string }
      products:
        type: array
        items: { type: string }
    required: [sentiment, problem]
```

The agent now gets one extra tool, `respond`, and the run ends when it calls it. mani validates
the payload before returning it, and an answer that does not fit goes back to the model with the
reason, so stdout always matches the declaration.

```bash
mani run --config classify.yaml --task "$(cat review.txt)" | jq -r .sentiment
```

```
negative
```

## Keep the sets closed

A schema is worth having when it narrows the answer. `enum: [positive, negative, neutral]` gives
you three values to branch on. A bare `{ type: string }` gives you something to parse, which is
what you were trying to avoid.

For a list too long to write out, load it from a file:

```yaml
person: { type: string, enum: !include ./people.txt }
```

One value per line, with blank lines and `#` comments skipped; `.json`, `.yaml` and `.yml` files
are read as lists. Above 200 values `mani validate` warns, because the schema travels with every
request. At that size, a lookup tool over the same list usually costs less: see
[custom tools](../custom-tools/).

## Validation goes all the way down

Nested shapes are checked, not just the top level:

```yaml
output:
  schema:
    type: object
    properties:
      letters:
        type: array
        items:
          type: object
          properties:
            place: { type: string, enum: [Mantova, Ferrara] }
            year:  { type: integer }
          required: [place]
    required: [letters]
```

An answer with `letters[1]` missing its `place`, or carrying `1495.5` as a year, is refused and
retried. The error names the element:

```
letters[1].place must be one of [Mantova Ferrara]
```

A deep schema can therefore cost one extra model call when the first answer misses. That is the
trade: you pay a retry instead of discovering the bad field downstream.

## Use it from a script

Structured output is what makes an agent usable as a component. Three shapes that come up:

```bash
# branch on a field
severity=$(mani run --config triage.yaml --task "$(cat report.txt)" | jq -r .severity)
[ "$severity" = high ] && notify-oncall

# keep the provenance with the answer
mani run --config triage.yaml --task "$(cat report.txt)" --provenance > triage.json

# the whole inbox, resumable, four at a time
mani batch --config triage.yaml --in reports.jsonl --out out/ --jobs 4
```

{{< callout type="info" >}}
  Without a schema the run prints the model's text, and `--provenance` wraps that text as
  `{"response": "..."}`. Declaring a schema is what turns a run into a typed function.
{{< /callout >}}

## What you should see

- `mani validate` reports `output: structured`.
- A run prints a JSON object and nothing else on stdout; progress and logs are on stderr.
- A malformed answer never reaches you: it is retried, or the run fails.

Next: [process a file of tasks](../batch/).
