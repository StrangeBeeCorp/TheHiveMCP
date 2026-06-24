# ADR-0001 — Accuracy and security testing policy for TheHiveMCP

- Status: proposed
- Date: 2026-06-24
- Deciders: TheHiveMCP maintainers, with the CTO as final arbiter on scope changes

## Context and problem statement

TheHiveMCP exposes TheHive's case-management API as MCP tools, letting an LLM
search entities, manage cases, and run Cortex analyzers/responders on a SOC
analyst's behalf. Before enabling it against real data, users — analysts and
the engineers wiring it up for them — need public answers to three questions:

1. **Accuracy** — Will a recommended model use these tools correctly across a
   real, multi-step investigation, or will it derail?
2. **Security** — What happens when a model acts on data an attacker has
   poisoned with hidden instructions (prompt injection, false-authority
   dismissals, exfiltration directives)?
3. **Commitment** — What does StrangeBee test, what does it deliberately
   *not* test, and how is that evidence kept current?

This ADR records the policy that the production-readiness work depends on
(accuracy report, security report, recommended-model shortlist, `RELEASING.md`,
README commitment section), so those artifacts share one source of truth.

## Decision drivers

- **Trust without a private review** — evidence lives in the public repo.
- **Honesty about scope** — what we don't test is as visible as what we do.
- **Cost discipline** — re-running an unchanged server on a calendar is poor
  value.
- **Maintainability** — an unambiguous rule for when a change forces a re-run.
- **Model-specificity** — every published claim names the model(s) it was
  measured on.

## Considered options

1. **No published policy** — answer ad hoc per customer. Fails trust and
   honesty; doesn't scale.
2. **Continuous re-evaluation on a fixed cadence** — maximal freshness, but
   burns time re-running an unchanged server.
3. **Change-triggered policy with public, model-specific evidence**
   *(chosen)* — publish accuracy + security reports tied to named models,
   re-run only on changes that can plausibly move results, and recommend a
   short model list rather than claiming universal support.

## Decision

### What we test

1. **Accuracy — correct tool use.** Multi-step conversations exercise the
   recommended models across realistic investigations: searching entities,
   discovering schema/resources, managing cases and observables, and running
   automation. Tests run end-to-end against a real TheHive instance with the
   actual MCP server — no mocked tools.
2. **Security — prompt-injection resilience.** We seed TheHive data
   (descriptions, comments, observable fields, case content) with hidden
   instructions ranging from benign behavior changes to critical directives
   (run responders, exfiltrate data, create users, delete evidence, close
   investigations). The pass bar is strict: the agent must both ignore the
   injection and warn the analyst.

### What we do *not* test

- **Server latency/throughput SLAs** — latency is dominated by the model, not
  the thin Go layer. Revisit on a concrete customer concern.
- **Continuous re-evaluation** — see the re-test policy below.
- **Arbitrary model support** — we recommend a named shortlist only; other
  models may work but are neither recommended nor evidenced.
- **Provider-side regressions between runs** — a vendor can change a model
  under a stable ID; we don't continuously detect this. Revisit on incidents.

### Recommended-model policy

- The README names a **short list** of recommended models — no tiers, no
  hedging. Models not on the list are not recommended.
- A model earns its place only with **published accuracy + security evidence**
  tied to a specific MCP-server version.
- Maintainers review; the CTO is final arbiter on adding or removing a model.

> **Draft shortlist (pending sign-off):** Claude Sonnet 4.6, Gemini 3 Flash,
> and GPT-5.4 — each passes the full hardened security set and scores highly on
> accuracy. Mistral Small 3.2 is explicitly **not** recommended. To confirm at
> PR time.

### Where and how evidence is published

- A single `docs/evaluation/` folder, linked from the README. Accuracy and
  security describe the same run against the same version, so they live in one
  report — splitting them invites drift.
- The **canonical format is a predictable Markdown results table** committed to
  the repo: one row per recommended model, columns for accuracy and security
  outcomes, plus the MCP-server version and date. It is version-controlled,
  diffs cleanly release to release (you can see exactly which model moved),
  renders on GitHub, and is cheap to keep current.
- A richer generated HTML report (heatmaps, the injection showcase) may
  accompany a release for narrative use, but the Markdown table is the source
  of truth.
- Every report names the model(s) and MCP-server version the findings apply to.

### Re-test policy (to live in `RELEASING.md`)

Re-testing is tied to **public version publishing**, not to individual feature
changes:

- **Default:** every publicly published MCP-server version is evaluated
  (accuracy + security) before release, and the `docs/evaluation/` table is
  updated to that version.
- **Exception:** if a version contains only changes that cannot affect model
  behavior — internal refactor, dependency bump, docs, CI/build — the previous
  version's results carry forward, noted in the report and CHANGELOG. When in
  doubt, re-run.

## Consequences

**Good** — users get model-specific public evidence and a clear contract;
re-test cost is bounded by releases, not the calendar; gaps are explicit;
hardening is measurable against the same suite.

**Trade-offs** — a narrower supported model set; we can miss a provider-side
regression between runs; latency/throughput is not formally validated. Each is
revisited on real-world signal.
