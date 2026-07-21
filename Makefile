BUILD_DATE := $(shell date -u +'%Y-%m-%dT%H:%M:%SZ')
GIT_COMMIT=$(shell git rev-parse HEAD)
VERSION=$(shell git describe --tags 2> /dev/null || echo "v0.0.0-${GIT_COMMIT}")
GO_IMAGE := golang:1.26.5-alpine
# The race detector requires cgo (a C toolchain), which the Alpine GO_IMAGE lacks.
# GO_IMAGE_CGO is the Debian-based image used by `make test-race`; it ships gcc, so
# `-race` builds there with CGO_ENABLED=1.
GO_IMAGE_CGO := golang:1.26.5
# jq and yq run the help-xml pipeline (JSON assembly + JSON→XML). Pinned by digest
# so `make help-xml` needs no host jq/yq — everything stays in Docker.
JQ_IMAGE := ghcr.io/jqlang/jq@sha256:b9c68867e5766576263a222e91db3de422d802069c7af70440e667a95344e486
YQ_IMAGE := mikefarah/yq@sha256:11a1f0b604b13dbbdc662260d8db6f644b22d8553122a25c1b5b2e8713ca6977
# Bind mount of the working tree at /app, used by nearly every `docker run` below.
# Hoisted into a variable so the literal `$(CURDIR):/app` never appears on a recipe
# line: checkmake's parser reparses such a line as a target (the `:` reads as the
# rule separator) when it directly follows an `ifeq (…)` conditional, a false
# positive that would otherwise trip the phonydeclared / uniquetargets rules.
MOUNT_APP := $(CURDIR):/app
# All Go work runs inside the $(GO_IMAGE) container — there is NO host Go install
# (see install-dev-deps). So every cache is a Docker named volume, never a host
# path: named volumes are linux-native, populated in-container, and — unlike a
# bind mount of a host path — work identically whether `make` runs on the host
# or itself inside a container talking to the same daemon (docker-in-docker),
# where a host path like $(HOME)/.cache would be meaningless to the daemon.
# Module cache (downloaded source) and build cache (compiled objects, hash-keyed
# by GOOS/GOARCH) persist across runs. go/bin is deliberately NOT shared with the
# host: it holds arch-specific executables and would shadow tools an image bakes
# into /go/bin (e.g. golangci-lint); it gets its own volume via GO_TOOLS_CACHE.
# GO_CACHE_DIR: opt-in host directory backing the Go caches, for CI where a fresh
# runner starts with empty named volumes (so every job re-downloads modules,
# recompiles the test binary, and re-`go install`s gotestsum — ~2min/job). Set it
# to a host path that actions/cache restores/saves (keyed on go.sum) and the three
# caches become bind mounts of $(GO_CACHE_DIR)/{gomod,gobuild,go-tools} instead of
# named volumes — the daemon on a hosted runner shares the host filesystem, so the
# docker-in-docker caveat above does not apply there. Unset (local default) keeps
# named volumes, whose whole point is to survive across local runs anyway.
#
# In this mode the container also runs as the host uid (GO_RUN_AS_HOST_UID) so the
# cache files it writes are owned by the runner user, not root — otherwise
# actions/cache (which saves as the runner user) cannot read them. As a non-root
# uid several defaults break and are pinned here:
#   HOME=/tmp        — default HOME (/) is unwritable, and `go` writes there.
#   GOCACHE, GOMODCACHE — point at the mounted, runner-owned cache dirs (the
#                    non-root default $HOME/.cache/go-build is not the mount).
#   GOPATH=/go       — keep the image's GOPATH; only its mod/bin subdirs are mounted.
#   GOSUMDB=off      — `go install` otherwise writes the sumdb cache under
#                    /go/pkg/sumdb, but /go/pkg (parent of the mounted /go/pkg/mod)
#                    is root-owned and unwritable by the host uid. Safe: go.sum
#                    still verifies module content locally; this only skips the
#                    remote checksum-db lookup.
#   GOTOOLCHAIN=local — never auto-download the go.mod `toolchain` version; use the
#                    image's Go. Non-root can't verify a toolchain module under
#                    GOSUMDB=off (it errors), and it matters for the golangci image,
#                    whose Go differs from go.mod's toolchain.
# GOLANGCI_LINT_CACHE moves golangci's analysis cache to a mounted, runner-owned
# dir too (its default /root/.cache is unwritable by the host uid).
ifdef GO_CACHE_DIR
DOCKER_CACHE_MOUNTS := -v $(GO_CACHE_DIR)/gomod:/go/pkg/mod -v $(GO_CACHE_DIR)/gobuild:/go/cache/go-build
GO_TOOLS_CACHE := -v $(GO_CACHE_DIR)/go-tools:/go/bin
GO_RUN_AS_HOST_UID := --user $(shell id -u):$(shell id -g) -e HOME=/tmp -e GOCACHE=/go/cache/go-build -e GOMODCACHE=/go/pkg/mod -e GOPATH=/go -e GOSUMDB=off -e GOTOOLCHAIN=local
GOLANGCI_CACHE := -v $(GO_CACHE_DIR)/golangci:/go/cache/golangci -e GOLANGCI_LINT_CACHE=/go/cache/golangci
else
DOCKER_CACHE_MOUNTS := -v thehivemcp-gomod:/go/pkg/mod -v thehivemcp-gobuild:/root/.cache/go-build
# Named volume (linux-native, populated in-container) for go-installed tools.
GO_TOOLS_CACHE := -v thehivemcp-go-tools:/go/bin
GO_RUN_AS_HOST_UID :=
# Named volume for golangci-lint's analysis cache (same rationale as above).
GOLANGCI_CACHE := -v thehivemcp-golangci-cache:/root/.cache/golangci-lint
endif
# Git worktree support for tools that shell out to git inside a container
# (golangci-lint, gitleaks). In a linked worktree, $(CURDIR)/.git is a FILE
# pointing at the main repo's .git/worktrees/<name> via an ABSOLUTE path, which
# in turn references the common .git (objects) — both OUTSIDE $(CURDIR). The
# working tree is mounted at /app via $(CURDIR), but that absolute .git pointer
# is not, so git in the sibling container fails with "not a git repository".
# Bind-mount the common .git at its real host path so the pointer resolves.
# In a normal checkout the common dir is $(CURDIR)/.git — already under /app —
# so this expands to empty and adds no mount. Read-only: these tools only read.
GIT_COMMON_DIR := $(shell git rev-parse --path-format=absolute --git-common-dir 2>/dev/null)
ifeq ($(filter $(CURDIR)/.git,$(GIT_COMMON_DIR)),)
GIT_WORKTREE_MOUNT := $(if $(GIT_COMMON_DIR),-v $(GIT_COMMON_DIR):$(GIT_COMMON_DIR):ro)
else
GIT_WORKTREE_MOUNT :=
endif
GOLDFLAGS := -ldflags="-s -w -X 'github.com/StrangeBeeCorp/TheHiveMCP/version.buildDate=${BUILD_DATE}' -X 'github.com/StrangeBeeCorp/TheHiveMCP/version.gitCommit=${GIT_COMMIT}' -X 'github.com/StrangeBeeCorp/TheHiveMCP/version.gitVersion=${VERSION}'"
BUILDDIR := ./build
DISTDIR := ./dist
BINARY_NAME := thehivemcp
BGreen="\033[1;32m"       # Green
Color_Off="\033[0m"       # Text Reset

