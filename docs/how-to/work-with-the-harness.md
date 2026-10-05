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

## With Claude Code

The hooks of `.claude/settings.json` send every tool call to `bin/harness hook`:

- an edit or a read is judged by the role's paths: the Tester writes tests, the Coder writes code, and agents leave the harness, the gates, the spec, the
  baselines and the default permission profile to people;
- when an agent stops, `make check-fast` runs, and every file the agent changed is judged by its role again, whatever command changed it. A change it may not
  make sends it back with the command that restores the file;
- an agent does not stop red: after six red stops it writes `.cache/harness/BLOCKED.md` with what it tried and what it needs.

Claude Code's own permission rules refuse a push, a merge and an approval from a Claude session. [ADR-0004](../explanation/adr/0004-judge-outcomes.md) explains
why the harness judges what changed rather than the commands typed.

## Ship

Once the adversarial review is ready for HEAD:

```sh
make pr
```

It runs the pr gates at that exact commit, records the evidence under `.cache/harness/evidence/`, checks that the review covers the same commit, and pushes the
branch, never main. `bin/harness evidence --markdown` prints the Gate evidence section for the pull request. `make all` runs the same gates without recording or
pushing.

CI runs every gate again on the pull request, against TheHive 5.5 and 5.6, and a person merges it.
