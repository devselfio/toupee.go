GO ?= go

.DEFAULT_GOAL := help
.PHONY: help build test spec-count spec-drift check tidy require-git-status-clear

help: ## Show this help
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-26s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: ## Build every package (writes no binary)
	$(GO) build ./...

test: ## Run the tests
	$(GO) test ./...

spec-count: ## Print the spec corpus pass count
	@output=$$($(GO) test -run 'TestSpec$$' -v . 2>&1) || { printf '%s\n' "$$output"; exit 1; }; \
	printf '%s\n' "$$output" | grep 'corpus:'

spec-drift: ## Check the corpus record and, when online, Toupee at its pinned tag
	$(GO) run ./cmd/driftcheck

check: ## gofmt, go vet and go mod tidy -diff
	@unformatted=$$(gofmt -l .); \
	if [ -n "$$unformatted" ]; then \
		echo "FAIL: not gofmt'd:"; \
		echo "$$unformatted"; \
		exit 1; \
	fi
	$(GO) vet ./...
	$(GO) mod tidy -diff

tidy: require-git-status-clear ## go mod tidy, on a clean worktree only
	$(GO) mod tidy

require-git-status-clear: ## Fail unless the worktree is clean
	@if [ -n "$$(git status --porcelain)" ]; then \
		echo "FAIL: worktree is dirty; commit or stash first"; \
		exit 1; \
	fi