# Release matrix
RELEASE_TARGETS := linux-amd64 linux-arm64 darwin-amd64 darwin-arm64 windows-amd64 windows-arm64

# Windows targets need a .exe suffix on the produced binary. bin_ext returns
# ".exe" for any windows-* target and "" otherwise, so the rest of the build /
# packaging logic can stay platform-agnostic.
bin_ext = $(if $(filter windows-%,$(1)),.exe,)

.PHONY: all
all: fmt lint security test build ## Full local gate before pushing: fmt, lint, security, test, build ##@ make all

.PHONY: lint-makefile
lint-makefile: ## Check the Makefile's syntax and hygiene (checkmake) ##@ make lint-makefile
	@echo $(BGreen)----------------------$(Color_Off)
	@echo $(BGreen)-- Linting Makefile --$(Color_Off)
	@echo $(BGreen)----------------------$(Color_Off)
	@make --dry-run -n all > /dev/null || exit 1
	@echo "Syntax OK"
	@./scripts/lint-makefile.sh Makefile || exit 1
	@echo "checkmake OK"

.PHONY: fmt
fmt: ## Auto-format the whole repo (Go, shell, markdown) before committing ##@ make fmt
	@echo $(BGreen)-------------$(Color_Off)
	@echo $(BGreen)--- Format --$(Color_Off)
	@echo $(BGreen)-------------$(Color_Off)
	./scripts/fmt.sh --all --fix

