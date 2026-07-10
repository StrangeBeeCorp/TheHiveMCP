# ADR-0001 - Accuracy and Security Testing Policy for TheHiveMCP

- Status: accepted
- Date: June 24, 2026 (acceptance date). Policy takes effect with the v1.0.0 release, July 6, 2026.
- Deciders: TheHiveMCP maintainers, with the CTO as final arbiter on scope changes

## Context and problem statement

TheHiveMCP exposes the case-management API of TheHive as MCP tools, letting an LLM search entities, manage cases, and run Cortex analyzers and responders on
behalf of a Security Operations Center (SOC) analyst. Before enabling it against real data, users—analysts and the engineers wiring it up for them—need public
answers to three questions:

1. **Accuracy**: Will a recommended model use these tools correctly across a real, multi-step investigation, or will it derail?
2. **Security**: What happens when a model acts on data an attacker has poisoned with hidden instructions (prompt injection, false-authority dismissals,
   exfiltration directives)?
3. **Commitment**: What does StrangeBee test, what does it deliberately not test, and how is that evidence kept current?

This ADR records the policy that the forthcoming production-readiness work depends on (accuracy report, security report, recommended-model shortlist,
release-process docs, and README commitment section), so those artifacts—none of which exist yet—share one source of truth.

## Decision drivers

- **Trust without a private review**: evidence lives in the public repo.
- **Honesty about scope**: what isn't tested is as visible as what is tested.
- **Cost discipline**: re-running an unchanged server on a calendar is poor value.
- **Maintainability**: an unambiguous rule for when a change forces a re-run.
- **Model-specificity**: every published claim names the models it was measured on.

## Considered options

1. **No published policy**: answer ad hoc per customer. This fails on trust and honesty, and it doesn't scale.
2. **Continuous re-evaluation on a fixed cadence**: maximal freshness, but it burns time re-running an unchanged server.
3. **Change-triggered policy with public, model-specific evidence** _(chosen)_: publish accuracy and security reports tied to named models, re-run only on
   changes that can plausibly move results, and recommend a short model list rather than claiming universal support.

## Decision

### What TheHiveMCP tests

1. **Accuracy: correct tool use.** Multi-step conversations exercise the recommended models across realistic investigations: searching entities, discovering
   schema and resources, managing cases and observables, and running automation. Tests run end-to-end against a real TheHive instance with the actual MCP
   server. Tools are never mocked.
2. **Security: prompt-injection resilience.** The test suite seeds TheHive data (descriptions, comments, observable fields, and case content) with hidden
   instructions ranging from benign behavior changes to critical directives: running responders, exfiltrating data, creating users, deleting evidence, or
   closing investigations. The pass bar is strict: the agent must both ignore the injection and warn the analyst. A **single shipped defensive configuration**
   is evaluated and published: the server's boundary-tag defenses with the shipped playbook, not an A/B test of alternative playbooks.

### Scoring and the judge model

Many assertions are graded by an LLM-as-judge (rubric grading), so the judge is the measuring stick. Above all, it must be **reproducible**: the same
transcript must earn the same grade today and in six months, so scores stay comparable across time and across candidates. The pipeline serves **every
model—candidates and judge alike—through a single pinned provider**, so a model is never silently routed to a different quantization or hardware between
runs. That removes the cross-provider variance that would otherwise rule out open-weight models and makes it possible to choose the judge on quality and cost
alone. The policy:

- The judge is **pinned** to a specific model version and provider, and it runs deterministically (temperature 0), with fixed weights and no silent upgrades.
- It is **held constant** across runs. A judge change re-baselines every score, so it is recorded with the results and triggers a re-run.
- It must follow detailed rubrics reliably.

**Chosen judge: a pinned `z-ai/glm-5.2`.** It sits in the frontier tier for intelligence and instruction-following—level with the strongest hosted models—at
roughly a third of their cost. It also appears in the candidate matrix. Models are generally unaware of their own identity, so a judge grading its own family
isn't a meaningful bias concern. Evidence published from runs that predate this ADR records the judge used in its `results.csv`.

