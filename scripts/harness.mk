# The harness: every target runs bin/harness, which builds cmd/harness in the pinned Go image and
# runs the gates of gates/registry.yaml (listed in docs/reference/gates.md). The Makefile includes
# this file, so `make help` lists these targets with the others; it is harness policy, as the
# Makefile is.
.PHONY: check-fast
check-fast: ## The fast loop: the gates the changed paths select, about a minute ##@ make check-fast
	@bin/harness run --cadence stop

.PHONY: pre-commit
pre-commit: ## What the git pre-commit hook runs: the commit gates on the staged files ##@ make pre-commit
	@bin/harness run --cadence commit --staged

.PHONY: gate
gate: ## Run one gate now, whatever changed (list: docs/reference/gates.md) ##@ make gate ID=<gate> [CADENCE=pr]
	@test -n "$$ID" || { echo 'usage: make gate ID=<gate> [CADENCE=pr]'; exit 2; }
	@bin/harness run --cadence "$${CADENCE:-pr}" --gate "$$ID" --force

.PHONY: next
next: ## What to work on: a blocked agent, then the open nightly findings, the oldest first ##@ make next
	@bin/harness next

.PHONY: dashboard
dashboard: ## The repository's health page, from git, gh and the tree, opened in the browser ##@ make dashboard [WEEKS=26]
	@bin/harness dashboard --open $(if $(WEEKS),--weeks $(WEEKS))

.PHONY: harness-docs
harness-docs: ## Regenerate docs/reference/gates.md from gates/registry.yaml ##@ make harness-docs
	@bin/harness docs

.PHONY: pr
pr: ## The pr gates at HEAD, recorded, then push the branch (never main) ##@ make pr [FORCE=1]
	@bin/harness pr $(if $(FORCE),--force)

.PHONY: go
go: ## Run a go command in the pinned image (the harness's Go programs) ##@ make go CMD="run ./cmd/surface --check" [GOFLAGS=-short]
	@docker run -i --rm -v $(MOUNT_APP) -w /app $(CA_MOUNT) -e CGO_ENABLED=0 $(DOCKER_CACHE_MOUNTS) $(if $(GOFLAGS),-e GOFLAGS) $(GO_IMAGE_CGO) go $(CMD)

.PHONY: surface-propose
surface-propose: ## Rewrite baselines/surface/ from what the server advertises ##@ make surface-propose
	@$(MAKE) --no-print-directory go CMD="run ./cmd/surface --write"

# The workflows' tests (.claude/workflows/*.test.mjs) run in Node, which has all they need.
NODE_IMAGE := node:22.23.3-alpine@sha256:0a7108bf6c7bf5de370ffb1a3ed6be93d405b43ff159f681a8d18c0e2bc2e402

.PHONY: test-workflows
test-workflows: ## The Claude workflows' tests, against scripted agents (no model runs) ##@ make test-workflows
	@docker run -i --rm -v $(MOUNT_APP):ro -w /app $(NODE_IMAGE) node --test '.claude/workflows/*.test.mjs' && echo '✓ workflow tests passed'