.PHONY: fmt-changed
fmt-changed: ## Fast format loop while iterating: only git-changed files ##@ make fmt-changed
	@echo $(BGreen)-------------$(Color_Off)
	@echo $(BGreen)--- Format --$(Color_Off)
	@echo $(BGreen)-------------$(Color_Off)
	./scripts/fmt.sh --changed --fix

.PHONY: security
security: vulncheck sast dockerlint dockersec secrets ## Run every security check at once (vuln, SAST, docker, secrets) ##@ make security

# Help system. Each documented target carries a `## <purpose>` comment and an
# optional `##@ <usage>` marker on the same line, e.g.
#   test: ## Run fast unit tests ##@ make test [COVERAGE=1]
# HELP_PARSE turns those grepped lines into TAB-separated `target<TAB>purpose
# <TAB>usage` records (no formatting), so both `help` (pretty table) and
# `help-json` (machine-readable) render from the exact same source of truth.
# Targets with no `##@` marker fall back to `make <target>` as the usage.
# No leading `@`: recipes prefix their own (help/help-json) so this stays pure
# command text usable inside `$$(...)` (help-xml captures it to guard on empty).
define HELP_PARSE
	grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk ' \
	{ \
		target = $$0; sub(/:.*/, "", target); \
		rest = $$0; sub(/^[^#]*## /, "", rest); \
		usage = "make " target; purpose = rest; \
		idx = index(rest, "##@ "); \
		if (idx > 0) { usage = substr(rest, idx + 4); purpose = substr(rest, 1, idx - 1); sub(/[ \t]+$$/, "", purpose); } \
		printf "%s\t%s\t%s\n", target, purpose, usage; \
	}'
endef

# HELP_TABLE renders TAB-separated records as an aligned 3-column table. It
# buffers every row to size the TARGET and PURPOSE columns to their longest
# entry (auto-fit — no truncation when the terminal is wide enough), but caps
# PURPOSE so the USAGE column still fits in COLUMNS, trimming with `…` only when
# the terminal is too narrow. Column separator is two spaces.
define HELP_TABLE
	awk -F'\t' -v cols="$${COLUMNS:-0}" ' \
	{ t[NR]=$$1; p[NR]=$$2; u[NR]=$$3; \
	  if (length($$1)>tw) tw=length($$1); if (length($$2)>pw) pw=length($$2); if (length($$3)>uw) uw=length($$3); } \
	END { \
		if (tw<6) tw=6; \
		hdr_pw=pw; if (hdr_pw<7) hdr_pw=7; \
		if (cols>0) { avail=cols-tw-uw-4; if (avail<10) avail=10; if (pw>avail) pw=avail; } \
		if (pw<hdr_pw && cols<=0) pw=hdr_pw; \
		printf "\033[1;36m%-*s\033[0m  \033[1m%-*s\033[0m  %s\n", tw, "TARGET", pw, "PURPOSE", "USAGE"; \
		for (i=1;i<=NR;i++) { \
			pp=p[i]; if (length(pp)>pw) pp=substr(pp,1,pw-1) "…"; \
			printf "\033[1;36m%-*s\033[0m  %-*s  %s\n", tw, t[i], pw, pp, u[i]; \
		} \
	}'
endef

# HELP_JSON renders the records as a JSON array of {target,purpose,usage}.
# json_escape guards the few chars that would break JSON in these short strings.
define HELP_JSON
	awk -F'\t' ' \
	function esc(s) { gsub(/\\/,"\\\\",s); gsub(/"/,"\\\"",s); gsub(/\t/,"\\t",s); return s; } \
	BEGIN { printf "[" } \
	{ printf "%s\n  {\"target\":\"%s\",\"purpose\":\"%s\",\"usage\":\"%s\"}", (NR>1?",":""), esc($$1), esc($$2), esc($$3) } \
	END { printf "%s]\n", (NR>0?"\n":"") }'
endef

.PHONY: help
help: ## Display this help as a table ##@ make help
	@$(HELP_PARSE) | $(HELP_TABLE)

.PHONY: help-json
help-json: ## Display this help as JSON (for agents/tooling) ##@ make help-json
	@$(HELP_PARSE) | $(HELP_JSON)