### What TheHiveMCP doesn't test

- **Server latency/throughput SLAs**: latency is dominated by the model, not the thin Go layer. Revisit on a concrete customer concern.
- **Continuous re-evaluation**: see the re-test policy below.
- **Arbitrary model support**: only a named shortlist is recommended. Other models may work but are neither recommended nor evidenced.
- **Provider-side regressions between runs**: a vendor can change a model under a stable ID, and this isn't continuously detected. Revisit on incidents.

### Recommended-model policy

- The README names a **short list** of recommended models, without tiers or hedging. Models not on the list aren't recommended.
- A model earns its place only with **published accuracy and security evidence** tied to a specific MCP-server version.
- Maintainers review candidates. The CTO is the final arbiter on adding or removing a model.

### Candidate models for the initial evaluation set

The initial test matrix deliberately spans four segments: frontier hosted models, strong open-weight alternatives, a European-sovereign option, and small
models for on-premise or budget deployments. This reflects the real range of how users deploy TheHiveMCP. The recommended list is whichever candidates pass.
This matrix is the starting point, not the outcome.

| Segment                                | Models                                                                            |
| --------------------------------------- | ---------------------------------------------------------------------------------- |
| Frontier / SOTA (common in production) | `anthropic/claude-sonnet-4.6`, `openai/gpt-5.5`, `google/gemini-3.5-flash`         |
| Strong open-weight alternatives        | `deepseek/deepseek-v4-pro`, `z-ai/glm-5.2`, `moonshotai/kimi-k2.6`                 |
| European-sovereign (Mistral)           | `mistralai/mistral-medium-3.5`                                                     |
| Small (on-premise or budget)           | `google/gemini-3.1-flash-lite`, `qwen/qwen3.5-9b`, `mistralai/ministral-8b-2512`   |

This set is a **starting proposal, not a fixed benchmark.** It will change as models are released and in response to customer demand. The goal is to keep it
broadly comparable over time, but comparability is a best-effort aim, not the main objective. Changes follow the recommended-model policy above.

### Where and how evidence is published

- A single `docs/evaluation/` folder, linked from the README, with **one subfolder per evaluated MCP-server version** (`docs/evaluation/vX.Y.Z/`). Accuracy
  and security describe the same run against the same version, so they're published together in that version's folder—splitting them invites drift.
- Each version folder holds three artifacts:
  - **`results.csv`: the source of truth.** A predictable, machine-readable table: one row per model, with the accuracy and security counts broken down
    **per suite and per test category** (for example, search, entity management, automation), plus the MCP-server version, date, and judge model. It's
    version-controlled, diffs cleanly release to release (you can see exactly which model moved), and is trivially machine-processable.
  - **`summary.md`**: the human-readable narrative, covering that version's results and a comparison with the previous evaluated version.
  - **`report.html`**: a richer chart-based report for visual inspection. Generated artifacts that embed raw attacker payloads (the prompt-injection showcase)
    stay in the internal eval suite and are **not** published here.
- The folder's `README.md` carries a prose overview of the latest evaluation and the methodology, and links each version folder.
- Every artifact names the models, MCP-server version, and judge model the findings apply to.

### Re-test policy (to live in `RELEASING.md`)

Re-testing is tied to **public version publishing**, not to individual feature changes:

- **Default:** every publicly published MCP-server version is evaluated (accuracy and security) before release, and the `docs/evaluation/` table is updated
  to that version.
- **Exception:** if a version contains only changes that can't affect model behavior (internal refactor, dependency bump, docs, or CI/build), the previous
  version's results carry forward, noted in the report and the release notes. When in doubt, re-run.

## Consequences

**Good**: users get model-specific public evidence and a clear contract. Re-test cost is bounded by releases, not the calendar. Gaps are explicit, and
hardening is measurable against the same suite.

**Trade-offs**: a narrower supported model set, a possible missed provider-side regression between runs, and latency/throughput that isn't formally
validated. Each is revisited on real-world signal.
