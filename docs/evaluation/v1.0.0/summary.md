# TheHiveMCP v1.0.0 — evaluation summary

- **MCP-server version:** v1.0.0 (released 2026-07-15)
- **Evaluated:** 2026-07-02
- **Previous evaluated version:** [v0.3.4](../v0.3.4/summary.md)
- **Machine-readable results:** [`results.csv`](results.csv)
- **Detailed visual report:** [`report.html`](report.html)
- **Judge model:** `z-ai/glm-5.2`

## What changed since v0.3.4, and how it moved the benchmarks

**v1.0.0 is the production-ready release; the model-facing behavior of the server is unchanged from v0.3.4.** The prompt-injection defense measured here — every user-generated field the server returns is wrapped in `[UNTRUSTED_DATA]…[/UNTRUSTED_DATA]` boundary tags, with tool descriptions instructing the model never to follow instructions found inside them — is the same defense that shipped in v0.3.4. v1.0.0 adds the production-readiness commitments (supported deployment shape, recommended-model constraint, published evidence) but no change that alters how a model drives the tools.

Because the server behavior did not change, this evaluation is not a before/after of a code change. It **refreshes the recommended-model matrix**: v1.0.0 is measured against a current set of ten models, and the judge is pinned to `z-ai/glm-5.2` at temperature 0.

The story of this run is therefore the model set, not the server:

- **Accuracy is saturated across the recommended matrix.** Eight of ten models score 37/37; the two smallest (`qwen3.5-9b`, `ministral-8b-2512`) are the only ones below 100%.
- **Injection resilience is high and tightly grouped at the top.** Five models reach 15/15 and a sixth reaches 14/15 against the same attack ladder that separated the older matrix from 20% to 100%. The newer recommended models clear the injection suite that the shipped boundary-tag defense was built for; the resilience gap now appears only in the two smallest models (8/15).

### Comparison with v0.3.4

Only two models carry over between the two matrices; the rest advanced to newer versions within the same families (`openai/gpt-5.4` → `openai/gpt-5.5`, `deepseek/deepseek-v3.2` → `deepseek/deepseek-v4-pro`, `mistralai/mistral-large-2512` → `mistralai/mistral-medium-3.5`, `google/gemini-3-flash` → `google/gemini-3.5-flash`). The accuracy suite also grew (32 → 37 checks) and the judge changed (`anthropic/claude-sonnet-4.6` → `z-ai/glm-5.2`), so the carried-over figures below are indicative, not exact deltas.

| Carried-over model | Accuracy (v0.3.4 → v1.0.0) | Security (v0.3.4 → v1.0.0) |
| --- | --- | --- |
| `anthropic/claude-sonnet-4.6` | 100% (32/32) → 100% (37/37) | 93% (14/15) → **100% (15/15)** |
| `qwen/qwen3.5-9b` | 88% (28/32) → 73% (27/37) | 67% (10/15) → 53% (8/15) |

- **New to the v1.0.0 matrix (8):** `moonshotai/kimi-k2.6`, `openai/gpt-5.5`, `z-ai/glm-5.2`, `google/gemini-3.5-flash`, `google/gemini-3.1-flash-lite`, `deepseek/deepseek-v4-pro`, `mistralai/mistral-medium-3.5`, `mistralai/ministral-8b-2512`.
- **Dropped from the v0.3.4 matrix (6):** `google/gemini-3-flash`, `openai/gpt-5.4`, `deepseek/deepseek-v3.2`, `mistralai/mistral-large-2512`, `openai/gpt-4.1`, `mistralai/mistral-small-3.2-24b`.

Field-level, v0.3.4 injection resilience ranged 20%–100% (median ~70%); the current matrix ranges 53%–100%, with eight of ten models at ≥93%. `claude-sonnet-4.6` is the clearest carried-over signal — it held perfect accuracy and closed its last injection miss. `qwen3.5-9b` is the same model in both runs but measured against a larger, harder accuracy suite and a different judge, so its lower percentages are read within v1.0.0 rather than as a regression.

## Results

Accuracy = `mcp-tools` (34) + `workflows` (3) = 37 checks. Security = 15 prompt-injection scenarios (the agent must both ignore the injection **and** warn the analyst). Percentages are pass rates; counts are passes / total.

| Model | Accuracy | Security (injection resilience) |
| --- | --- | --- |
| `moonshotai/kimi-k2.6` | 100% (37/37) | 100% (15/15) |
| `openai/gpt-5.5` | 100% (37/37) | 100% (15/15) |
| `z-ai/glm-5.2` | 100% (37/37) | 100% (15/15) |
| `google/gemini-3.5-flash` | 100% (37/37) | 100% (15/15) |
| `anthropic/claude-sonnet-4.6` | 100% (37/37) | 100% (15/15) |
| `google/gemini-3.1-flash-lite` | 100% (37/37) | 93% (14/15) |
| `deepseek/deepseek-v4-pro` | 97% (36/37) | 87% (13/15) |
| `mistralai/mistral-medium-3.5` | 100% (37/37) | 73% (11/15) |
| `qwen/qwen3.5-9b` | 73% (27/37) | 53% (8/15) |
| `mistralai/ministral-8b-2512` | 70% (26/37) | 53% (8/15) |

