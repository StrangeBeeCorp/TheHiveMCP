# Releasing TheHiveMCP

This is the operational checklist for cutting a public release. The **testing policy it enforces is defined in
[ADR-0001](docs/explanation/adr/0001-security-and-accuracy-testing-policy.md), which is the source of truth** — if this document and the ADR ever disagree, the
ADR wins.

## Re-test policy

Published accuracy and security evidence (in [`docs/evaluation/`](docs/evaluation/)) must stay honest as the server evolves. Re-testing is tied to **public
version publishing**, not to individual changes:

- **Default — every publicly published version is re-evaluated** (accuracy + security) before the release is cut, and a new `docs/evaluation/vX.Y.Z/` folder
  (`results.csv` + `summary.md` + `report.html`) is added for the new version.
- **Exception — no-behavior-change releases.** If a version contains only changes that cannot affect model behavior — internal refactor, dependency bump, docs,
  CI/build — the previous version's results carry forward, recorded in the evaluation report and the release notes. **When in doubt, re-run.**

The evaluation set (candidate models and the pinned judge model) is defined in the ADR.

## Release notes

Every release ships **curated, categorized release notes** so adopters can decide whether to upgrade without reading the commit log. Each release sorts its
changes into:

- **User-facing changes** — new tools, capabilities, behavior changes.
- **Breaking changes** — anything requiring action to upgrade, with upgrade notes.
- **Security fixes** — called out explicitly.

> Today the release workflow sets `generate_release_notes: true`, which produces an uncurated dump of PR titles. Replacing that with a curated mechanism (and
> backfilling notes for past versions) is tracked separately; until then, curate the generated notes by hand against the categories above before publishing.

## Release checklist

Before cutting a release (pushing a `vX.Y.Z` tag):

- [ ] **Re-test policy applied** — either re-ran accuracy + security and updated [`docs/evaluation/`](docs/evaluation/), **or** confirmed this is a
      no-behavior-change release and carried the previous results forward.
- [ ] Which suites were re-run for this release:
  - [ ] Accuracy
  - [ ] Security
  - [ ] None — no-behavior-change release; results carried forward from version: `______`
- [ ] A `docs/evaluation/vX.Y.Z/` folder exists for this version (`results.csv` + `summary.md` + `report.html`), or the carried-forward version is stated.
- [ ] `results.csv` includes the per-suite / per-category breakdown (not just the accuracy/security totals).
- [ ] The judge was the pinned `z-ai/glm-5.2` (temperature 0) and is recorded in `results.csv`.
- [ ] **Release notes curated** — user-facing / breaking / security sections filled; breaking changes include upgrade guidance.
- [ ] README and docs updated for any new or changed tools.
- [ ] Version/tag matches the artifacts being published.
