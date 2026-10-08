# Handle a CVE

A known vulnerability reaches this repository by one of three roads. Each says what to do.

## A Renovate pull request labelled `security`

Renovate reads [OSV](https://osv.dev/) for the Go modules `go.mod` requires directly (`osvVulnerabilityAlerts` in `renovate.json`). When a fixed version exists,
it opens a pull request at once, whatever its schedule, with no three-day wait, labelled `security` and suffixed `[SECURITY]`. The Dependency Dashboard issue
lists the CVEs that have no fixed version yet.

1. Read the advisory the pull request links: what is vulnerable, and whether our code reaches it. `make vulncheck` answers the second question for Go.
2. CI runs every gate on the pull request. A person reviews and merges it, as any other.
3. If our code reached the vulnerable function, cut a patch release (`RELEASING.md`) and say so under `### Security` in `CHANGELOG.md`.

## A failing `vulncheck` gate

`govulncheck` (`make vulncheck`) runs at every pull request, in CI and every night. It fails when a known vulnerability is reachable from our code, through a
direct dependency or an indirect one, and names the call path. At night, the triage files it as an issue labelled `nightly`.

1. Upgrade the module that carries the fix: `go get <module>@<fixed version>`, then `go mod tidy`, on a branch of its own. If the fix is in the Go toolchain
   itself, bump the Go version: the `go` directive and the `golang` images move together (Renovate groups them as `go version`).
2. No fixed version yet: stop calling the vulnerable function if the code can, or replace the module. The gate stays red until then; a known reachable
   vulnerability is not waived.
3. Once fixed and released, cut a patch release as above.

## The images

The server's image is `gcr.io/distroless/static-debian12`, pinned by digest: a static Go binary and almost no system package, so a CVE in a Debian package
rarely reaches it. Renovate updates the digest in its `dependency pins` group, with no wait. `make dockersec` (trivy config, in the `dockerlint` gate) holds the
Dockerfiles to their HIGH and CRITICAL misconfiguration checks at every pull request. When a base image's CVE does matter, merge Renovate's digest pull request,
or bump the digest by hand with `docker buildx imagetools inspect <image>:<tag>`, and cut a patch release.

## Report a vulnerability

Do not open a public issue for a vulnerability in TheHiveMCP itself: `.github/SECURITY.md` says how to report one privately.