.PHONY: help-xml
# jq/yq run in pinned containers so this stays fully in Docker (no host deps).
# Guard against an empty parse: "[]" means no documented targets were matched,
# which would otherwise emit a near-empty doc with exit 0 and look like success.
# The stages run one at a time through captured variables (not a single `|`
# pipe) so a jq/yq/docker failure aborts under `set -e` instead of a broken
# stage silently emitting empty XML with the last stage's exit 0. `pipefail` is
# avoided on purpose — recipes run under /bin/sh, which is dash on many hosts.
# Each record is wrapped in <command> (not <target>) so the inner <target> name
# field stays unambiguous: `//target` selects only the names, never the wrapper.
help-xml: ## Display this help as XML (for agents/tooling) ##@ make help-xml
	@set -e; \
		json=$$($(HELP_PARSE) | $(HELP_JSON)); \
		case "$$json" in ""|"[]") echo "help-xml: no documented targets found" >&2; exit 1;; esac; \
		wrapped=$$(printf '%s' "$$json" | docker run -i --rm $(JQ_IMAGE) '{targets: {command: .}}'); \
		printf '%s' "$$wrapped" | docker run -i --rm $(YQ_IMAGE) -p=json -o=xml

.PHONY: clean
clean: ## Wipe build/dist artifacts and coverage.out for a clean slate ##@ make clean
	@echo $(BGreen)-------------------$(Color_Off)
	@echo $(BGreen)-- Cleaning up   --$(Color_Off)
	@echo $(BGreen)-------------------$(Color_Off)
	@rm -rf $(BUILDDIR)
	@rm -rf $(DISTDIR)
	@rm -f coverage.out

.PHONY: pre
pre: ## Internal: ensure the build/ directory exists (a build dependency) ##@ make pre (usually run automatically)
	@mkdir -p $(BUILDDIR)

.PHONY: sast
sast: ## Find insecure code patterns via SAST (gosec) ##@ make sast
	@echo $(BGreen)---------------------------$(Color_Off)
	@echo $(BGreen)-- Running SAST Analysis --$(Color_Off)
	@echo $(BGreen)---------------------------$(Color_Off)
	docker run -i --rm -v $(MOUNT_APP) -w /app $(GO_RUN_AS_HOST_UID) $(DOCKER_CACHE_MOUNTS) $(GO_TOOLS_CACHE) $(GO_IMAGE) sh -c 'go install github.com/securego/gosec/v2/cmd/gosec@v2.26.1 && gosec ./...'

# Every Dockerfile in the repo. hadolint lints each — the production image plus
# the MCPB and LibreChat helper images — so a misconfig regression in any of them
# turns the build red, not just the one we ship.
DOCKERFILES := deployment/Dockerfile scripts/Dockerfile.mcpb docs/how-to/docker/Dockerfile.librechat

.PHONY: dockerlint
dockerlint: ## Catch Dockerfile hygiene issues in all images (hadolint) ##@ make dockerlint
	@echo $(BGreen)---------------------------$(Color_Off)
	@echo $(BGreen)-- Linting Dockerfiles   --$(Color_Off)
	@echo $(BGreen)---------------------------$(Color_Off)
	@for f in $(DOCKERFILES); do \
		echo "hadolint $$f"; \
		docker run --rm -i hadolint/hadolint:v2.12.0 hadolint - < $$f || exit 1; \
	done
	@echo "Dockerfiles OK"

# hadolint checks Dockerfile hygiene but has no rule for "container runs as
# root"; trivy config does (DS-0002), matching SonarCloud's docker:S6471. We scan
# each Dockerfile for HIGH+ misconfigs only: the LOW findings here are just the
# missing-HEALTHCHECK check (DS-0026), which is noise for a short-lived CI packer
# and a thin config-overlay image. The named volume caches trivy's checks bundle
# (a per-run network download otherwise); like GO_TOOLS_CACHE it is linux-native
# and populated in-container, so it is not bind-mounted from the host.
TRIVY_CACHE := -v thehivemcp-trivy-cache:/root/.cache/trivy
.PHONY: dockersec
dockersec: ## Scan Dockerfiles for HIGH+ security misconfigs (trivy) ##@ make dockersec
	@echo $(BGreen)---------------------------$(Color_Off)
	@echo $(BGreen)-- Scanning Dockerfiles  --$(Color_Off)
	@echo $(BGreen)---------------------------$(Color_Off)
	@for f in $(DOCKERFILES); do \
		echo "trivy config $$f"; \
		docker run --rm -v $(CURDIR):/scan -w /scan $(TRIVY_CACHE) aquasec/trivy:0.72.0 \
			config --quiet --exit-code 1 --severity HIGH,CRITICAL "$$f" || exit 1; \
	done
	@echo "Dockerfiles security OK"

