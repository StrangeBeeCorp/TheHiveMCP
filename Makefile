BUILD_DATE=$(shell date -u +'%Y-%m-%dT%H:%M:%SZ')
GIT_COMMIT=$(shell git rev-parse HEAD)
VERSION=$(shell git describe --tags 2> /dev/null || echo "v0.0.0-${GIT_COMMIT}")
GO := go
GO_IMAGE := golang:1.24.12-alpine
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

.PHONY: fmt
fmt: ## Format the code
	@echo $(BGreen)-------------$(Color_Off)
	@echo $(BGreen)--- Format --$(Color_Off)
	@echo $(BGreen)-------------$(Color_Off)
	docker run -i --rm -v $(CURDIR):/app -w /app $(GO_IMAGE) go fmt ./...
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
	docker run -i --rm -v $(CURDIR):/app -w /app $(GO_IMAGE) sh -c 'go install github.com/securego/gosec/v2/cmd/gosec@latest && gosec ./...'

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

.PHONY: test
test: pre ## Run tests with coverage
	@echo $(BGreen)-----------------------$(Color_Off)
	@echo $(BGreen)-- Running UnitTests --$(Color_Off)
	@echo $(BGreen)-----------------------$(Color_Off)
	docker run -i --rm --network host -v $(CURDIR):/app -w /app -v /var/run/docker.sock:/var/run/docker.sock $(GO_IMAGE) go test -coverprofile=coverage.out -covermode=atomic -v ./...
	docker run -i --rm -v $(CURDIR):/app -w /app $(GO_IMAGE) go tool cover -func=coverage.out

.PHONY: docker
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
	docker run -i --rm -v $(CURDIR):/app -w /app $(GO_IMAGE) sh -c 'go install golang.org/x/vuln/cmd/govulncheck@latest && govulncheck ./...'

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
	docker run -i --rm -v $(CURDIR):/app -w /app $(GO_IMAGE) sh -c 'go get -u ./... && go mod tidy'

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
	docker run -i --rm -v $(CURDIR):/app -w /app -e GOOS=$$OS -e GOARCH=$$ARCH $(GO_IMAGE) go build $(GOLDFLAGS) -o $(BUILDDIR)/$(BINARY_NAME)-$* ./cmd/server/main.go

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
