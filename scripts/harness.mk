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
	@test -n "$(ID)" || { echo 'usage: make gate ID=<gate> [CADENCE=pr]'; exit 2; }
	@bin/harness run --cadence $(or $(CADENCE),pr) --gate $(ID) --force

.PHONY: harness-docs
harness-docs: ## Regenerate docs/reference/gates.md from gates/registry.yaml ##@ make harness-docs
	@bin/harness docs

.PHONY: push
push: ## Push this branch to origin: never main, never the public mirror ##@ make push [FORCE=1]
	@bin/harness push $(if $(FORCE),--force)
