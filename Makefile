BUILD_DATE := $(shell date -u +'%Y-%m-%dT%H:%M:%SZ')
GIT_COMMIT=$(shell git rev-parse HEAD)
VERSION=$(shell git describe --tags 2> /dev/null || echo "v0.0.0-${GIT_COMMIT}")
GO := go
GO_IMAGE := golang:1.26.4-alpine
GOPATH ?= $(shell go env GOPATH)
DOCKER_CACHE_MOUNTS := -v $(GOPATH)/pkg/mod:/go/pkg/mod -v $(HOME)/.cache/go-build:/root/.cache/go-build -v $(GOPATH)/bin:/go/bin
GOLDFLAGS := -ldflags="-s -w -X 'github.com/StrangeBeeCorp/TheHiveMCP/version.buildDate=${BUILD_DATE}' -X 'github.com/StrangeBeeCorp/TheHiveMCP/version.gitCommit=${GIT_COMMIT}' -X 'github.com/StrangeBeeCorp/TheHiveMCP/version.gitVersion=${VERSION}'"
BUILDDIR := ./build
DISTDIR := ./dist
BINARY_NAME := thehivemcp
BGreen="\033[1;32m"       # Green
Color_Off="\033[0m"       # Text Reset

# Release matrix
RELEASE_TARGETS := linux-amd64 linux-arm64 darwin-amd64 darwin-arm64

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
security: vulncheck sast vetlint ## Run security checks

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
	docker run -i --rm -v $(CURDIR):/app -w /app $(DOCKER_CACHE_MOUNTS) $(GO_IMAGE) sh -c 'go install github.com/securego/gosec/v2/cmd/gosec@v2.26.1 && gosec -exclude=G101 ./...'

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

.PHONY: test
test: pre ## Run fast unit tests, skipping integration tests (COVERAGE=1 for coverage, RUN=<regexp> to filter)
	@echo $(BGreen)-----------------------$(Color_Off)
	@echo $(BGreen)-- Running UnitTests --$(Color_Off)
	@echo $(BGreen)-----------------------$(Color_Off)
	docker run -i --rm -v $(CURDIR):/app -w /app $(DOCKER_CACHE_MOUNTS) $(GO_IMAGE) go test $(GO_TEST_COVER) $(GO_TEST_RUN) -short -v ./...
ifeq ($(COVERAGE),1)
	docker run -i --rm -v $(CURDIR):/app -w /app $(DOCKER_CACHE_MOUNTS) $(GO_IMAGE) go tool cover -func=coverage.out
endif

.PHONY: test-integration
test-integration: pre ## Run the full test suite against the docker-compose test stack (COVERAGE=1 for coverage, RUN=<regexp> to filter)
	@echo $(BGreen)------------------------------$(Color_Off)
	@echo $(BGreen)-- Running Integration Tests --$(Color_Off)
	@echo $(BGreen)------------------------------$(Color_Off)
	# Bring up the TheHive + Elasticsearch + MITRE stack (THEHIVE_TEST_IMAGE
	# selects the version), run the suite against it on the host network, then
	# tear it down regardless of the test outcome and propagate that outcome.
	# -p 1 serializes packages: they share one mutable instance, and one TheHive
	# stack at a time keeps memory in bounds on a 16 GB CI runner.
	#
	# -timeout 20m raises the per-package deadline above the default 10m: under
	# memory pressure a single request can stall, and the setup helpers retry
	# those (each capped at the client's 90s transport timeout), so the headroom
	# keeps a transient stall from tripping the package timeout. A true deadlock
	# is still bounded.
	# `up` is chained with && into the same shell line as the test run and an
	# unconditional `down`, so the stack is always torn down — even if `up`
	# itself fails (otherwise make would stop before reaching `down`). STATUS
	# captures the `up && test` outcome and is propagated after teardown.
	docker compose -f docker-compose.test.yml up -d && docker run -i --rm --network host -v $(CURDIR):/app -w /app -e THEHIVE_TEST_URL -e LOG_LEVEL -e THEHIVE_TEST_IMAGE $(DOCKER_CACHE_MOUNTS) $(GO_IMAGE) sh -c 'command -v gotestsum >/dev/null 2>&1 || go install gotest.tools/gotestsum@v1.13.0 ; gotestsum --format pkgname --hide-summary=skipped -- $(GO_TEST_COVER) $(GO_TEST_RUN) -timeout 20m -p 1 ./...' ; STATUS=$$? ; docker compose -f docker-compose.test.yml down -v ; exit $$STATUS
