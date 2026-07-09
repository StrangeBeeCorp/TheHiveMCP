BUILD_DATE := $(shell date -u +'%Y-%m-%dT%H:%M:%SZ')
GIT_COMMIT=$(shell git rev-parse HEAD)
VERSION=$(shell git describe --tags 2> /dev/null || echo "v0.0.0-${GIT_COMMIT}")
GO_IMAGE := golang:1.26.5-alpine
# The race detector requires cgo (a C toolchain), which the Alpine GO_IMAGE lacks.
# GO_IMAGE_CGO is the Debian-based image used by `make test-race`; it ships gcc, so
# `-race` builds there with CGO_ENABLED=1.
GO_IMAGE_CGO := golang:1.26.5
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
DOCKER_CACHE_MOUNTS := -v thehivemcp-gomod:/go/pkg/mod -v thehivemcp-gobuild:/root/.cache/go-build
# Named volume (linux-native, populated in-container) for go-installed tools.
GO_TOOLS_CACHE := -v thehivemcp-go-tools:/go/bin
# Named volume for golangci-lint's analysis cache (same rationale as above).
GOLANGCI_CACHE := -v thehivemcp-golangci-cache:/root/.cache/golangci-lint
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
all: fmt security test build ## Format, run security checks, test, and build

.PHONY: lint-makefile
lint-makefile: ## Lint the Makefile
	@echo $(BGreen)----------------------$(Color_Off)
	@echo $(BGreen)-- Linting Makefile --$(Color_Off)
	@echo $(BGreen)----------------------$(Color_Off)
	@make --dry-run -n all > /dev/null || exit 1
	@echo "Syntax OK"
	@docker run --rm --workdir / -v $(CURDIR)/Makefile:/Makefile -v $(CURDIR)/checkmake.ini:/checkmake.ini quay.io/checkmake/checkmake:latest || exit 1
	@echo "checkmake OK"

.PHONY: fmt
fmt: ## Format the code
	@echo $(BGreen)-------------$(Color_Off)
	@echo $(BGreen)--- Format --$(Color_Off)
	@echo $(BGreen)-------------$(Color_Off)
	docker run -i --rm -v $(CURDIR):/app -w /app $(DOCKER_CACHE_MOUNTS) $(GO_IMAGE) go fmt ./...
	@echo "Code formatted"

.PHONY: security
security: vulncheck sast lint dockerlint dockersec secrets ## Run security checks

