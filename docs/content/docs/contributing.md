---
title: Contributing
description: How to set up mani for development, what the project accepts, the invariants a change must respect, and how to send it.
weight: 7
---

mani is a one-person learning project, maintained alongside other work. Contributions are welcome,
and the bar is less "does it work" than "does it belong, and can you explain why it looks like
this".

[`CONTRIBUTING.md`](https://github.com/Federicoand98/mani/blob/master/CONTRIBUTING.md) in the
repository is the full version. This page is the short one.

## Set up

```bash
git clone https://github.com/Federicoand98/mani.git
cd mani
go build ./...
go test ./...
make build          # writes build/mani
```

Needs Go 1.25 or newer, and a local [Ollama](https://ollama.com) if you want to run an agent
rather than only the tests. The tests are hermetic: no network, no clock dependency, no reliance
on your timezone.

```bash
go test -race ./...                                   # what CI runs
go test ./app -run TestLoadFlow -v                    # one package, one test
go run ./cmd/mani validate --config _examples/manifest.yaml
```

## Open an issue first

For anything beyond a typo. A patch that arrives without a conversation has to be judged against a
goal nobody agreed on, which is how good code gets rejected.

## The scope filter

A feature belongs in mani if it deepens one of three things:

| Axis | Question |
|---|---|
| Manifest expressiveness | can more of the agent be *declared* instead of coded? |
| Safe autonomy | can an unattended agent do more without becoming dangerous? |
| Operability as a service | is it easier to run, observe and keep running? |

If it deepens none of them, it is probably out of scope. Retrieval pipelines, vector stores,
prompt-tuning helpers, evaluation harnesses and reflection loops are deliberately absent: they are
well served elsewhere, and each one would cost the legibility that is the point of the project.

## The invariants

Architectural rules, not preferences. A change that breaks one will be asked to change regardless
of how good the rest is.

1. **`core/` has zero external dependencies.** It is the domain; adapters live in `app/`, `tool/`,
   `llm/`, `server/`.

   ```bash
   go list -deps ./core/... | grep '\.' | grep -v '^github.com/Federicoand98/mani'   # must print nothing
   ```

2. **No cgo.** Releases cross-compile to five targets from one machine, so a dependency that needs
   cgo breaks the release.

   ```bash
   CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build ./cmd/mani
   ```

3. **Dependencies point inward.** `cmd → app → core`. A driven adapter never imports a driving one.

4. **An unknown manifest key is a hard error.** A new key means a new field *and* a case in
   `Validate`, so `mani validate` fails in CI rather than the agent misbehaving at three in the
   morning.

5. **`gofmt`, `go vet` and `go test ./...` are green** before the PR opens.

## Tests

New behaviour comes with tests. Two expectations that come from real bites:

- **Ports get contract tests, not per-adapter tests.** `Journal`, `TaskQueue` and `LLMClient` have
  several implementations; one table that runs against all of them is what keeps them from
  diverging.
- **Measure the hot paths.** The journal writes on every tool call; the loop runs on every turn. If
  your change touches one, put a number in the PR description.

## Branches and commits

- Branch from `development`, target `development`. `master` is the release branch: it receives a
  merge from `development` immediately before a tag.
- Conventional subjects: `feat:`, `fix:`, `docs:`, `test:`, `chore:`.
- One PR, one concern.

## AI-assisted contributions

Welcome, with two conditions. **Disclose it** in the PR description, one line is enough. And **be
able to answer for the design**: expect questions of the form "why this structure", "what happens
under concurrency", "what does the failure path do". A PR that cannot be discussed at that level
gets closed even when the tests pass.

Generated code tends to be strong on the requirements that were written down and blind to the ones
that were not. Spend your attention on the second kind.

## Documentation

This site lives in [`docs/`](https://github.com/Federicoand98/mani/tree/master/docs) and is built
with Hugo and the Hextra theme. Its
[README](https://github.com/Federicoand98/mani/blob/master/docs/README.md) covers running it
locally and adding a page. Documentation changes do not wait for a release: a merge to `master`
publishes the site.

## After you open a PR

CI runs build, vet, gofmt and tests on Linux, macOS and Windows. PRs from forks need a maintainer
to approve the workflow run, so the checks may sit idle before they start. Expect a first response
within a few days, and substantive review rather than a rubber stamp.
