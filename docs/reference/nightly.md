# The nightly run and the gate-time notes

This page describes what the nightly run checks, the metric thresholds `bin/harness metrics` holds, and the format of the gate-time notes `make pr` records.

## What the nightly run does

`.github/workflows/nightly.yml` checks main whole at 02:17 UTC, or when started by hand: every ci gate over every file, the integration suite on TheHive 5.5 and
5.8, fuzzing, the unit suite, the canaries, mutation testing of every package with a floor, and the server under load: 20 clients at once over HTTP against a
TheHive stub and 10 against TheHive 5.5 and 5.8, then 30 minutes of steady calls, held to `gates/load.yaml`, and TheHiveMCPEval's `mcp-tools` suite on
`mimo-v2.6-flash`, whose cost is capped and whose pass rate may not fall under the previous night's (`gates/eval.yaml`). Pull requests run no eval: to measure
one, start the nightly workflow on its branch. Its last job, `bin/harness triage`, files each gate that fails or cannot run as an issue labelled `nightly`,
comments on the issue each night it still does, and closes it once it passes. Started by hand on another branch, it runs that branch and only prints what it
would file.

## Metric thresholds

`bin/harness metrics` holds the dashboard's metrics to `gates/metrics.yaml`: the night's coverage may not fall more than 2 points from the previous night's
(once it has, its issue stays open until coverage is back within 2 points of the level it fell from), main's CI may not grow more than 25 % over the four weeks
before, no finding or debt issue may stay open past 14 days, and a release must come within 60 days. A threshold crossed files an issue labelled `metrics`,
commented on each night it stays crossed and closed on the first night the metric is back under it. A metric the night could not measure, or could not compare,
is said on its issue, which it neither files nor closes. Each night that changed its issues keeps its values as the run's `metrics` artifact, for the next night
to compare with, and a drop's issue keeps the level the drop is measured from, for a night that cannot download them.

## Gate-time notes

`make pr` also notes each run's gate times on its commit, with the machine's platform, processors and memory, under one ref per clone
(`refs/notes/harness/<id>`, an id drawn at random once and kept as `harness.notes-id` in the clone's git config), and the push takes that ref along, merged with
what origin holds under it: the timings of every developer's machine reach origin, for the repository's dashboard. A notes push that fails is a warning; the
branch is pushed all the same. Read them with `git fetch origin 'refs/notes/harness/*:refs/notes/harness/*'` and `git notes --ref harness/<id> show <commit>`. A
note is one JSON object per line: one run, or several when two clones noted the same commit. They describe developers' machines, so they go to origin only:
`.githooks/pre-push` (`make dev-setup`) refuses a push that would take `refs/notes/harness/*` to another remote, the public mirror's `git push --mirror`
included.