.PHONY: help
help: ## Display this help
	@echo $(BGreen)--------------$(Color_Off)
	@echo $(BGreen)-- Help : --$(Color_Off)
	@echo $(BGreen)--------------$(Color_Off)
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[1;36m%-30s\033[0m %s\n", $$1, $$2}'

.PHONY: clean
clean: ## Remove build artifacts and coverage files
	@echo $(BGreen)-------------------$(Color_Off)
	@echo $(BGreen)-- Cleaning up   --$(Color_Off)
	@echo $(BGreen)-------------------$(Color_Off)
	@rm -rf $(BUILDDIR)
	@rm -rf $(DISTDIR)
	@rm -f coverage.out

.PHONY: pre
pre: ## Create build directory
	@mkdir -p $(BUILDDIR)

.PHONY: sast
sast: ## Static Application Security Testing
	@echo $(BGreen)---------------------------$(Color_Off)
	@echo $(BGreen)-- Running SAST Analysis --$(Color_Off)
	@echo $(BGreen)---------------------------$(Color_Off)
	docker run -i --rm -v $(CURDIR):/app -w /app $(DOCKER_CACHE_MOUNTS) $(GO_TOOLS_CACHE) $(GO_IMAGE) sh -c 'go install github.com/securego/gosec/v2/cmd/gosec@v2.26.1 && gosec ./...'

# Every Dockerfile in the repo. hadolint lints each — the production image plus
# the MCPB and LibreChat helper images — so a misconfig regression in any of them
# turns the build red, not just the one we ship.
DOCKERFILES := deployment/Dockerfile scripts/Dockerfile.mcpb docs/how-to/docker/Dockerfile.librechat

.PHONY: dockerlint
dockerlint: ## Lint the Dockerfiles (hadolint)
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
dockersec: ## Scan the Dockerfiles for security misconfigurations (trivy)
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
secrets: ## Scan the working tree for committed secrets (gitleaks)
	@echo $(BGreen)---------------------------$(Color_Off)
	@echo $(BGreen)-- Scanning for secrets  --$(Color_Off)
	@echo $(BGreen)---------------------------$(Color_Off)
	docker run --rm -v $(CURDIR):/repo -w /repo $(GIT_WORKTREE_MOUNT) zricethezav/gitleaks:v8.30.1 dir --redact --verbose --config /repo/.gitleaks.toml .

.PHONY: build
build: ## Build binary for current host OS/Arch
	@echo $(BGreen)----------------------------$(Color_Off)
	@echo $(BGreen)-- Building TheHive MCP --$(Color_Off)
	@echo $(BGreen)----------------------------$(Color_Off)
	@HOST_OS=$$(uname -s | tr '[:upper:]' '[:lower:]'); \
	HOST_ARCH=$$(uname -m); \
	if [ "$$HOST_ARCH" = "x86_64" ]; then HOST_ARCH="amd64"; fi; \
	if [ "$$HOST_ARCH" = "aarch64" ]; then HOST_ARCH="arm64"; fi; \
	echo "Building for $$HOST_OS-$$HOST_ARCH..."; \
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

# Integration mode is NOT chosen by a committed/configured secret. It is decided
# at run time by scripts/reset-integration-db.sh purely on whether the StrangeBee
# licensing image is pullable (see that script + the recipe below): pullable ⇒
# mint a dev license ⇒ parallel one-org-per-test; not pullable ⇒ free license ⇒
# sequential shared-org. The script materialises the minted token to this file,
# which the recipe reads to set THEHIVE_TEST_LICENSE and the go-test parallelism
# for the go-test container.
LICENSE_FILE := internal/testutils/testdata/.test-license.lic.local

.PHONY: test
test: pre ## Run fast unit tests, skipping integration tests (COVERAGE=1 for coverage, RUN=<regexp> to filter)
	@echo $(BGreen)-----------------------$(Color_Off)
	@echo $(BGreen)-- Running UnitTests --$(Color_Off)
	@echo $(BGreen)-----------------------$(Color_Off)
	docker run -i --rm -v $(CURDIR):/app -w /app $(DOCKER_CACHE_MOUNTS) $(GO_IMAGE) go test $(GO_TEST_COVER) $(GO_TEST_RUN) -short -v ./...
ifeq ($(COVERAGE),1)
	docker run -i --rm -v $(CURDIR):/app -w /app $(DOCKER_CACHE_MOUNTS) $(GO_IMAGE) go tool cover -func=coverage.out
endif

# test-race runs -short only: the race detector is meaningful on our own
# concurrency (parallel tests, fan-out helpers), not on the live SDK/HTTP client
# the integration suite drives, and a -race integration run would be far slower.
# It uses GO_IMAGE_CGO with CGO_ENABLED=1 because -race needs cgo (the Alpine
# GO_IMAGE has no C toolchain).
.PHONY: test-race
test-race: pre ## Run fast unit tests under the race detector (RUN=<regexp> to filter)
	@echo $(BGreen)-- Running UnitTests under -race --$(Color_Off)
	docker run -i --rm -v $(CURDIR):/app -w /app -e CGO_ENABLED=1 $(DOCKER_CACHE_MOUNTS) $(GO_IMAGE_CGO) go test -race $(GO_TEST_RUN) -short ./...

.PHONY: test-integration
test-integration: pre ## Run the full test suite against the docker-compose test stack (COVERAGE=1 for coverage, RUN=<regexp> to filter). Leaves the stack UP for fast reruns; use `make test-integration-down` to tear it down.
	@echo $(BGreen)------------------------------$(Color_Off)
	@echo $(BGreen)-- Running Integration Tests --$(Color_Off)
	@echo $(BGreen)------------------------------$(Color_Off)
	# Bring up the TheHive + Elasticsearch + Cassandra + MITRE stack
	# (THEHIVE_TEST_IMAGE selects the version) and reset its databases to a clean
	# state (scripts/reset-integration-db.sh — see it for the why), then run the
	# suite against it on the host network. The stack is deliberately LEFT UP for
	# fast reruns; `make test-integration-down` tears it down.
	#
	# The reset script decides the mode by whether the licensing image is
	# pullable, and writes the minted token (or nothing) to LICENSE_FILE. We read
	# that file here to configure the go-test container:
	#   - Non-empty (license minted) ⇒ export THEHIVE_TEST_LICENSE so the Go suite
	#     runs per-test org + user via t.Parallel (testutils.Parallel), packages
	#     concurrent (no -p 1).
	#   - Empty/absent (free license) ⇒ one shared org, testutils.Parallel no-ops
	#     so tests run sequentially and purge their own data; `-p 1` stops packages
	#     from racing on that shared org.
	# Isolation in either mode comes from internal/testutils/orgs.go.
	#
	# -timeout 20m raises the per-package deadline above the default 10m: under
	# memory pressure a single request can stall, and the setup helpers retry
	# those (each capped at the client's 90s transport timeout), so the headroom
	# keeps a transient stall from tripping the package timeout. The sequential
	# free-license path is the slowest run, so it needs this headroom most.
	./scripts/reset-integration-db.sh docker-compose.test.yml
	@if [ -s "$(LICENSE_FILE)" ]; then \
		echo "License minted → parallel multi-org mode."; \
		LIC="$$(cat "$(LICENSE_FILE)")"; PARALLELISM=""; \
	else \
		echo "No license → sequential shared-org mode."; \
		LIC=""; PARALLELISM="-p 1"; \
	fi; \
	docker run -i --rm --network host -v $(CURDIR):/app -w /app -e THEHIVE_TEST_URL -e LOG_LEVEL -e THEHIVE_TEST_IMAGE -e THEHIVE_TEST_LICENSE="$$LIC" $(DOCKER_CACHE_MOUNTS) $(GO_TOOLS_CACHE) $(GO_IMAGE) sh -c "command -v gotestsum >/dev/null 2>&1 || go install gotest.tools/gotestsum@v1.13.0 ; gotestsum --format pkgname --hide-summary=skipped -- $(GO_TEST_COVER) $(GO_TEST_RUN) $$PARALLELISM -timeout 20m ./..."
ifeq ($(COVERAGE),1)
	docker run -i --rm -v $(CURDIR):/app -w /app $(DOCKER_CACHE_MOUNTS) $(GO_IMAGE) go tool cover -func=coverage.out
endif

.PHONY: test-integration-down
test-integration-down: ## Tear down the integration test stack left up by `make test-integration` (containers, network, volumes)
	docker compose -f docker-compose.test.yml down -v

.PHONY: docker-build
docker-build: ## Build Docker image
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
docker-run: docker-build ## Run production Docker container
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
dev: ## Run development server with hot reload
	@echo $(BGreen)--------------------------------$(Color_Off)
	@echo $(BGreen)-- Starting development mode --$(Color_Off)
	@echo $(BGreen)--------------------------------$(Color_Off)
	@if ! command -v air >/dev/null 2>&1; then \
		echo "Installing air for hot reload..."; \
		go install github.com/air-verse/air@latest; \
	fi; \
	air

.PHONY: run
run: build ## Run the application (usage: make run ARGS="your arguments here")
	@echo $(BGreen)---------------------------$(Color_Off)
	@echo $(BGreen)-- Starting application  --$(Color_Off)
	@echo $(BGreen)---------------------------$(Color_Off)
	@HOST_OS=$$(uname -s | tr '[:upper:]' '[:lower:]'); \
	HOST_ARCH=$$(uname -m); \
	if [ "$$HOST_ARCH" = "x86_64" ]; then HOST_ARCH="amd64"; fi; \
	if [ "$$HOST_ARCH" = "aarch64" ]; then HOST_ARCH="arm64"; fi; \
	$(BUILDDIR)/$(BINARY_NAME)-$$HOST_OS-$$HOST_ARCH $(ARGS)


.PHONY: vulncheck
vulncheck: ## Check for vulnerabilities
	@echo $(BGreen)------------------------------$(Color_Off)
	@echo $(BGreen)-- Security Vulnerability  --$(Color_Off)
	@echo $(BGreen)------------------------------$(Color_Off)
	docker run -i --rm -v $(CURDIR):/app -w /app $(DOCKER_CACHE_MOUNTS) $(GO_TOOLS_CACHE) $(GO_IMAGE) sh -c 'go install golang.org/x/vuln/cmd/govulncheck@v1.3.0 && govulncheck ./...'

.PHONY: lint
lint: ## Run linter checks without modifying files
	@echo $(BGreen)-----------------------------$(Color_Off)
	@echo $(BGreen)-- Linter Checks --$(Color_Off)
	@echo $(BGreen)-----------------------------$(Color_Off)
	docker run --rm -v $(CURDIR):/app -w /app golangci/golangci-lint:v2.12.2 golangci-lint config verify
	docker run -v $(CURDIR):/app -w /app -i --rm $(DOCKER_CACHE_MOUNTS) $(GOLANGCI_CACHE) $(GIT_WORKTREE_MOUNT) golangci/golangci-lint:v2.12.2 golangci-lint run

.PHONY: lint-fix
lint-fix: fmt ## Format the code, then run linter checks with auto-fix
	@echo $(BGreen)-----------------------------$(Color_Off)
	@echo $(BGreen)-- Linter Checks with auto-fix --$(Color_Off)
	@echo $(BGreen)-----------------------------$(Color_Off)
	docker run --rm -v $(CURDIR):/app -w /app golangci/golangci-lint:v2.12.2 golangci-lint config verify
	docker run -v $(CURDIR):/app -w /app -i --rm $(DOCKER_CACHE_MOUNTS) $(GOLANGCI_CACHE) $(GIT_WORKTREE_MOUNT) golangci/golangci-lint:v2.12.2 golangci-lint run --fix

.PHONY: updatedep
updatedep: ## Update dependencies
	@echo $(BGreen)-----------------------$(Color_Off)
	@echo $(BGreen)-- Update Dependencies --$(Color_Off)
	@echo $(BGreen)-----------------------$(Color_Off)
	docker run -i --rm -v $(CURDIR):/app -w /app $(DOCKER_CACHE_MOUNTS) $(GO_IMAGE) sh -c 'go get -u ./... && go mod tidy'

.PHONY: install-dev-deps
install-dev-deps: ## Install development dependencies
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
	docker run -i --rm -v $(CURDIR):/app -w /app -e GOOS=$$OS -e GOARCH=$$ARCH $(DOCKER_CACHE_MOUNTS) $(GO_IMAGE) go build $(GOLDFLAGS) -o $(BUILDDIR)/$(BINARY_NAME)-$*$(call bin_ext,$*) ./cmd/server/main.go

.PHONY: pre-dist
pre-dist: ## Create distribution directory
	@mkdir -p $(DISTDIR)

.PHONY: build-current
build-current: build ## Alias for build (builds for current host platform)

.PHONY: build-all
build-all: $(addprefix build-,$(RELEASE_TARGETS)) ## Build binaries for all release targets

.PHONY: package-release
package-release: build-all package-from-built ## Build, then package all binaries and MCPB for release (local: no signing)

# package-from-built packages whatever binaries already exist in $(BUILDDIR) — it
# does NOT (re)build. This is the seam the release CI needs: build-all -> sign the
# windows .exe in place -> package-from-built, so packaging consumes the *signed*
# binaries instead of clobbering them with a fresh build-all.
.PHONY: package-from-built
package-from-built: pre-dist mcpb-ci-nobuild ## Package pre-built (already-signed) binaries + MCPB
	@echo $(BGreen)--------------------------------$(Color_Off)
	@echo $(BGreen)-- Packaging release binaries --$(Color_Off)
	@echo $(BGreen)--------------------------------$(Color_Off)
	@for target in $(RELEASE_TARGETS); do \
		echo "Packaging $$target..."; \
		case "$$target" in windows-*) ext=".exe";; *) ext="";; esac; \
		cp $(BUILDDIR)/$(BINARY_NAME)-$$target$$ext $(DISTDIR)/$(BINARY_NAME)-$$target$$ext; \
		(cd $(DISTDIR) && tar -czf $(BINARY_NAME)-$(VERSION)-$$target.tar.gz $(BINARY_NAME)-$$target$$ext); \
		shasum -a 256 $(DISTDIR)/$(BINARY_NAME)-$(VERSION)-$$target.tar.gz > $(DISTDIR)/$(BINARY_NAME)-$(VERSION)-$$target.tar.gz.sha256; \
		rm $(DISTDIR)/$(BINARY_NAME)-$$target$$ext; \
	done
	@echo "All release packages created in $(DISTDIR)/"
	@ls -1 $(DISTDIR)/

