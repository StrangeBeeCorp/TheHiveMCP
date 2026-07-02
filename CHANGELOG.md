# Changelog

All notable changes to TheHiveMCP are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and per
[RELEASING.md](RELEASING.md) each release sorts its changes into user-facing,
breaking, and security categories.

## [Unreleased]

### User-facing changes

- **TheHiveMCP is now production-ready and out of beta.** The beta warning has
  been removed from the README, the docs site, and the install/example guides.
  The README now carries a **"Production-ready: what we commit to"** section
  stating the supported deployment shape, the recommended-models constraint, the
  security and accuracy envelopes we vouch for (and their boundaries), and what
  we explicitly do not commit to (arbitrary models, untested workloads,
  latency/throughput SLAs).
- Published **accuracy and security evaluation evidence** in
  [`docs/evaluation/`](docs/evaluation/), with per-model results tied to a named
  MCP-server version. The commitment shape and testing policy are recorded in
  [ADR-0001](docs/adr/0001-security-and-accuracy-testing-policy.md); the re-test
  policy that keeps the evidence current is in [RELEASING.md](RELEASING.md).

### Security fixes

- None in this entry. The shipped prompt-injection defense (`[UNTRUSTED_DATA]`
  boundary tags on all user-generated fields) and its measured resilience per
  recommended model are documented in [`docs/evaluation/`](docs/evaluation/).
