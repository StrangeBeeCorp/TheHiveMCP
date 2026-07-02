---
title: ADR 0002 — Code-signing for TheHiveMCP Windows binaries
type: adr
status: draft
owner: "@antoine"
tags: [adr, thehivemcp, windows, release, signing, security]
created: 2026-06-30
last_reviewed: 2026-06-30
review_cadence: on-change
---

> **Status:** 🟡 draft — pending approval from @mathilde (cost / procurement) before the mechanism is provisioned.\
> **Driver:** @antoine\
> **Motivating work:** [DL-6023](https://strangebee.atlassian.net/browse/DL-6023) (parent epic [DL-5723](https://strangebee.atlassian.net/browse/DL-5723),
> TheHiveMCP beta exit).
>
> **⚠️ Partial reversal — interim unsigned release ([DL-6329](https://strangebee.atlassian.net/browse/DL-6329), decided by @antoine 2026-07-01; scope arbiter
> @mathilde to confirm).** The **"signed-only — no unsigned or self-signed interim release"** policy below (see [Decision outcome](#decision-outcome)) is
> **relaxed** to unblock Windows adoption while the signing identity reviews are still pending. Unsigned windows-amd64/arm64 binaries and `.mcpb` are published
> now with `SIGNING_PROVIDER=none`. **The signed end state is unchanged** — [DL-6301](https://strangebee.atlassian.net/browse/DL-6301) /
> [DL-6304](https://strangebee.atlassian.net/browse/DL-6304) remain open and the interim ship does **not** close them; only the never-ship-unsigned-first
> constraint is lifted. **Accepted risk:** an unsigned `.exe` may be hard-blocked by Smart App Control / WDAC on hardened enterprise Windows (our SOC audience)
> with no "Run anyway" escape — documented for users, not mitigated, in the interim.

---

## Context and problem statement

TheHiveMCP is about to exit beta. As part of that ([DL-6023](https://strangebee.atlassian.net/browse/DL-6023)) we are adding `windows-amd64` / `windows-arm64`
binaries to the release matrix — the server already cross-compiles cleanly for Windows, so this is a packaging + distribution task, not a code change.

A Windows executable downloaded from the internet runs into two Microsoft trust gates:

1. **SmartScreen** — shows an "unrecognized app" prompt on download/run until the file accumulates download reputation. Whether the prompt names a _verified
   publisher_ or warns of an _unknown publisher_ depends on signing.
2. **Smart App Control / WDAC (Windows 11, enterprise-managed)** — can **block execution of unsigned files** outright, with no "Run anyway" escape on hardened
   configurations.

Our audience is SOC analysts and platform engineers — precisely the population most likely to run on hardened, policy-managed Windows. The question: **do we
sign the Windows binaries, and if so, how?** The ticket title ("signed Windows binaries") states the intent; this ADR records the decision and, critically, the
_mechanism_, because the landscape changed materially in 2023–2024.

The binary is consumed as an **MCP server** — wired into an MCP host config as a `command`, typically delivered inside an `.mcpb` package that Claude Desktop
installs and launches as a child process. So the _download_ warning hits the archive, and the _execution_ gate (Smart App Control / WDAC) is where unsigned
binaries are most likely to be hard-blocked.

## Decision drivers

- **Trust / credibility** — a _security product_ shipping an "unknown publisher" binary is a visible objection during evaluation.
- **Executability on locked-down machines** — must run under Smart App Control / WDAC "signed-only" policies.
- **CI-native** — signing must run unattended in GitHub Actions (`release.yml`); no human plugging in a USB token per release.
- **Time-to-ship** — DL-6023 is on the beta-exit critical path. The dominant cost of every signing option is a third-party identity-validation/onboarding review
  that **cannot be expedited**; predictability of that review matters more than the marginal $/yr.
- **Cost / procurement effort** — lower is better, but secondary to time-to-ship for this decision.
- **EU eligibility** — StrangeBee is an EU (France) organization; the mechanism must be available to us.

## Considered options

1. **Ship unsigned**
2. **Ship self-signed**
3. **Sign properly — Azure Artifact Signing** (formerly Trusted Signing) _(chosen — run in parallel)_
4. **Sign properly — traditional OV certificate** (DigiCert / Sectigo / GlobalSign + cloud HSM)
5. **Sign properly — traditional EV certificate**
6. **Sign properly — SignPath Foundation** (free OV-level signing for OSS) _(chosen — run in parallel)_

### Comparison

|                                         |      Unsigned       |         Self-signed         |                    Azure Artifact Signing ⭐                     |                   OV cert                    |      EV cert       |     SignPath Foundation ⭐      |
| --------------------------------------- | :-----------------: | :-------------------------: | :--------------------------------------------------------------: | :------------------------------------------: | :----------------: | :-----------------------------: |
| **Cost**                                |        Free         |            Free             |                        ~$10/mo (~$120/yr)                        |                 $150–300/yr                  |      $400+/yr      |   **Free** (if OSS-eligible)    |
| **Publisher name shown**                |    ❌ "Unknown"     |        ❌ Untrusted         |                           ✅ Verified                            |                 ✅ Verified                  |    ✅ Verified     |           ✅ Verified           |
| **SmartScreen (first downloads)**       | ⚠️ Unsigned warning |       ⚠️ Strong block       |               ⚠️ Verified-publisher warning, fades               |                   ⚠️ Same                    | ⚠️ Same since 2024 |             ⚠️ Same             |
| **Runs under Smart App Control / WDAC** |  ❌ May be blocked  | ❌ Blocked (untrusted root) |                                ✅                                |                      ✅                      |         ✅         |               ✅                |
| **CI-native (GitHub Actions)**          |         n/a         |           trivial           |                       ✅ Native, no token                        |     🟡 Needs USB token or paid cloud-HSM     |      🟡 Same       |       ✅ Managed pipeline       |
| **HW token required**                   |          —          |              —              |                             ✅ None                              |        ❌ Yes (CA/B Forum, June 2023)        |       ❌ Yes       |             ✅ None             |
| **EU org eligible**                     |          —          |              —              |                              ✅ Yes                              |                 ✅ Worldwide                 |    ✅ Worldwide    |        ✅ (OSS criteria)        |
| **Time to first signed release**        |       instant       |           instant           | 1–20 business days (org IV, can't expedite); rest parallelizable | days–weeks (CA validation) + token logistics |        same        | days–weeks (Foundation review)  |
| **Per-release friction**                |        none         |            none             |                        ✅ Fully automatic                        |                  automatic                   |     automatic      | ⚠️ Manual approval each release |

> **The 2024 SmartScreen change is the key fact:** EV certificates _used_ to bypass SmartScreen on first download — the historical reason to pay the EV premium.
> Microsoft **removed that behavior in 2024**. EV, OV, and Azure now produce **identical** SmartScreen behaviour: a verified-publisher prompt that fades as
> download volume grows. Nothing short of the Microsoft Store buys instant trust, and the Store path doesn't fit a CLI MCP binary.

## Decision outcome

**Chosen: sign the Windows binaries properly, via a proper trusted-root mechanism. Run SignPath Foundation and Azure Artifact Signing onboarding _in parallel_;
ship with whichever clears its review first.**

Both are acceptable end states (free vs ~$120/yr; both give the same SmartScreen behaviour and run under Smart App Control / WDAC). The binding constraint is
**time-to-ship**, and for both the gate is a third-party review that **cannot be expedited** — so the fastest strategy is not to bet on one and wait, but to
**start both reviews on day 0** and take the first to complete:

- **Default preference if both land together → SignPath Foundation** (free, and TheHiveMCP is verified eligible). Accept its manual-per-release approval.
- **Switch to Azure Artifact Signing if** it clears first, or if SignPath's per-release manual approval proves operationally unacceptable. Azure's ~$120/yr is
  immaterial against a missed beta-exit window.

> **Timing reality (Microsoft Learn, 2026-05):** Azure org identity validation is officially **1–20 business days and cannot be expedited** (marketing "as
> little as an hour" assumes public business records cross-reference cleanly with zero discrepancies). The single highest-leverage action is pre-flight
> accuracy: legal entity name, address, website, and business identifier must exactly match StrangeBee's public records before submitting, and the verification
> email link expires in **7 days**. SignPath's Foundation review is comparably "a few days to a few weeks."

**De-risk the critical path:** the `release.yml` signing integration can be built and proven end-to-end against a **Public Trust _Test_ profile** (Azure) or a
sandbox while the real identity reviews are pending — so review-completion is the _only_ thing on the critical path, and shipping is same-day once it clears.

**Policy: signed-only — no unsigned or self-signed interim release.** Windows artifacts are not published until a proper signing path is wired into
`release.yml`.

> **Relaxed for the interim ship (DL-6329, 2026-07-01) — see the reversal banner at the top.** This "no unsigned interim release" constraint is temporarily
> lifted so unsigned binaries can ship now; the signed end state remains the target.

Rationale:

- **Unsigned (1)** and **self-signed (2)** are rejected. Self-signed is _worse than unsigned_ for public distribution — Microsoft treats it the same as no
  signature, and a security vendor shipping a cert that chains to nothing reads as more suspicious, not less. Unsigned risks a hard execution block under Smart
  App Control / WDAC for exactly our enterprise audience. (Neither shortens time-to-ship enough to justify the trust cost, given the integration work is the
  same.)
- **EV (5)** is rejected: since the 2024 change it buys nothing over OV for SmartScreen, costs the most, and still needs a hardware token incompatible with CI.
- **OV cert (4)** is a worldwide fallback only — same SmartScreen result as EV, but the June-2023 HSM/USB-token mandate adds token logistics _on top of_ CA
  validation, making it the **slowest and most friction-heavy** trusted-root path. Use only if both SignPath and Azure are unavailable.
- **Azure Artifact Signing (3)** and **SignPath Foundation (6)** are the two viable paths and are run **concurrently**. Azure: ~$120/yr, fully automatic, no
  token, EU-org eligible. SignPath: free, manual-per-release approval, OSS-eligible (verified). Whichever review completes first wins; default to SignPath on a
  tie.

### Consequences

- **Positive:** Windows users get a verified-publisher binary that runs under hardened policies; the documented `windows-amd64.mcpb` download (already
  advertised in the README) goes live; the beta exit clears a platform gap.
- **Positive:** **$0** if SignPath wins, ~$120/yr if Azure wins — either is immaterial against the beta-exit deadline.
- **Negative / accepted:** first-download SmartScreen warnings still appear and fade over time — **no signing option avoids this anymore**; we will tell early
  adopters to expect it (and prefer the `.mcpb` install path).
- **Negative:** introduces a new secret/identity into CI (SignPath token _or_ Azure credentials) and a signing step in `release.yml` to maintain. Signing must
  slot in **after `build-all`, before `package-release`/`mcpb-ci`** so the signed `.exe` is what gets packaged. The integration is written mechanism-agnostic
  where possible so a late switch between the two is cheap.
- **Negative / accepted (if SignPath wins):** **manual approval per release** — `release.yml` is currently fully automatic on a `v*.*.*` tag; the SignPath step
  submits artifacts and pauses for a human approval in the dashboard. A deliberate supply-chain safeguard, but it changes the release ritual. Azure has no such
  step.
- **Critical-path gate:** the binding cost is the identity review on **both** paths — Azure org IV (1–20 business days, not expeditable) and SignPath's
  Foundation review (days–weeks). Mitigated by starting both on day 0 and building/proving the CI integration against a test profile in parallel, so
  review-completion is the only thing left on the critical path.

## Pros and cons of each option

**1. Unsigned** — ➕ zero cost/effort. ➖ unknown-publisher warning; may be hard-blocked by Smart App Control / WDAC; poor look for a security product.
Rejected.

**2. Self-signed** — ➕ free, trivial in CI. ➖ Microsoft treats it as _no signature_ for public distribution (strong block unless the cert is pre-trusted via
Intune/GPO — internal-only); worse optics than unsigned. Rejected.

**3. Azure Artifact Signing ⭐** — ➕ ~$10/mo, no hardware token, native GitHub Actions, EU-org eligible, same SmartScreen as EV, **fully automatic (no
per-release approval)**. ➖ Azure subscription + Entra tenant prerequisite; org identity validation **1–20 business days, not expeditable**; short cert validity
makes timestamping mandatory. **Chosen — run in parallel with SignPath; wins on a tie only if it clears first or per-release approval is unacceptable.**

**4. OV certificate** — ➕ worldwide, traditional CA relationship, ~$150–300/yr. ➖ June-2023 HSM/token mandate complicates unattended CI (USB token or paid
cloud-HSM). Correct fallback if Azure is unavailable.

**5. EV certificate** — ➕ rigorous identity (may matter for some enterprise procurement). ➖ most expensive ($400+/yr), hardware token, **and no SmartScreen
advantage over OV since 2024**. Rejected.

**6. SignPath Foundation ⭐** — ➕ **free** OV-level signing for qualifying OSS, managed CI pipeline, no token; TheHiveMCP meets every hard criterion (verified
2026-06-30). ➖ manual approval per release; dependency on a third-party program's review (days to weeks to onboard). **Chosen — run in parallel with Azure;
default winner on a tie.**

---

_Sources: [Microsoft Learn — Code signing options](https://learn.microsoft.com/en-us/windows/apps/package-and-deploy/code-signing-options) ·
[Microsoft Learn — SmartScreen reputation](https://learn.microsoft.com/en-us/windows/apps/package-and-deploy/smartscreen-reputation) (both reviewed
2026-06-30)._