.PHONY: version
version: ## Display current version
	@echo $(VERSION)

.PHONY: mcpb-build-image
mcpb-build-image: ## Build Docker image for MCPB generation
	@echo $(BGreen)-----------------------------$(Color_Off)
	@echo $(BGreen)-- Building MCPB CI Image  --$(Color_Off)
	@echo $(BGreen)-----------------------------$(Color_Off)
	docker build -f scripts/Dockerfile.mcpb -t thehivemcp-mcpb:latest .

.PHONY: mcpb-local
mcpb-local: build ## Generate MCPB package locally
	@echo $(BGreen)------------------------------$(Color_Off)
	@echo $(BGreen)-- Generating MCPB Package --$(Color_Off)
	@echo $(BGreen)------------------------------$(Color_Off)
	./scripts/generate-mcpb.sh

# The set of targets mcpb-ci-nobuild packages. Defaults to every release
# target; override it to a subset for a fast packaging smoke-test (see
# mcpb-smoke and the ci.yml mcpb-package job).
MCPB_TARGETS ?= $(RELEASE_TARGETS)

.PHONY: mcpb-ci
mcpb-ci: build-all mcpb-ci-nobuild ## Build, then generate MCPB packages for all architectures

# mcpb-smoke exercises the MCPB packaging path (container permissions, manifest
# generation, non-empty output) for a single target — fast enough to run on
# every PR that touches the packaging inputs. Reuses build-<target> +
# mcpb-ci-nobuild with MCPB_TARGETS narrowed to one arch.
.PHONY: mcpb-smoke
mcpb-smoke: ## Smoke-test MCPB packaging for MCPB_SMOKE_TARGET (default linux-amd64)
	@$(MAKE) build-$(MCPB_SMOKE_TARGET)
	@$(MAKE) mcpb-ci-nobuild MCPB_TARGETS=$(MCPB_SMOKE_TARGET)
