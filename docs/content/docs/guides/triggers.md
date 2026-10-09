---
title: Run on a schedule or a webhook
description: Start an agent from a timer, a daily time or an HTTP call, with a queue that survives a restart.
weight: 2
---

Goal: an agent that starts by itself, on a timer or when something calls it, and that does not
lose work if the process dies.

Prerequisites: a manifest that runs headless, and a policy written with `allow` and `deny` rather
than `ask`. Nobody is there to answer a prompt.

## Declare the triggers

```yaml
run:
  triggers:
    - type: every
      every: 30m
      name: disk-watch
      prompt: "Report partitions above 85%."

    - type: daily
      at: "02:00"
      name: nightly
      catch_up: true
      prompt: "Summarize anomalies in today's logs."

    - type: webhook
      addr: 127.0.0.1:8787
      path: /deploy
      token: ${DEPLOY_TOKEN}
      prompt: "A deploy finished. Check the service and report: {{body}}"

  scheduler:
    path: ./queue
    concurrency: 1
    max_pending: 64
    retry: { max_attempts: 3, backoff: 30s }
```

Then start the daemon by leaving out `--task`:

```bash
mani run --config watchdog.yaml
```

```
[daemon]: starting  triggers=3
```

It stays in the foreground and runs until you stop it. The scheduler is in-process, so the same
binary and manifest work on Linux, macOS and Windows with no cron entry and no systemd unit.

## Timers

`every: 30m` fires on an interval. `daily: at "02:00"` fires once a day in local time, and
handles the two days a year when daylight saving makes "02:00" ambiguous or missing.

`catch_up: true` runs a daily trigger that was missed because the process was down. Without it, a
machine that was asleep at 02:00 skips the night.

Give each trigger a `name`. It becomes the run's source in the journal (`trigger:nightly`), which
is how you later tell which schedule produced which record.

## Memory

A trigger starts with fresh memory by default: every firing is independent. Add
`memory: persistent` and the trigger remembers its previous runs, which is what you want for
"tell me what changed since last time" and wrong for anything else.

## Webhooks

Every webhook trigger shares one listener on `addr`, and gets its own route from `path` (default
`/hook`). One port, several routes, because N open ports is N things to close on a firewall.

```bash
curl -XPOST -H "Authorization: Bearer $DEPLOY_TOKEN" \
     -d '{"service":"api","version":"1.4.2"}' \
     http://127.0.0.1:8787/deploy
```

```
accepted
```

The route accepts `POST` only, replies `202 Accepted` once the task is queued, and `503` when the
queue is full. Bodies are capped at 64 KB. `{{body}}` in the prompt is replaced by the request
body.

Each route carries its own `token`, so revoking one leaves the others working. A trigger without
a `token` falls back to `MANI_WEBHOOK_TOKEN`.

{{< callout type="warning" >}}
  The request body goes into the prompt. An unauthenticated endpoint is therefore prompt
  injection with extra steps, which is why a token is required. `--insecure` starts the listener
  without one, and exists for local development only.
{{< /callout >}}

## Make the queue durable

```yaml
run:
  scheduler:
    path: ./queue
```

With `path` set, queued tasks are written to disk before they run. Kill the process mid-task and
the next start picks it up, which is what makes an unattended agent survive a reboot. Without
`path`, the queue is in memory and dies with the process.

`concurrency` is how many queued tasks run at once, `max_pending` is how long the queue may get,
and `retry` controls what happens to a task that fails.

## Check what happened while you were away

```bash
mani runs --config watchdog.yaml --since 24h
mani runs --config watchdog.yaml --status error --since 24h
```

The journal is the point of the exercise. A nightly agent that nobody watches is only safe if
its decisions, including the calls policy refused, are readable the next morning. See
[runs and the journal](../../concepts/runs/).

## What you should see

- `mani validate` lists the triggers it found.
- The daemon logs each firing, and `mani runs` shows one record per firing with source
  `trigger:<name>`.
- `curl` against a webhook route answers `202 accepted`, and a wrong token answers `401`.
- Killing the daemon mid-task and restarting it resumes that task, with `scheduler.path` set.

Next: [run it as a service](../service/).
