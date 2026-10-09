---
title: The manifest
description: Why an agent file has eight blocks, what question each one answers, and how to tell where a new setting belongs.
weight: 1
---

A manifest is one YAML file that holds a whole agent. It has eight top-level blocks, and the
design rule is that **each block answers exactly one question**.

| Block | Question |
|---|---|
| `identity` | who thinks? |
| `capabilities` | what can it do? |
| `context` | what does it see and remember? |
| `output` | what does it return? |
| `policy` | what is it allowed to do? |
| `limits` | how much may it consume? |
| `run` | when does it start, and how? |
| `observability` | what does it leave behind? |

Only `identity` is required. A three-line manifest is a valid agent.

## Why one question per block

The eight questions are a lookup table for a question you will ask constantly: *where does this
setting go?* Without it, a growing manifest format turns into a flat bag of keys, and nobody can
guess whether a timeout belongs next to the tool, next to the model, or next to the scheduler.

The test is what the setting *does*:

- Can it stop an action from happening? That is `policy`.
- Does it constrain the answer? That is `output`.
- Does it change what the model sees? That is `context`.
- Does it cap consumption? That is `limits`.
- Does it decide when a run starts? That is `run`.

Three examples of the rule in action. A tool timeout is a budget, so it is `limits.tool_timeout`,
not a property of the tool. A pattern that blocks `rm -rf` can stop an action, so it is a
`policy.rules` entry, not a flag on the bash tool. The prompt injection of `AGENTS.md` changes
what the model reads, so it is `context.inject`.

## An unknown key is an error

Loading a manifest fails on any key the runtime does not know, and the error carries the line
number:

```
[manifest]: decode agent.yaml: yaml: unmarshal errors:
  line 14: field tool not found in type app.PolicySpec
```

The alternative, ignoring unknown keys, means a typo silently disables a rule. `policy.tool`
instead of `policy.tools` would read as "no policy at all", and the agent would run with
permissions nobody intended. That failure is invisible until it matters, which is the worst kind.

The same reasoning applies to `${VAR}` references: an undefined variable is an error, not an
empty string. A blank bearer token would mean "authentication disabled" to the webhook listener.

## The manifest is meant to be committed

Everything in the file is safe to put in git. Credentials are not in it: they live in
`auth.json`, outside the repository, and a manifest refers to them indirectly through environment
variables.

That is also why `!include` exists. A real system prompt is prose, often a hundred lines, and
YAML is a poor place to keep prose:

```yaml
identity:
  prompt: !include ./prompts/maintainer.md
```

Paths resolve against the manifest, so the pair moves together and runs from any working
directory.

## Validation is a separate step

`mani validate` resolves everything the runtime resolves — includes, variables, tool names,
policy rules, journal settings, and for a flow every manifest it names — and calls no model. It
belongs in CI, where it catches a missing prompt file or an unset variable before a scheduled run
does, at three in the morning.

Next: [the agent loop](../agent-loop/), which is what runs once the manifest has loaded.