ifeq ($(COVERAGE),1)
	docker run -i --rm -v $(CURDIR):/app -w /app $(DOCKER_CACHE_MOUNTS) $(GO_IMAGE) go tool cover -func=coverage.out
endif

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
	docker run -i --rm -v $(CURDIR):/app -w /app $(DOCKER_CACHE_MOUNTS) $(GO_IMAGE) sh -c 'go install golang.org/x/vuln/cmd/govulncheck@v1.3.0 && govulncheck ./...'

.PHONY: vetlint
vetlint: ## Run linter checks
	@echo $(BGreen)-----------------------------$(Color_Off)
	@echo $(BGreen)-- Linter Checks --$(Color_Off)
	@echo $(BGreen)-----------------------------$(Color_Off)
	docker run -v $(CURDIR):/app -w /app -i --rm golangci/golangci-lint:latest golangci-lint run -v
	docker build -t ${BINARY_NAME}:latest -f deployment/Dockerfile .
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
	docker run -i --rm -v $(CURDIR):/app -w /app -e GOOS=$$OS -e GOARCH=$$ARCH $(DOCKER_CACHE_MOUNTS) $(GO_IMAGE) go build $(GOLDFLAGS) -o $(BUILDDIR)/$(BINARY_NAME)-$* ./cmd/server/main.go

.PHONY: pre-dist
pre-dist: ## Create distribution directory
	@mkdir -p $(DISTDIR)

.PHONY: build-current
build-current: build ## Alias for build (builds for current host platform)

.PHONY: build-all
build-all: $(addprefix build-,$(RELEASE_TARGETS)) ## Build binaries for all release targets

.PHONY: package-release
package-release: pre-dist build-all mcpb-ci ## Package all binaries and MCPB for release
	@echo $(BGreen)--------------------------------$(Color_Off)
	@echo $(BGreen)-- Packaging release binaries --$(Color_Off)
	@echo $(BGreen)--------------------------------$(Color_Off)
	@for target in $(RELEASE_TARGETS); do \
		echo "Packaging $$target..."; \
		cp $(BUILDDIR)/$(BINARY_NAME)-$$target $(DISTDIR)/$(BINARY_NAME)-$$target; \
		(cd $(DISTDIR) && tar -czf $(BINARY_NAME)-$(VERSION)-$$target.tar.gz $(BINARY_NAME)-$$target); \
		shasum -a 256 $(DISTDIR)/$(BINARY_NAME)-$(VERSION)-$$target.tar.gz > $(DISTDIR)/$(BINARY_NAME)-$(VERSION)-$$target.tar.gz.sha256; \
		rm $(DISTDIR)/$(BINARY_NAME)-$$target; \
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

.PHONY: mcpb-ci
mcpb-ci: pre-dist build-all mcpb-build-image ## Generate MCPB packages for all architectures
	@echo $(BGreen)----------------------------------$(Color_Off)
	@echo $(BGreen)-- Generating MCPB Packages CI --$(Color_Off)
	@echo $(BGreen)----------------------------------$(Color_Off)
	@for target in $(RELEASE_TARGETS); do \
		echo "Generating MCPB for $$target..."; \
		mkdir -p /tmp/mcpb-workspace-$$target/binaries; \
		cp $(BUILDDIR)/thehivemcp-$$target /tmp/mcpb-workspace-$$target/binaries/; \
		docker run --rm \
			-v /tmp/mcpb-workspace-$$target:/workspace \
			-e CI_MODE=true \
			-e VERSION=$(VERSION) \
			-e TARGET_ARCH=$$target \
			thehivemcp-mcpb:latest; \
		cp /tmp/mcpb-workspace-$$target/thehivemcp-$(VERSION)-$$target.mcpb $(DISTDIR)/; \
		shasum -a 256 $(DISTDIR)/thehivemcp-$(VERSION)-$$target.mcpb > $(DISTDIR)/thehivemcp-$(VERSION)-$$target.mcpb.sha256; \
		docker run --rm -v /tmp/mcpb-workspace-$$target:/workspace alpine:latest rm -rf /workspace/* || rm -rf /tmp/mcpb-workspace-$$target || true; \
	done
	@echo "All MCPB packages created in $(DISTDIR)/"
