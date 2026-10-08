# Work with the harness

The harness runs the same gates on every change, from your first edit to the pull request. This guide takes one change through it. Every gate, when it runs and
what makes it fail are listed in [the gates reference](../reference/gates.md).

## Once per clone

Point git at the tracked commit hook:

```sh
make dev-setup
```

Each commit then runs the commit gates on exactly what it carries. A Claude Code session says so when a checkout's commits do not run them.

## While you work

Run the gates your changed paths select, in about a minute:

```sh
make check-fast
```

The changed paths pick the suites of `gates/registry.yaml`, and the suites pick the gates: a documentation change runs the docs checks, a change to a tool runs
its tests against a live TheHive. A path no suite claims runs everything. To run one gate whatever changed, use `make gate ID=<gate>`.

`bin/harness structure` shows the largest code files and the fullest directories against the structure limits (500 lines a file, 14 code files a directory
besides tests), with `--json` for scripts.

## Every night

`.github/workflows/nightly.yml` checks main whole at 02:17 UTC, or when started by hand: every ci gate over every file, the integration suite on TheHive 5.5 and
5.8, fuzzing, the unit suite, the canaries, mutation testing of every package with a floor, and the server under load: 20 clients at once over HTTP against a
TheHive stub and 10 against TheHive 5.5 and 5.8, then 30 minutes of steady calls, held to `gates/load.yaml`, and TheHiveMCPEval's `mcp-tools` suite on
`mimo-v2.6-flash`, whose cost is capped and whose pass rate may not fall under the previous night's (`gates/eval.yaml`). Pull requests run no eval: to measure
one, start the nightly workflow on its branch. Its last job, `bin/harness triage`, files each gate that fails or cannot run as an issue labelled `nightly`,
comments on the issue each night it still does, and closes it once it passes. Started by hand on another branch, it runs that branch and only prints what it
would file. To see what to work on:

```sh
make next
```

It shows a blocked agent's `.cache/harness/BLOCKED.md` first, then the open nightly findings, the oldest first, then the open debt. Each finding says how to run
its gate again: `make gate ID=<gate> CADENCE=nightly`.

## File what is out of scope

A change often finds work it should not do: a file to split, a package to reorganise, a flaky test. Squeezing code where it does not belong to stay under the
structure limits is worse than leaving the work for later. File it instead, from an agent or by hand:

```sh
bin/harness debt --path internal/tools/search/handler.go --why "470 of 500 lines: the parsing belongs in its own file" "split the search handler"
```

It opens a GitHub issue labelled `debt` with the path, the reason and the branch it was found on. A path already filed is not filed twice, and a branch files at
most three: past that, the work is the branch's own. Close a debt issue from the pull request that does the work.

## With Claude Code

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
branch, never main. `bin/harness evidence --markdown` prints the Gate evidence section for the pull request. `make all` runs the same gates without recording or
pushing.

`make pr` also notes each run's gate times on its commit, with the machine's platform, processors and memory, under one ref per clone
(`refs/notes/harness/<id>`, an id drawn at random once and kept as `harness.notes-id` in the clone's git config), and the push takes that ref along, merged with
what origin holds under it: the timings of every developer's machine reach origin, for the repository's dashboard. A notes push that fails is a warning; the
branch is pushed all the same. Read them with `git fetch origin 'refs/notes/harness/*:refs/notes/harness/*'` and `git notes --ref harness/<id> show <commit>`. A
note is one JSON object per line: one run, or several when two clones noted the same commit. They describe developers' machines, so they go to origin only:
`.githooks/pre-push` (`make dev-setup`) refuses a push that would take `refs/notes/harness/*` to another remote, the public mirror's `git push --mirror`
included.

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
