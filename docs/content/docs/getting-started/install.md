---
title: Install
description: Install the mani binary, connect a model provider, and learn where configuration and credentials are stored.
weight: 1
---

mani is a single binary with no runtime dependencies. You need one more thing: a model it can
talk to, either a local one or an API.

## Get the binary

{{< tabs >}}

  {{< tab name="Prebuilt" >}}
Download the archive for your platform from the
[releases page](https://github.com/Federicoand98/mani/releases/latest) and put `mani` on your
`PATH`. Builds are published for Linux, macOS and Windows, on amd64 and arm64 (Windows on amd64
only).

```bash
tar xzf mani_*_linux_amd64.tar.gz
sudo install -m 755 mani /usr/local/bin/mani
```
  {{< /tab >}}

  {{< tab name="go install" >}}
Needs Go 1.25 or newer.

```bash
go install github.com/Federicoand98/mani/cmd/mani@latest
```

The binary lands in `$(go env GOPATH)/bin`.
  {{< /tab >}}

  {{< tab name="From source" >}}
```bash
git clone https://github.com/Federicoand98/mani.git
cd mani
make build          # writes build/mani
```
  {{< /tab >}}

{{< /tabs >}}

Check that it runs:

```bash
mani --version
```

## Connect a model

mani talks to five providers: `ollama`, `openai`, `anthropic`, `copilot` and `openrouter`. Out of
the box it is configured for Ollama on `http://localhost:11434` with the model `qwen3.5:9b`, which
means a local install needs no configuration at all.

{{< tabs >}}

  {{< tab name="Local (Ollama)" >}}
Install [Ollama](https://ollama.com), pull a model that supports tool calling, and you are done:

```bash
ollama pull qwen3.5:9b
mani --version          # nothing else to configure
```
  {{< /tab >}}

  {{< tab name="Hosted API" >}}
Start the interactive chat and log in to the provider you want. The key is written to
`auth.json`, never to a manifest and never to `config.json`:

```bash
mani
/login anthropic
/provider anthropic
/model claude-sonnet-5
```

`/logout anthropic` removes the key again.
  {{< /tab >}}

{{< /tabs >}}

{{< callout type="warning" >}}
  A manifest that names a provider mani cannot use will fail the run. It never falls back to
  another model, because cost and privacy would then differ from what you declared, and you would
  find out afterwards.
{{< /callout >}}

## Where things live

| What | Path |
|---|---|
| Settings | `$XDG_CONFIG_HOME/mani/config.json`, or `~/.config/mani/config.json` |
| Credentials | `$XDG_DATA_HOME/mani/auth.json`, or `~/.local/share/mani/auth.json`, mode `0600` |
| Log file | `~/.config/mani/mani.log` |

`config.json` holds the default provider, the per-provider base URL and model, the context window
and the log level. You rarely edit it by hand: `/provider`, `/model` and `/config` in the chat
write it for you.

Credentials are kept in a separate file for a reason. A manifest is something you commit, share
and run on someone else's machine, so secrets never go in it. Where a manifest needs one, it
refers to an environment variable:

```yaml
run:
  triggers:
    - { type: webhook, token: ${DEPLOY_TOKEN} }
```

## Environment overrides

Useful in CI or a container, where editing `config.json` is awkward:

| Variable | Overrides |
|---|---|
| `MANI_PROVIDER` | the active provider |
| `MANI_MODEL` | the model for that provider |
| `MANI_CONTEXT_WINDOW` | the context window, in tokens |
| `MANI_MAX_ITERATIONS` | how many model calls one run may make |
| `MANI_LOG_LEVEL` | `error`, `warn`, `info` or `debug` |

Next: [your first agent](../first-agent/).
