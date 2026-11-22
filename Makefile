# ==============================================================================
# Makefile
# Developer & Operational Automation Interface for arturonavax/anx.
# ==============================================================================

.DEFAULT_GOAL := help
.PHONY: help build install test test-go test-e2e test-unit test-integration lint check wizard plan dry-run uninstall-plan backup restore diff sync clean rebuild rebuild-tools optimize optimize-apply anx-plan anx-status anx-doctor

BUILD_VERSION := ./internal/buildversion/cmd
export ANX_BUILD_VERSION := $(VERSION)
export ANX_BUILD_COMMIT := $(COMMIT)
export ANX_BUILD_DATE := $(DATE)
ANX_RUN = flags=$$(go run $(BUILD_VERSION) -ldflags) && go run -ldflags "$$flags" ./cmd/anx

.PHONY: version version-next
version:
	@go run $(BUILD_VERSION)

version-next:
	@go run $(BUILD_VERSION) -next

build:
	@mkdir -p bin
	@flags=$$(go run $(BUILD_VERSION) -ldflags) && go build -ldflags "$$flags" -o bin/anx ./cmd/anx
	@ln -sf anx bin/aitools
	@echo "Built bin/anx and bin/aitools (multicall symlink) successfully."

install:
	@./scripts/bootstrap_anx.sh apply

test-go:
	@go test -v -race ./...

test-e2e:
	@./scripts/test_e2e_docker.sh all

anx-plan:
	@$(ANX_RUN) plan

anx-status:
	@$(ANX_RUN) status

anx-doctor:
	@$(ANX_RUN) doctor

help:
	@echo "arturonavax/anx — Development & Operational Interface"
	@echo ""
	@echo "Available Targets:"
	@echo "  make version           Print the development version for this checkout"
	@echo "  make version-next      Print the planned stable version (no tag or release)"
	@echo "  make build             Compile anx and aitools binaries into bin/"
	@echo "  make install           Bootstrap Go and apply workstation state via anx"
	@echo "  make test-go           Run full Go unit & race test suite"
	@echo "  make test              Run comprehensive test harness (syntax, lint, unit, integration)"
	@echo "  make anx-plan          Run anx execution planner"
	@echo "  make anx-status        Inspect anx platform and package status"
	@echo "  make anx-doctor        Run anx diagnostic health checks"
	@echo "  make plan              Preview installation plan, packages, and touched files (dry-run)"
	@echo "  make uninstall-plan    Preview uninstallation plan, purged binaries, and caches"
	@echo "  make lint              Run syntax check (bash -n) and ShellCheck analysis"
	@echo "  make wizard            Launch interactive installation wizard"
	@echo "  make clean             Remove temporary test sandboxes and stale caches"
	@echo ""

optimize:
	@$(ANX_RUN) optimize --report

optimize-apply:
	@$(ANX_RUN) optimize --apply

rebuild:
	@$(ANX_RUN) rebuild all

rebuild-tools: rebuild

plan:
	@./install.sh --plan

dry-run:
	@./install.sh --plan

uninstall-plan:
	@./uninstall.sh --plan

test:
	@./tests/run_tests.sh

test-unit:
	@go test -race ./internal/...

test-integration:
	@./tests/integration/test_cli_smoke.sh

lint:
	@./tests/run_tests.sh --lint

check:
	@./install.sh --check

wizard:
	@./install.sh --wizard

backup:
	@$(ANX_RUN) backup create

restore:
	@$(ANX_RUN) backup restore -latest

diff:
	@$(ANX_RUN) sync --dry-run

sync:
	@$(ANX_RUN) sync

clean:
	@rm -rf bin/ /tmp/env_test_sandbox.* /tmp/bats_sandbox.* 2>/dev/null || true
	@echo "Cleaned temporary test sandboxes and bin/."