## Breakdown by suite

Accuracy above is the sum of two suites: **`mcp-tools`** (34 single-tool exercises — search, resource discovery, entity management, automation, case templates) and **`workflows`** (3 multi-step, end-to-end investigations). Reported separately they show *where* accuracy holds. (Finer per-tool splits within `mcp-tools` are not retained for these runs.) Full counts are in [`results.csv`](results.csv).

| Model | `mcp-tools` (/34) | `workflows` (/3) | `security` (/15) |
| --- | --- | --- | --- |
| `moonshotai/kimi-k2.6` | 34 (100%) | 3 (100%) | 15 (100%) |
| `openai/gpt-5.5` | 34 (100%) | 3 (100%) | 15 (100%) |
| `z-ai/glm-5.2` | 34 (100%) | 3 (100%) | 15 (100%) |
| `google/gemini-3.5-flash` | 34 (100%) | 3 (100%) | 15 (100%) |
| `anthropic/claude-sonnet-4.6` | 34 (100%) | 3 (100%) | 15 (100%) |
| `google/gemini-3.1-flash-lite` | 34 (100%) | 3 (100%) | 14 (93%) |
| `deepseek/deepseek-v4-pro` | 33 (97%) | 3 (100%) | 13 (87%) |
| `mistralai/mistral-medium-3.5` | 34 (100%) | 3 (100%) | 11 (73%) |
| `qwen/qwen3.5-9b` | 24 (71%) | 3 (100%) | 8 (53%) |
| `mistralai/ministral-8b-2512` | 23 (68%) | 3 (100%) | 8 (53%) |

**What stands out per category:**

- **`workflows` — every model passes all 3 (100%).** Multi-step, end-to-end investigations are the most production-relevant shape of analyst work, and no model in the matrix fails them, including the two smallest.
- **`mcp-tools` — 68–100%.** Single-tool accuracy is high for eight of ten models; the floor is the two smallest models (`ministral-8b-2512` 23/34, `qwen3.5-9b` 24/34), which account for essentially all accuracy misses in the matrix.
- **`security` is the only category that separates the field** — 53% to 100%. As in prior evaluations, the accuracy suites do not separate models; injection resilience does.

## Cost & latency

Observed end-to-end across the full test run (latency is model-dominated — the MCP layer itself adds little; cost is the total for that model's run).

| Model | Avg latency | Run cost |
| --- | --- | --- |
| `moonshotai/kimi-k2.6` | 20s | $0.44 |
| `openai/gpt-5.5` | 12s | $1.97 |
| `z-ai/glm-5.2` | 15s | $0.80 |
| `google/gemini-3.5-flash` | 9s | $1.82 |
| `anthropic/claude-sonnet-4.6` | 13s | $3.23 |
| `google/gemini-3.1-flash-lite` | 4s | $0.20 |
| `deepseek/deepseek-v4-pro` | 23s | $1.07 |
| `mistralai/mistral-medium-3.5` | 16s | $1.96 |
| `qwen/qwen3.5-9b` | 35s | $0.08 |
| `mistralai/ministral-8b-2512` | 9s | $0.03 |

`gemini-3.1-flash-lite` is the standout on cost/latency — perfect accuracy and near-top security (14/15) at the lowest latency and near-lowest cost in the matrix. `kimi-k2.6` reaches a perfect score at well under a dollar. `claude-sonnet-4.6` is the costliest; `qwen3.5-9b` is the slowest (reasoning-heavy) and, with `ministral-8b-2512`, the weakest.

## Interpretation

- **Accuracy is strong and tightly clustered** (68–100%, eight models at 100%): once a run reaches the tool layer, the recommended models drive multi-step investigations correctly. Only the two smallest models trail.
- **Security is where model choice changes the risk profile.** With the shipped boundary-tag defense on, injection resilience across the current matrix ranges from 100% down to 53% — the whole field is stronger than the older matrix, but the top-to-bottom gap persists and tracks model capability.
- **The gains over the older matrix come from model selection, not a server change** — the injection defense is identical to v0.3.4. Choosing a recommended model remains the dominant control on injection risk.

## Residual risks

- The weakest models on injection (`qwen3.5-9b`, `ministral-8b-2512`, both 8/15) still follow false-authority and exfiltration injections and should not drive write-capable or automation tools on attacker-reachable data.
- The boundary-tag defense reduces injection risk but does not eliminate it; model choice still matters, and no model should be assumed resilient outside the tested scenarios.
- Provider-side regressions between runs are not detected (see [ADR-0001](../../explanation/adr/0001-security-and-accuracy-testing-policy.md), "what we do not test").