.PHONY: secrets
secrets: ## Detect committed secrets in the working tree (gitleaks) ##@ make secrets
	@echo $(BGreen)---------------------------$(Color_Off)
	@echo $(BGreen)-- Scanning for secrets  --$(Color_Off)
	@echo $(BGreen)---------------------------$(Color_Off)
	docker run --rm -v $(CURDIR):/repo -w /repo $(GIT_WORKTREE_MOUNT) zricethezav/gitleaks:v8.30.1 dir --redact --verbose --config /repo/.gitleaks.toml .

.PHONY: build
build: ## Build the binary for your current host OS/arch ##@ make build (output: build/thehivemcp-<os>-<arch>)
	@echo $(BGreen)----------------------------$(Color_Off)
	@echo $(BGreen)-- Building TheHive MCP --$(Color_Off)
	@echo $(BGreen)----------------------------$(Color_Off)
	@HOST_OS=$$(uname -s | tr '[:upper:]' '[:lower:]'); \
	HOST_ARCH=$$(uname -m); \
	if [ "$$HOST_ARCH" = "x86_64" ]; then HOST_ARCH="amd64"; fi; \
	if [ "$$HOST_ARCH" = "aarch64" ]; then HOST_ARCH="arm64"; fi; \
	$(MAKE) build-$$HOST_OS-$$HOST_ARCH

# Coverage is opt-in via COVERAGE=1. The unit-test (-short) runs skip the
# integration tests (which need the docker-compose stack), so results are cached
# by `go test` and a second consecutive `make test` is near-instant.
ifeq ($(COVERAGE),1)
GO_TEST_COVER := -coverprofile=coverage.out -covermode=atomic
endif

# RUN=<regexp> restricts a test run to matching tests (passed through as `go test
# -run`). e.g. `make test-integration RUN=TestSearchAdditionalQueriesSimilar`.
ifdef RUN
GO_TEST_RUN := -run $(RUN)
endif

# Where the integration suite reaches the compose stack. Override to run against
# an external TheHive, e.g. `make test-integration THEHIVE_TEST_URL=http://host:9000`.
THEHIVE_TEST_URL ?= http://localhost:9000
export THEHIVE_TEST_URL

# THEHIVE_TEST_IMAGE reaches the reset script (which derives the licensing tag
# from it) and the go-test container; export it so both sub-shells see it.
export THEHIVE_TEST_IMAGE

# Integration mode is decided at run time by scripts/reset-integration-db.sh,
# highest-priority first (see that script + the recipe below):
#   1. THEHIVE_TEST_LICENSE set ⇒ use that token directly (no probe, no mint) ⇒
#      parallel one-org-per-test. Supply it as a CI secret or a local export to
#      get parallel mode without needing the licensing image to be pullable.
#   2. else the StrangeBee licensing image is pullable ⇒ mint a dev license ⇒
#      parallel one-org-per-test.
#   3. else free license ⇒ sequential shared-org.
# In cases 1-2 the script materialises the token to this file, which the recipe
# reads to set THEHIVE_TEST_LICENSE and the go-test parallelism for the go-test
# container. Export it so the reset script (a separate sub-shell) sees case 1.
export THEHIVE_TEST_LICENSE
LICENSE_FILE := internal/testutils/testdata/.test-license.lic.local

.PHONY: test
test: pre ## Default test loop: fast, cached unit tests (no live TheHive) ##@ make test [COVERAGE=1] [RUN=<regexp>]
	@echo $(BGreen)-----------------------$(Color_Off)
	@echo $(BGreen)-- Running UnitTests --$(Color_Off)
	@echo $(BGreen)-----------------------$(Color_Off)
	docker run -i --rm -v $(MOUNT_APP) -w /app $(DOCKER_CACHE_MOUNTS) $(GO_IMAGE) go test $(GO_TEST_COVER) $(GO_TEST_RUN) -short -v ./...
ifeq ($(COVERAGE),1)
	docker run -i --rm -v $(MOUNT_APP) -w /app $(DOCKER_CACHE_MOUNTS) $(GO_IMAGE) go tool cover -func=coverage.out
endif

# test-race runs -short only: the race detector is meaningful on our own
# concurrency (parallel tests, fan-out helpers), not on the live SDK/HTTP client
# the integration suite drives, and a -race integration run would be far slower.
# It uses GO_IMAGE_CGO with CGO_ENABLED=1 because -race needs cgo (the Alpine
# GO_IMAGE has no C toolchain).
.PHONY: test-race
test-race: pre ## Catch data races in our concurrency (unit tests, -race) ##@ make test-race [RUN=<regexp>]
	@echo $(BGreen)-- Running UnitTests under -race --$(Color_Off)
	docker run -i --rm -v $(MOUNT_APP) -w /app -e CGO_ENABLED=1 $(DOCKER_CACHE_MOUNTS) $(GO_IMAGE_CGO) go test -race $(GO_TEST_RUN) -short ./...

