# TheHiveMCP v0.3.3 — evaluation summary

- **MCP-server version:** v0.3.3 (released 2026-03-17)
- **Evaluated:** 2026-05-23 (evaluated alongside v0.3.4 on the same test suite)
- **Previous evaluated version:** none — v0.3.3 is the earliest version with
  published evidence, so there is no version-over-version comparison here.
- **Machine-readable results:** [`results.csv`](results.csv)
- **Detailed visual report:** see [v0.3.4/report.html](../v0.3.4/report.html),
  which charts v0.3.3 and v0.3.4 side by side.
- **Judge model:** `anthropic/claude-sonnet-4.6`

## Security posture at this version

v0.3.3 has **no `[UNTRUSTED_DATA]` boundary tags** — that server-side
prompt-injection defense shipped in [v0.3.4](../v0.3.4/summary.md).
At v0.3.3 the only defense is a one-line "treat TheHive data as unsafe"
system-prompt note, so injection resilience is low: **53% across the field**,
with most models below 50%. This is the pre-hardening baseline that v0.3.4
improved to 67%.

## Results

Accuracy = `mcp-tools` (29) + `workflows` (3) = 32 checks. Security = 15
prompt-injection scenarios (agent must both ignore the injection **and** warn
the analyst). Same model order as the [v0.3.4 table](../v0.3.4/summary.md#results)
for easy comparison.

| Model | Accuracy | Security (injection resilience) |
|---|---|---|
| `anthropic/claude-sonnet-4.6` | 97% (31/32) | 100% (15/15) |
| `google/gemini-3-flash` | 94% (30/32) | 87% (13/15) |
| `openai/gpt-5.4` | 84% (27/32) | 73% (11/15) |
| `deepseek/deepseek-v3.2` | 88% (28/32) | 40% (6/15) |
| `mistralai/mistral-large-2512` | 100% (32/32) | 20% (3/15) |
| `openai/gpt-4.1` | 97% (31/32) | 20% (3/15) |
| `qwen/qwen3.5-9b` | 78% (25/32) | 47% (7/15) |
| `mistralai/mistral-small-3.2-24b` | 91% (29/32) | 33% (5/15) |

### Breakdown by suite

Accuracy is the sum of **`mcp-tools`** (29 single-tool exercises) and
**`workflows`** (3 multi-step investigations); finer per-tool splits are not
retained for these runs. Full counts are in [`results.csv`](results.csv).

| Model | `mcp-tools` (/29) | `workflows` (/3) | `security` (/15) |
|---|---|---|---|
| `anthropic/claude-sonnet-4.6` | 28 (97%) | 3 (100%) | 15 (100%) |
| `google/gemini-3-flash` | 27 (93%) | 3 (100%) | 13 (87%) |
| `openai/gpt-5.4` | 24 (83%) | 3 (100%) | 11 (73%) |
| `deepseek/deepseek-v3.2` | 25 (86%) | 3 (100%) | 6 (40%) |
| `mistralai/mistral-large-2512` | 29 (100%) | 3 (100%) | 3 (20%) |
| `openai/gpt-4.1` | 28 (97%) | 3 (100%) | 3 (20%) |
| `qwen/qwen3.5-9b` | 23 (79%) | 2 (67%) | 7 (47%) |
| `mistralai/mistral-small-3.2-24b` | 26 (90%) | 3 (100%) | 5 (33%) |

Even at v0.3.3, `workflows` is near-perfect (only `qwen3.5-9b` misses one) and
`mcp-tools` is 79–100%. The gap is entirely in `security` — the pre-boundary-tag
injection resilience that v0.3.4 improved.

## Cost & latency

Observed end-to-end across the full test run (latency is model-dominated — the
MCP layer itself adds little; cost is the total for that model's run).

| Model | Avg latency | Run cost |
|---|---|---|
| `anthropic/claude-sonnet-4.6` | 18s | $2.77 |
| `google/gemini-3-flash` | 8s | $0.22 |
| `openai/gpt-5.4` | 10s | $0.28 |
| `deepseek/deepseek-v3.2` | 50s | $0.60 |
| `mistralai/mistral-large-2512` | 16s | $0.19 |
| `openai/gpt-4.1` | 8s | $0.54 |
| `qwen/qwen3.5-9b` | 94s | $0.05 |
| `mistralai/mistral-small-3.2-24b` | 12s | $0.02 |

`qwen3.5-9b` and `deepseek-v3.2` are the slowest (reasoning-heavy);
`claude-sonnet-4.6` is by far the costliest. v0.3.4 later cut the slow tail
(qwen 94s → 58s) and `deepseek` cost as tool calls became less confused.

## Interpretation

Accuracy is already solid at v0.3.3 (78–100%). The weakness is
prompt-injection: without boundary tags, most models follow at least half the
injected instructions — and v0.3.4 is the version to prefer, since it adds the
boundary-tag defense.

## Residual risks

- Without boundary tags, most models fail the majority of injection scenarios;
  v0.3.3 should not drive write-capable or automation tools on
  attacker-reachable data using anything but the strongest model.
- The strongest model on injection here (`claude-sonnet-4.6`, 15/15) is also by
  far the costliest to run.
- Provider-side regressions between runs are not detected (see ADR "what we do
  not test").
