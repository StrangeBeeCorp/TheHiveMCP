# Work with the harness

The harness runs the same gates on every change, from your first edit to the pull request. This guide takes one change through it. Every gate, when it runs and
what makes it fail are listed in [the gates reference](../reference/gates.md).

## Once per clone

Point git at the tracked commit hook:

```sh
make dev-setup
```

This points git at the tracked hooks in `.githooks/` (`git config core.hooksPath .githooks`): `pre-commit` runs `make pre-commit`, the commit gates on exactly
what the commit will carry. It rewrites no file, and `--no-verify` bypasses it: it is a fast local signal, while the authoritative gate stays CI. A Claude Code
session says so when a checkout's commits do not run them. Every make target is listed in [the make targets reference](../reference/make-targets.md).

## While you work

Run the gates your changed paths select, in about a minute:

```sh
make check-fast
```

The changed paths pick the suites of `gates/registry.yaml`, and the suites pick the gates: a documentation change runs the docs checks, a change to a tool runs
its tests against a live TheHive. A path no suite claims runs everything. To run one gate whatever changed, use `make gate ID=<gate>`.

`bin/harness structure` shows the largest code files and the fullest directories against the structure limits (500 lines a file, 14 code files a directory
besides tests), with `--json` for scripts.

## Test options

- Coverage is off by default. Add `COVERAGE=1` to either test target to write `coverage.out` and print a per-function report:
  - `make test COVERAGE=1`
  - `make test-integration COVERAGE=1`

**Integration test TheHive version:** the integration suite (`make test-integration`) boots a live TheHive + Elasticsearch stack via docker compose
(`docker-compose.test.yml`). CI runs `make test-integration` as a matrix against the oldest and newest supported TheHive versions (currently 5.5 and 5.8), so a
compatibility break at either end fails the PR. The image is a single source of truth: set `THEHIVE_TEST_IMAGE` to override it locally (e.g.
`THEHIVE_TEST_IMAGE=strangebee/thehive:5.5.16 make test-integration`); unset, it defaults to `strangebee/thehive:5.8.0`.

**Integration tests against a shared TheHive (opt-in):** the default above stays the reference. If you have a long-running TheHive shared between projects,
`make test-integration-shared` runs the suite sequentially inside one organisation of it instead (org-admin API key, no compose stack, no database reset, no
instance-wide change), and the same instance can back a locally running server. See [Develop and test against a shared TheHive](shared-thehive.md).

## Work the nightly findings

The nightly run files each gate that fails or cannot run as an issue labelled `nightly`; what it checks and the thresholds it holds are in
[the nightly reference](../reference/nightly.md).

To see what to work on:

```sh
make next
```

It shows a blocked agent's `.cache/harness/BLOCKED.md` first, then the open nightly findings, the oldest first, then the metrics past their threshold, then the
open debt. Each finding says how to run its gate again: `make gate ID=<gate> CADENCE=nightly`.

## File what is out of scope

A change often finds work it should not do: a file to split, a package to reorganise, a flaky test. Squeezing code where it does not belong to stay under the
structure limits is worse than leaving the work for later. File it instead, from an agent or by hand:

```sh
bin/harness debt --path internal/tools/search/handler.go --why "470 of 500 lines: the parsing belongs in its own file" "split the search handler"
```

It opens a GitHub issue labelled `debt` with the path, the reason and the branch it was found on. A path already filed is not filed twice, and a branch files at
most three: past that, the work is the branch's own. Close a debt issue from the pull request that does the work.

## What the Claude Code hooks enforce

The hooks of `.claude/settings.json` send every tool call to `bin/harness hook`:

- an edit or a read is judged by the role's paths: the Tester writes tests, the Coder writes code, and agents leave the harness, the gates, the spec, the
  baselines and the default permission profile to people;
- when an agent stops, `make check-fast` runs, and every file the agent changed is judged by its role again, whatever command changed it. A change it may not
  make sends it back with the command that restores the file;
- an agent does not stop red: after six red stops it writes `.cache/harness/BLOCKED.md` with what it tried and what it needs;
- an agent whose green stop leaves a file at 90 % of its line limit, a full directory or a new one is asked once to split it or to file it with
  `bin/harness debt`;
- a commit made from a Claude Code session carries three trailers, `Agent-Model`, `Agent-Effort` and `Skills-Used`, read from the session's environment and
  transcript (`.githooks/prepare-commit-msg`): git history then says which models, efforts and skills wrote the code. The skills the repository defines
  (`.claude/skills/`) are named; those of the user's own plugins are only counted, since the history reaches the public mirror. What the session cannot tell is
  `unknown`; a person's commit carries none.

Claude Code's own permission rules refuse a push, a merge and an approval from a Claude session. [ADR-0004](../explanation/adr/0004-judge-outcomes.md) explains
why the harness judges what changed rather than the commands typed.

## Ship

Once the adversarial review is ready for HEAD:

```sh
make pr
```

It runs the pr gates at that exact commit, records the evidence under `.cache/harness/evidence/`, checks that the review covers the branch, and pushes the
branch, never main. `bin/harness evidence --markdown` prints the Gate evidence section for the pull request (add `--base origin/<its base>` for a stacked one),
followed by its Change metrics, measured on the merge base and on the branch:

- the diff coverage: the share of the changed non-test Go statements that the unit tests ran, from the coverage profile `make pr` keeps beside the evidence;
- the code files the change lengthens to within a quarter of their size limit, before and after;
- the `//nolint` directives, the TODO comments and the dependencies it adds.

`make all` runs the same gates without recording or pushing.

`make pr` also notes each run's gate times on its commit; the format and how to read the notes are in
[the nightly reference](../reference/nightly.md#gate-time-notes).

A rebase asks for no new review: `make pr FORCE=1` pushes the rebased branch on the review that read it before. The push pairs the branch's commits with the
ones the review read, and lists two kinds in its output and in the Gate evidence, for whoever merges: a commit the rebase changed (it resolved a conflict in it)
and a commit added since the review, which no review read. A branch none of whose commits a ready review read is refused.

CI runs every gate again on the pull request, against TheHive 5.5 and 5.8, and a person merges it.

## See the repository's health

```sh
make dashboard
```

It builds one page, `.cache/dashboard/index.html`, and opens it: who and which models contribute (the commits' trailers, `Co-Authored-By` before them), how long
the gates take on CI and on every developer's machine (the notes of `make pr`), the tests per layer, merges and releases, lead times, quality (pull request
grades, the last local run's coverage and mutation scores, files against the structure limits, `//nolint` and TODO counts) and DORA-style measures of main.
`make dashboard WEEKS=52` looks further back than the default 26 weeks. Every number is read when the page is built, from git, `gh` and the tree, in about ten
seconds; a source it cannot read is named at the top and in the sections it feeds, and never stops the build.

Jira's lead times need `JIRA_BASE_URL`, `JIRA_EMAIL`, `JIRA_API_TOKEN` and `JIRA_JQL` (the done tickets to chart, such as
`project = DL AND statusCategory = Done AND resolved >= -26w`) in the environment; without them that chart is left out, and the page says so.
