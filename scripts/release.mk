# The release: the cross-compiled binaries, their tarballs and MCPB packages, and the signed
# Windows binaries, as release.yml runs them (build-all, sign-windows, package-from-built). The
# Makefile includes this file, so `make help` lists these targets with the others; it is harness
# policy, as the Makefile is.
# Dynamic build target for any platform in RELEASE_TARGETS
build-%: pre
	@echo "Building for $*..."
	@OS=$$(echo $* | cut -d- -f1); \
	ARCH=$$(echo $* | cut -d- -f2); \
	docker run -i --rm -v $(MOUNT_APP) -w /app $(CA_MOUNT) -e GOOS=$$OS -e GOARCH=$$ARCH $(DOCKER_CACHE_MOUNTS) $(GO_IMAGE) go build $(GOLDFLAGS) -o $(BUILDDIR)/$(BINARY_NAME)-$*$(call bin_ext,$*) ./cmd/server/main.go

.PHONY: pre-dist
pre-dist: ## Internal: ensure the dist/ directory exists (packaging dep) ##@ make pre-dist (usually run automatically)
	@mkdir -p $(DISTDIR)

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
	@bin/harness package tarballs --version "$(VERSION)" --build "$(BUILDDIR)" --dist "$(DISTDIR)" \
		--name "$(BINARY_NAME)" --targets "$(RELEASE_TARGETS)"

.PHONY: mcpb-build-image
mcpb-build-image: ## Internal: build the MCPB packaging image (dep of mcpb-*) ##@ make mcpb-build-image (usually run automatically)
	@echo $(BGreen)-----------------------------$(Color_Off)
	@echo $(BGreen)-- Building MCPB CI Image  --$(Color_Off)
	@echo $(BGreen)-----------------------------$(Color_Off)
	docker build -f scripts/Dockerfile.mcpb -t thehivemcp-mcpb:latest .

.PHONY: mcpb-local
mcpb-local: build mcpb-build-image ## Build + package a single MCPB for your host, to test it ##@ make mcpb-local [PERMISSIONS_CONFIG=<file>]
	@echo $(BGreen)------------------------------$(Color_Off)
	@echo $(BGreen)-- Generating MCPB Package --$(Color_Off)
	@echo $(BGreen)------------------------------$(Color_Off)
	bin/harness package mcpb --host --version "$(VERSION)" --build "$(BUILDDIR)" --dist "$(DISTDIR)" --name "$(BINARY_NAME)"

# The set of targets mcpb-ci-nobuild packages. Defaults to every release
# target; override it to a subset for a fast packaging smoke-test (see
# mcpb-smoke and the ci.yml mcpb-package job).
MCPB_TARGETS ?= $(RELEASE_TARGETS)

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
	@bin/harness package mcpb --version "$(VERSION)" --build "$(BUILDDIR)" --dist "$(DISTDIR)" \
		--name "$(BINARY_NAME)" --targets "$(MCPB_TARGETS)"

# The windows release targets, expanded to their built .exe paths.
WINDOWS_TARGETS := $(filter windows-%,$(RELEASE_TARGETS))
WINDOWS_BINARIES := $(foreach t,$(WINDOWS_TARGETS),$(BUILDDIR)/$(BINARY_NAME)-$(t).exe)

.PHONY: sign-windows
sign-windows: ## Release CI: code-sign the built Windows .exe files in place ##@ make sign-windows
	@echo $(BGreen)-----------------------------$(Color_Off)
	@echo $(BGreen)-- Signing Windows binaries --$(Color_Off)
	@echo $(BGreen)-----------------------------$(Color_Off)
	bin/harness sign-windows $(WINDOWS_BINARIES)