.PHONY: test-integration
test-integration: pre ## Full suite vs a live TheHive stack; slow, leaves stack up ##@ make test-integration [COVERAGE=1] [RUN=<regexp>] [THEHIVE_TEST_IMAGE=..] [THEHIVE_TEST_URL=..]; then make test-integration-down
	@echo $(BGreen)------------------------------$(Color_Off)
	@echo $(BGreen)-- Running Integration Tests --$(Color_Off)
	@echo $(BGreen)------------------------------$(Color_Off)
	@LICENSE_FILE="$(LICENSE_FILE)" MOUNT_APP="$(MOUNT_APP)" GO_IMAGE="$(GO_IMAGE)" \
		GO_RUN_AS_HOST_UID="$(GO_RUN_AS_HOST_UID)" DOCKER_CACHE_MOUNTS="$(DOCKER_CACHE_MOUNTS)" \
		GO_TOOLS_CACHE="$(GO_TOOLS_CACHE)" GO_TEST_COVER="$(GO_TEST_COVER)" GO_TEST_RUN="$(GO_TEST_RUN)" \
		./scripts/run-integration-tests.sh
ifeq ($(COVERAGE),1)
	docker run -i --rm -v $(MOUNT_APP) -w /app $(DOCKER_CACHE_MOUNTS) $(GO_IMAGE) go tool cover -func=coverage.out
endif

.PHONY: test-integration-down
test-integration-down: ## Tear down the stack left up by test-integration ##@ make test-integration-down
	docker compose -f docker-compose.test.yml down -v

.PHONY: docker-build
docker-build: ## Build the production Docker image (thehivemcp:latest) ##@ make docker-build
	@echo $(BGreen)------------------------------$(Color_Off)
	@echo $(BGreen)-- Building Docker Image --$(Color_Off)
	@echo $(BGreen)------------------------------$(Color_Off)
	export DOCKER_BUILDKIT=1; \
	docker build \
		--build-arg BUILD_DATE=${BUILD_DATE} \
		--build-arg GIT_COMMIT=${GIT_COMMIT} \
		--build-arg VERSION=${VERSION} \
		-t ${BINARY_NAME}:latest \
		-f deployment/Dockerfile .

.PHONY: docker-run
docker-run: docker-build ## Build + run the prod container using your .env config ##@ make docker-run [ARGS=".."] (needs a .env file)
	@echo $(BGreen)--------------------------------$(Color_Off)
	@echo $(BGreen)-- Starting production mode --$(Color_Off)
	@echo $(BGreen)--------------------------------$(Color_Off)
	@set -a; \
	[ -f .env ] && . ./.env; \
	docker run \
		--name ${BINARY_NAME} \
		-p $${MCP_PORT:-8082}:$${MCP_PORT:-8082} \
		--env-file .env \
		${BINARY_NAME}:latest $(ARGS)

.PHONY: dev
dev: ## Iterate locally: hot-reloading dev server (air) ##@ make dev
	@echo $(BGreen)--------------------------------$(Color_Off)
	@echo $(BGreen)-- Starting development mode --$(Color_Off)
	@echo $(BGreen)--------------------------------$(Color_Off)
	@if ! command -v air >/dev/null 2>&1; then \
		echo "Installing air for hot reload..."; \
		go install github.com/air-verse/air@latest; \
	fi; \
	air

.PHONY: run
run: build ## Build then run the binary from the host ##@ make run ARGS="your arguments here"
	@echo $(BGreen)---------------------------$(Color_Off)
	@echo $(BGreen)-- Starting application  --$(Color_Off)
	@echo $(BGreen)---------------------------$(Color_Off)
	@HOST_OS=$$(uname -s | tr '[:upper:]' '[:lower:]'); \
	HOST_ARCH=$$(uname -m); \
	if [ "$$HOST_ARCH" = "x86_64" ]; then HOST_ARCH="amd64"; fi; \
	if [ "$$HOST_ARCH" = "aarch64" ]; then HOST_ARCH="arm64"; fi; \
	$(BUILDDIR)/$(BINARY_NAME)-$$HOST_OS-$$HOST_ARCH $(ARGS)


