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

This ADR records the policy that the forthcoming production-readiness work
depends on (accuracy report, security report, recommended-model shortlist,
release-process docs, README commitment section), so those artifacts — none of
which exist yet — share one source of truth.

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
   injection and warn the analyst. A **single shipped defensive configuration**
   is evaluated and published — the server's boundary-tag defenses with the
   shipped playbook — not an A/B of alternative playbooks.

### Scoring and the judge model

Many assertions are graded by an LLM-as-judge (rubric grading), so the judge is
the measuring stick. Above all it must be **reproducible**: the same transcript
must earn the same grade today and in six months, so scores stay comparable
across time and across candidates. The pipeline serves **every model —
candidates and judge alike — through a single pinned provider**, so a model is
never silently routed to a different quantization or hardware between runs.
That removes the cross-provider variance that would otherwise rule out
open-weight models, and lets us choose the judge on quality and cost. The
policy:

- The judge is **pinned** (model version and provider) and run
  deterministically (temperature 0) — fixed weights, no silent upgrades.
- It is **held constant** across runs. A judge change re-baselines every score,
  so it is recorded with the results and triggers a re-run.
- It must follow detailed rubrics reliably.

**Chosen judge: a pinned `z-ai/glm-5.2`.** It sits in the frontier tier for
intelligence and instruction-following — level with the strongest hosted
models — at roughly a third of their cost. It also appears in the candidate
matrix; models are generally unaware of their own identity, so a judge grading
its own family is not a meaningful bias concern. Evidence published from runs
that predate this ADR records the judge actually used in its `results.csv`.

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

### Candidate models for the initial evaluation set

The initial test matrix deliberately spans four segments, so the recommended
list reflects the real range of how users deploy: frontier hosted models,
strong open-weight alternatives, a European-sovereign option, and small models
for on-premise or budget deployments. The recommended list is whichever
candidates pass; this matrix is the starting point, not the outcome.

| Segment | Models |
|---------|--------|
| Frontier / SOTA (common in production) | `anthropic/claude-sonnet-4.6`, `openai/gpt-5.5`, `google/gemini-3.5-flash` |
| Strong open-weight alternatives | `deepseek/deepseek-v4-pro`, `z-ai/glm-5.2`, `moonshotai/kimi-k2.6` |
| European-sovereign (Mistral) | `mistralai/mistral-medium-3-5` |
| Small — on-premise / budget | `google/gemini-3.1-flash-lite`, `qwen/qwen3.5-9b`, `mistralai/ministral-8b-2512` |

This set is a **starting proposal, not a fixed benchmark.** It will change as
models are released and in response to customer demand. We aim to keep it
somewhat comparable over time, but comparability is a best-effort goal, not the
main objective; changes follow the recommended-model policy above.

### Where and how evidence is published

- A single `docs/evaluation/` folder, linked from the README, with **one
  subfolder per evaluated MCP-server version** (`docs/evaluation/vX.Y.Z/`).
  Accuracy and security describe the same run against the same version, so they
  are published together in that version's folder — splitting them invites
  drift.
- Each version folder holds three artifacts:
  - **`results.csv` — the source of truth.** A predictable, machine-readable
    table: one row per model, with the accuracy and security counts broken down
    **per suite and per test category** (e.g. search, entity management,
    automation), plus the MCP-server version, date, and judge model. It is
    version-controlled, diffs cleanly release to release (you can see exactly
    which model moved), and is trivially machine-processable.
  - **`summary.md`** — the human-readable narrative: that version's results and
    a comparison with the previous evaluated version.
  - **`report.html`** — a richer chart-based report for visual inspection.
    Generated artifacts that embed raw attacker payloads (the prompt-injection
    showcase) stay in the internal eval suite and are **not** published here.
- The folder's `README.md` carries a prose overview of the latest evaluation
  and the methodology, and links each version folder.
- Every artifact names the model(s), MCP-server version, and judge model the
  findings apply to.

### Re-test policy (to live in `RELEASING.md`)

Re-testing is tied to **public version publishing**, not to individual feature
changes:

- **Default:** every publicly published MCP-server version is evaluated
  (accuracy + security) before release, and the `docs/evaluation/` table is
  updated to that version.
- **Exception:** if a version contains only changes that cannot affect model
  behavior — internal refactor, dependency bump, docs, CI/build — the previous
  version's results carry forward, noted in the report and the release notes.
  When in doubt, re-run.

## Consequences

**Good** — users get model-specific public evidence and a clear contract;
re-test cost is bounded by releases, not the calendar; gaps are explicit;
hardening is measurable against the same suite.

**Trade-offs** — a narrower supported model set; we can miss a provider-side
regression between runs; latency/throughput is not formally validated. Each is
revisited on real-world signal.
