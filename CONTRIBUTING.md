# Contributing to mani

mani is a small, deliberately legible project. Every piece exists for a reason, and the reason is
usually written down — in the code, in `CONTEXT.md`, or in the issue that produced it. That is the
bar a contribution is measured against: not just "does it work", but "does it belong, and can you
explain why it looks like this".

## Before you write code

**Open an issue first**, or comment on an existing one, for anything beyond a typo. A patch that
arrives without a conversation is a patch that has to be evaluated against a goal nobody agreed on
— and that is how good code gets rejected.

**Check the scope filter.** A feature belongs in mani if it deepens one of three things:

| Axis | Question |
|---|---|
| Manifest expressiveness | can more of the agent be *declared* instead of coded? |
| Safe autonomy | can an unattended agent do more without becoming dangerous? |
| Operability as a service | is it easier to run, observe, and keep running? |

If it deepens none of them, it is probably out of scope. Retrieval pipelines, vector stores,
prompt-tuning helpers, evaluation harnesses and reflection loops are all deliberately *not* here:
they are well served elsewhere, and every one of them would cost legibility.

## The non-negotiables

These are architectural invariants, not preferences. A PR that breaks one will be asked to change
regardless of how good the rest is.

1. **`core/` has zero external dependencies.** It is the domain. Adapters live in `app/`, `tool/`,
   `llm/`, `server/`. Verify with:
   ```bash
   go list -deps ./core/... | grep '\.' | grep -v '^github.com/Federicoand98/mani'   # must print nothing
   ```

2. **No cgo.** Releases cross-compile to five targets from one machine. A dependency that needs cgo
   breaks the release pipeline, so pure-Go alternatives are required:
   ```bash
   CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build ./cmd/mani   # and linux/darwin, amd64/arm64
   ```

3. **Dependencies point inward.** `cmd → app → core`. A driven adapter never imports a driving one.

4. **Unknown manifest keys are a hard error**, never a silent no-op. A new key means a new field
   *and* a case in `RuntimeSpec.Validate`, so `mani validate` fails in CI rather than the agent
   misbehaving at 3am.

5. **`gofmt`, `go vet` and `go test ./...` are green** before you open the PR.

## Tests

New behaviour comes with tests. Two specific expectations, because they are where this codebase has
actually been bitten:

- **Ports get contract tests, not per-adapter tests.** `Journal`, `TaskQueue` and `LLMClient` are
  interfaces with several implementations. A test table that runs against *every* implementation is
  worth more than three separate suites, and it is the only thing that keeps adapters from quietly
  diverging.
- **Measure the hot paths.** The journal writes on every tool call and every model response; the
  agent loop runs on every turn. If your change touches one of them, include a number in the PR
  description. "It passes the tests" says nothing about a write that costs 20 ms.

Tests must be hermetic: no network, no clock dependency, no reliance on a machine's timezone unless
the test sets it explicitly.

## Branches and commits

- **Branch from `development` and target `development`.** `master` is the release branch: it only
  ever receives a merge from `development`, immediately before a tag.
- Conventional commit subjects (`feat:`, `fix:`, `docs:`, `test:`, `chore:`) — the CHANGELOG is
  written by hand, but the subjects are what it is assembled from.
- **One PR, one concern.** A PR that adds a feature and reformats three unrelated files is two PRs.

## AI-assisted contributions

They are welcome. This project is an agent runtime; being hostile to agents would be incoherent.
Two conditions, and they are not negotiable:

1. **Disclose it** in the PR description. One line is enough. Undisclosed generated code that is
   later discovered will be closed, whatever its quality.
2. **You must be able to answer for the design.** Expect review questions of the form "why this
   structure and not that one", "what does this do under concurrency", "what happens on the failure
   path". If a PR cannot be discussed at that level, it gets closed even when the code compiles and
   the tests pass — an unmaintainable contribution is a liability, not a gift.

Generated code tends to be strong on the requirements that were written down and blind to the ones
that were not. When you use a model, spend your own attention on the second kind: what is this
change actually *for*, and what would make it fail in production rather than in CI.

## What happens after you open a PR

CI runs build, vet, gofmt and tests on Linux, macOS and Windows. **PRs from forks need a maintainer
to approve the workflow run**, so the checks may sit idle for a while before they start — that is
not a problem with your PR.

This is a one-person project maintained alongside other work. Expect a first response within a few
days, and expect substantive review rather than a rubber stamp: a contribution that gets merged is
one someone else will have to understand a year from now.

## License

By contributing you agree that your contribution is licensed under the
[Apache License 2.0](LICENSE), the same terms as the project.