.PHONY: vulncheck
vulncheck: ## Check dependencies for known CVEs (govulncheck) ##@ make vulncheck
	@echo $(BGreen)------------------------------$(Color_Off)
	@echo $(BGreen)-- Security Vulnerability  --$(Color_Off)
	@echo $(BGreen)------------------------------$(Color_Off)
	docker run -i --rm -v $(MOUNT_APP) -w /app $(GO_RUN_AS_HOST_UID) $(DOCKER_CACHE_MOUNTS) $(GO_TOOLS_CACHE) $(GO_IMAGE) sh -c 'go install golang.org/x/vuln/cmd/govulncheck@v1.3.0 && govulncheck ./...'

.PHONY: lint
lint: ## Check the whole repo for lint issues, read-only ##@ make lint
	@echo $(BGreen)-----------------------------$(Color_Off)
	@echo $(BGreen)-- Linter Checks --$(Color_Off)
	@echo $(BGreen)-----------------------------$(Color_Off)
	./scripts/lint.sh --all --check

.PHONY: lint-changed
lint-changed: ## Fast read-only lint of git-changed files only ##@ make lint-changed
	@echo $(BGreen)-----------------------------$(Color_Off)
	@echo $(BGreen)-- Linter Checks: changed --$(Color_Off)
	@echo $(BGreen)-----------------------------$(Color_Off)
	./scripts/lint.sh --changed --check

.PHONY: lint-fix
lint-fix: ## Auto-apply safe lint fixes across the whole repo ##@ make lint-fix
	@echo $(BGreen)-----------------------------$(Color_Off)
	@echo $(BGreen)-- Linter Checks with auto-fix --$(Color_Off)
	@echo $(BGreen)-----------------------------$(Color_Off)
	./scripts/lint.sh --all --fix

.PHONY: lint-fix-changed
lint-fix-changed: ## Auto-apply safe lint fixes to git-changed files only ##@ make lint-fix-changed
	@echo $(BGreen)-----------------------------$(Color_Off)
	@echo $(BGreen)-- Linter Checks: auto-fix changed --$(Color_Off)
	@echo $(BGreen)-----------------------------$(Color_Off)
	./scripts/lint.sh --changed --fix

.PHONY: updatedep
updatedep: ## Upgrade Go deps to latest and tidy go.mod/go.sum ##@ make updatedep
	@echo $(BGreen)-----------------------$(Color_Off)
	@echo $(BGreen)-- Update Dependencies --$(Color_Off)
	@echo $(BGreen)-----------------------$(Color_Off)
	docker run -i --rm -v $(MOUNT_APP) -w /app $(DOCKER_CACHE_MOUNTS) $(GO_IMAGE) sh -c 'go get -u ./... && go mod tidy'

.PHONY: install-dev-deps
install-dev-deps: ## No-op: tooling runs on-demand via Docker (kept for habit) ##@ make install-dev-deps
	@echo $(BGreen)----------------------------------------$(Color_Off)
	@echo $(BGreen)-- Installing development dependencies --$(Color_Off)
	@echo $(BGreen)----------------------------------------$(Color_Off)
	@echo "Dependencies are now installed on-demand via Docker containers"
	@echo "No local Go installation required"

# Dynamic build target for any platform in RELEASE_TARGETS
build-%: pre
	@echo "Building for $*..."
	@OS=$$(echo $* | cut -d- -f1); \
	ARCH=$$(echo $* | cut -d- -f2); \
	docker run -i --rm -v $(MOUNT_APP) -w /app -e GOOS=$$OS -e GOARCH=$$ARCH $(DOCKER_CACHE_MOUNTS) $(GO_IMAGE) go build $(GOLDFLAGS) -o $(BUILDDIR)/$(BINARY_NAME)-$*$(call bin_ext,$*) ./cmd/server/main.go

.PHONY: pre-dist
pre-dist: ## Internal: ensure the dist/ directory exists (packaging dep) ##@ make pre-dist (usually run automatically)
	@mkdir -p $(DISTDIR)

.PHONY: build-current
build-current: build ## Alias for `build` (current host platform) ##@ make build-current

.PHONY: build-all
build-all: $(addprefix build-,$(RELEASE_TARGETS)) ## Cross-compile binaries for every release target ##@ make build-all

.PHONY: package-release
package-release: build-all package-from-built ## Local release dry-run: build all + package tarballs/MCPB (no signing) ##@ make package-release

