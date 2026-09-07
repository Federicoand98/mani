# mani, for everyone

*No Go, no programming. You write a YAML file describing what the agent may do, and run one
command.*

## What is an "agent" here?

A model (Claude, GPT, a local Llama…) that can **use tools** — read a file, run a command, call
your own script — in a loop, until the task is done. mani is the thing that runs that loop
*under rules you write*.

## One block, one question

A manifest has **8 top-level blocks**, and each answers exactly **one** question about the
agent. That's the whole mental model:

```yaml
identity:                                 # WHO thinks?
  provider: anthropic
  model: claude-sonnet-5
  prompt: "You are a nightly maintenance agent."

capabilities:                             # WHAT can it do?
  tools: [read, bash]

policy:                                   # WHAT is it allowed to do?
  tools:
    bash: ask                             # allow | ask | deny
    read: allow
```

Save it as `agent.yaml` and run:

```bash
mani run --config agent.yaml --task "Summarize the errors in today's log"
```

The other five blocks — `context`, `output`, `limits`, `run`, `observability` — are covered in
the [manifest reference](manifest.md). Adding something new? Walk the list top to bottom and the
first match wins, so you always know where a setting belongs.

Not sure the file is right? `mani validate --config agent.yaml` checks it without running
anything and without needing credentials.

## Safety nets you get for free

You don't have to trust the model. You constrain it — `policy` works at three levels of
granularity, from the coarsest to the finest:

```yaml
policy:
  tools:                                  # 1. THE TOOL: may it be used at all?
    bash: allow
  rules:                                  # 2. THE CALL: inspect the input, block this one
    - { tool: bash, pattern: 'rm\s+-rf', action: deny, label: "recursive delete" }
    - { tool: bash, pattern: 'curl.*\|\s*(sh|bash)', action: deny, label: "pipe-to-shell" }
  redact:                                 # 3. THE OUTPUT: scrub secrets before the model sees them
    - { pattern: 'sk-[A-Za-z0-9]{20,}', with: "***REDACTED***" }
  network:                                # and where it may reach on the internet
    allow: ["api.github.com"]

limits:                                   # a runaway agent can't drain your account
  max_tokens: 50000
  max_tool_calls: 20
  max_duration: 2m
```

## Let it run by itself, at night

```yaml
run:
  triggers:
    - type: daily
      at: "02:00"
      prompt: "Check the logs and summarize anomalies."
```

```bash
mani run --config agent.yaml        # no --task → starts the trigger scheduler, stays running
```

If the machine reboots or the process is killed mid-work, the task is not lost: it is a file on
disk, and it resumes on the next start.

## Know what it did while you slept

Every run is recorded — which tools ran, tokens spent, what got blocked or masked:

```yaml
observability:
  journal:
    enabled: true
    backend: jsonl        # jsonl (default) or sqlite
    path: ./runs         # one file per run, plain JSON lines
    retention: 200
```

```bash
cat runs/*.jsonl         # human-readable audit trail
```

## Teach it a new skill without writing Go

Any executable that reads JSON on stdin and writes its result on stdout becomes a tool — Python,
Node, a shell script, anything:

```yaml
capabilities:
  tools:
    - name: fetch_stock
      description: "fetch stock data"
      command: ./tools/.venv/bin/python
      args: ["tools/stock.py"]
      schema:
        type: object
        properties:
          symbol: { type: string, description: "stock symbol to fetch" }
        required: ["symbol"]
```

## Get structured data back instead of prose

Declare the shape of the answer and the agent must return exactly that:

```yaml
output:
  schema:
    type: object
    properties:
      sentiment: { type: string, enum: [positive, negative, neutral] }
      score:     { type: number }
    required: [sentiment, score]
```

```bash
mani run --config sentiment.yaml --task "The delivery was late but support was great"
# {"sentiment": "neutral", "score": 0.5}
```

That makes the agent a **typed function** you can pipe into other programs.

## Long prompts live in their own file

A serious system prompt is a hundred lines, and YAML is a miserable place to write one. Keep it
as markdown and point at it:

```yaml
identity:
  prompt: !include ./prompts/reviewer.md
```

---

Next: the [manifest reference](manifest.md) for every key, or [usage](usage.md) for the other
ways to run the same file.
