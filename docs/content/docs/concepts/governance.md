---
title: Governance
description: How permissions, risk levels, pattern rules, redaction and budgets work in mani, and why they live in the runtime rather than in the caller.
weight: 3
---

An agent that can edit files and run shell commands is a program that decides at run time what to
execute. Governance is the part of mani that decides what it may actually do.

Four mechanisms, each answering a different question.

## Permissions: allow, ask, deny

Every tool gets one of three verdicts:

```yaml
policy:
  tools:
    read: allow
    write: ask
    bash: deny
    default: allow
```

`ask` suspends the run until somebody decides. In the terminal chat you see a prompt; over the
WebSocket, the client receives a `permission_request` and answers it. Where no human is
reachable — `mani run --task`, a trigger, a batch, an MCP call — the request is **denied
automatically**.

That default is deliberate. A run that waits forever for an answer nobody is there to give is
worse than one that stops with a reason in the journal. The practical consequence: a manifest
meant to run unattended should be written with `allow` and `deny`, and `ask` is for the manifests
you sit in front of.

## Risk levels: what a tool can do to the world

Every tool declares a level: `none`, `network`, `write`, `execute`. Only `none` runs in parallel
and ungated.

Reaching the network is its own level rather than part of `none`, because a read-only GET can
carry out whatever the agent has just read. A tool marked `none` that happens to make an HTTP
request would be an exfiltration path that the policy never sees.

Network tools are confined further by `policy.network`, which is an allow and deny list of hosts.
The check also refuses private, loopback and link-local addresses at dial time, so an allowed
hostname that resolves to `127.0.0.1` cannot reach the agent's own server.

## Rules: what the arguments say

Permissions are per tool. Rules look at the arguments:

```yaml
policy:
  rules:
    - { tool: bash, pattern: 'rm\s+-rf', action: deny, label: "recursive delete" }
    - { tool: read, pattern: '\.env$',   action: deny, label: "secrets" }
```

A rule matches before the call happens, so a denied pattern never runs. The `label` is what the
journal records, which turns a blocked call into something you can count: *the nightly agent
tried the recursive delete four times this week* is a sentence you can only write if the label is
there.

`policy.redact` is the mirror image. It rewrites tool output before the model sees it, which is
how an API key in a log file stops at the boundary instead of travelling into the conversation
and then into the journal.

## Limits: how much it may consume

```yaml
limits:
  max_tokens: 50000
  max_tool_calls: 20
  max_duration: 2m
  max_iterations: 15
  tool_timeout: 15s
  subagent_depth: 5
```

These are per run. When one is reached the run stops and the journal records which one. They are
the difference between a loop that costs a few cents and a loop that runs until morning.

A flow adds one more, `limits.tokens`, which caps a whole pipeline across every run of every step.

## Why this lives in the runtime

Governance is composed entirely out of the hooks described in [the agent loop](../agent-loop/).
The domain package, `core/`, has no idea that policy exists: the runtime reads your manifest and
registers hooks. That has two consequences you can rely on.

**The rules do not depend on the caller.** The same manifest run from the terminal, from a
webhook, from `mani serve` or from an editor over MCP is governed identically. A transport cannot
opt out of a policy it does not know about.

**What governance decided is recorded.** A denied call, a blocked pattern, a masked value and an
exhausted budget are all events in the run record. An unattended agent is reviewable because the
decisions left a trace, not because you were watching.

Next: [runs and the journal](../runs/).