MCPB_SMOKE_TARGET ?= linux-amd64

# mcpb-ci-nobuild wraps the already-built (and, in release CI, already-signed)
# binaries in $(BUILDDIR) into MCPB packages. It deliberately does NOT depend on
# build-all so a signing step can run between build-all and packaging.
.PHONY: mcpb-ci-nobuild
mcpb-ci-nobuild: pre-dist mcpb-build-image ## Generate MCPB packages from pre-built binaries
	@echo $(BGreen)----------------------------------$(Color_Off)
	@echo $(BGreen)-- Generating MCPB Packages CI --$(Color_Off)
	@echo $(BGreen)----------------------------------$(Color_Off)
	@set -e; for target in $(MCPB_TARGETS); do \
		echo "Generating MCPB for $$target..."; \
		case "$$target" in windows-*) ext=".exe";; *) ext="";; esac; \
		ws=/tmp/mcpb-workspace-$$target; \
		mkdir -p $$ws/binaries; \
		cp $(BUILDDIR)/thehivemcp-$$target$$ext $$ws/binaries/; \
		docker run --rm \
			--user "$$(id -u):$$(id -g)" \
			-v $$ws:/workspace \
			-e HOME=/workspace \
			-e CI_MODE=true \
			-e VERSION=$(VERSION) \
			-e TARGET_ARCH=$$target \
			thehivemcp-mcpb:latest; \
		cp $$ws/thehivemcp-$(VERSION)-$$target.mcpb $(DISTDIR)/; \
		shasum -a 256 $(DISTDIR)/thehivemcp-$(VERSION)-$$target.mcpb > $(DISTDIR)/thehivemcp-$(VERSION)-$$target.mcpb.sha256; \
		docker run --rm -v $$ws:/workspace alpine:latest rm -rf /workspace/* || rm -rf $$ws || true; \
	done
	@echo "All MCPB packages created in $(DISTDIR)/"

# The windows release targets, expanded to their built .exe paths.
WINDOWS_TARGETS := $(filter windows-%,$(RELEASE_TARGETS))
WINDOWS_BINARIES := $(foreach t,$(WINDOWS_TARGETS),$(BUILDDIR)/$(BINARY_NAME)-$(t).exe)

.PHONY: sign-windows
sign-windows: ## Sign the built windows .exe binaries in place (release CI; mechanism-agnostic)
	@echo $(BGreen)-----------------------------$(Color_Off)
	@echo $(BGreen)-- Signing Windows binaries --$(Color_Off)
	@echo $(BGreen)-----------------------------$(Color_Off)
	./scripts/sign-windows.sh $(WINDOWS_BINARIES)