# package-from-built packages whatever binaries already exist in $(BUILDDIR) — it
# does NOT (re)build. This is the seam the release CI needs: build-all -> sign the
# windows .exe in place -> package-from-built, so packaging consumes the *signed*
# binaries instead of clobbering them with a fresh build-all.
.PHONY: package-from-built
package-from-built: pre-dist mcpb-ci-nobuild ## Release CI: package existing (signed) build/ binaries, no rebuild ##@ make package-from-built
	@echo $(BGreen)--------------------------------$(Color_Off)
	@echo $(BGreen)-- Packaging release binaries --$(Color_Off)
	@echo $(BGreen)--------------------------------$(Color_Off)
	@VERSION="$(VERSION)" BUILDDIR="$(BUILDDIR)" DISTDIR="$(DISTDIR)" \
		BINARY_NAME="$(BINARY_NAME)" RELEASE_TARGETS="$(RELEASE_TARGETS)" \
		./scripts/package-tarballs.sh

.PHONY: version
version: ## Print the version string baked into builds ##@ make version
	@echo $(VERSION)

.PHONY: mcpb-build-image
mcpb-build-image: ## Internal: build the MCPB packaging image (dep of mcpb-*) ##@ make mcpb-build-image (usually run automatically)
	@echo $(BGreen)-----------------------------$(Color_Off)
	@echo $(BGreen)-- Building MCPB CI Image  --$(Color_Off)
	@echo $(BGreen)-----------------------------$(Color_Off)
	docker build -f scripts/Dockerfile.mcpb -t thehivemcp-mcpb:latest .

.PHONY: mcpb-local
mcpb-local: build ## Build + package a single MCPB for your host, to test it ##@ make mcpb-local
	@echo $(BGreen)------------------------------$(Color_Off)
	@echo $(BGreen)-- Generating MCPB Package --$(Color_Off)
	@echo $(BGreen)------------------------------$(Color_Off)
	./scripts/generate-mcpb.sh

# The set of targets mcpb-ci-nobuild packages. Defaults to every release
# target; override it to a subset for a fast packaging smoke-test (see
# mcpb-smoke and the ci.yml mcpb-package job).
MCPB_TARGETS ?= $(RELEASE_TARGETS)

.PHONY: mcpb-ci
mcpb-ci: build-all mcpb-ci-nobuild ## Build all + package MCPB for every architecture ##@ make mcpb-ci

# mcpb-smoke exercises the MCPB packaging path (container permissions, manifest
# generation, non-empty output) for a single target — fast enough to run on
# every PR that touches the packaging inputs. Reuses build-<target> +
# mcpb-ci-nobuild with MCPB_TARGETS narrowed to one arch.
.PHONY: mcpb-smoke
mcpb-smoke: ## Fast PR check: MCPB packaging works for one arch ##@ make mcpb-smoke [MCPB_SMOKE_TARGET=linux-amd64]
	@$(MAKE) build-$(MCPB_SMOKE_TARGET)
	@$(MAKE) mcpb-ci-nobuild MCPB_TARGETS=$(MCPB_SMOKE_TARGET)
MCPB_SMOKE_TARGET ?= linux-amd64

# mcpb-ci-nobuild wraps the already-built (and, in release CI, already-signed)
# binaries in $(BUILDDIR) into MCPB packages. It deliberately does NOT depend on
# build-all so a signing step can run between build-all and packaging.
.PHONY: mcpb-ci-nobuild
mcpb-ci-nobuild: pre-dist mcpb-build-image ## Release CI: MCPB-package existing build/ binaries, no rebuild ##@ make mcpb-ci-nobuild [MCPB_TARGETS="..."]
	@echo $(BGreen)----------------------------------$(Color_Off)
	@echo $(BGreen)-- Generating MCPB Packages CI --$(Color_Off)
	@echo $(BGreen)----------------------------------$(Color_Off)
	@VERSION="$(VERSION)" BUILDDIR="$(BUILDDIR)" DISTDIR="$(DISTDIR)" \
		MCPB_TARGETS="$(MCPB_TARGETS)" \
		./scripts/package-mcpb-ci.sh

# The windows release targets, expanded to their built .exe paths.
WINDOWS_TARGETS := $(filter windows-%,$(RELEASE_TARGETS))
WINDOWS_BINARIES := $(foreach t,$(WINDOWS_TARGETS),$(BUILDDIR)/$(BINARY_NAME)-$(t).exe)

.PHONY: sign-windows
sign-windows: ## Release CI: code-sign the built Windows .exe files in place ##@ make sign-windows
	@echo $(BGreen)-----------------------------$(Color_Off)
	@echo $(BGreen)-- Signing Windows binaries --$(Color_Off)
	@echo $(BGreen)-----------------------------$(Color_Off)
	./scripts/sign-windows.sh $(WINDOWS_BINARIES)
