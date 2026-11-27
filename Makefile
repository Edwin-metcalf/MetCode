# Makefile for MetCode: shortcuts for the commands contributors (and CI) run.
#
# Run `make help` to see every target. `make check` runs the same steps as
# .github/workflows/ci.yml, so if it passes locally CI should pass too.
#
# Note: recipe lines (the indented commands under each target) MUST start with
# a tab character, not spaces, or make will fail with "missing separator".

# Let `go` be overridden, e.g. `make test GO=go1.27`.
GO ?= go

# Running plain `make` with no target runs `build`.
.DEFAULT_GOAL := build

# These targets are names of actions, not files. Without this, a file called
# `test` or `build` in the repo would make make think the target is up to date.
.PHONY: help build vet test fmt fmt-check bench-validate check

help: ## Show this list of targets
	@echo "Usage: make <target>"
	@echo ""
	@grep -E '^[a-zA-Z_-]+:.*## ' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*## "}; {printf "  %-15s %s\n", $$1, $$2}'

build: ## Compile every package (go build ./...)
	$(GO) build ./...

vet: ## Run go vet on every package
	$(GO) vet ./...

test: ## Run all tests (no Ollama server or network needed)
	$(GO) test ./...

fmt: ## Rewrite files in place with gofmt
	gofmt -w .

fmt-check: ## Fail if any file is not gofmt-formatted (what CI checks)
	@unformatted="$$(gofmt -l .)"; \
	if [ -n "$$unformatted" ]; then \
		echo "These files need formatting (run 'make fmt'):"; \
		echo "$$unformatted"; \
		exit 1; \
	fi

bench-validate: ## Check every benchmark fixture fails before a model touches it
	$(GO) run ./cmd/metbench validate

check: build vet fmt-check test bench-validate ## Run the full CI contract, in CI order
