# foundry-copilot — top-level dev Makefile
#
# All Go work lives in ./core; all TS extension work in ./extension.

.PHONY: help deps build build-go build-ext test test-lock test-go test-ext lint \
        sidecar harness vsix clean smoke run-sidecar run-harness

GO          ?= go
GO_PKG       = ./...
NODE        ?= node
NPM         ?= npm
VERSION     ?= 0.1.0
PLATFORMS    = darwin-arm64 darwin-amd64 linux-amd64 linux-arm64 windows-amd64

help: ## Show this help
	@awk 'BEGIN {FS = ":.*##"} /^[a-zA-Z_-]+:.*?##/ {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

deps: ## Install all dependencies (Go + TS)
	cd core && $(GO) mod download
	cd extension && $(NPM) install

build: build-go build-ext ## Build sidecar binary + extension bundle

build-go: ## Build sidecar binary for current platform
	mkdir -p extension/bin/$$($(GO) env GOOS)-$$($(GO) env GOARCH)
	cd core && $(GO) build -o ../extension/bin/$$($(GO) env GOOS)-$$($(GO) env GOARCH)/foundry-copilot-sidecar ./cmd/sidecar
	cd core && $(GO) build -o ../extension/bin/$$($(GO) env GOOS)-$$($(GO) env GOARCH)/foundry-copilot ./cmd/harness

build-ext: ## Build the VS Code extension bundle
	cd extension && $(NPM) run build

sidecar: ## Build only the sidecar binary
	cd core && $(GO) build -o ../extension/bin/$$($(GO) env GOOS)-$$($(GO) env GOARCH)/foundry-copilot-sidecar ./cmd/sidecar

harness: ## Build only the standalone TUI harness
	cd core && $(GO) build -o ../extension/bin/$$($(GO) env GOOS)-$$($(GO) env GOARCH)/foundry-copilot ./cmd/harness

test: test-go test-ext ## Run all tests

test-go: ## Run Go tests
	cd core && $(GO) test -race -count=1 $(GO_PKG)

test-lock: ## Run ONLY the hard-lock tests (CI gate)
	cd core && $(GO) test -race -count=1 -v ./internal/foundry/...

test-ext: ## Run extension tests (if any)
	cd extension && $(NPM) test --if-present

lint: ## Run Go vet + TS lint
	cd core && $(GO) vet $(GO_PKG)
	cd extension && $(NPM) run lint --if-present

vsix: build ## Package a VSIX for the current platform
	cd extension && $(NPM) run package

clean: ## Remove build artifacts
	rm -rf extension/bin extension/out extension/dist extension/*.vsix
	rm -rf core/bin core/dist core/coverage.txt
	cd extension && rm -rf node_modules || true

smoke: ## Live smoke test against Foundry (requires FOUNDRY_SMOKE=1 and az login)
	FOUNDRY_SMOKE=1 ./scripts/smoke.sh

run-sidecar: sidecar ## Run the sidecar interactively over stdio (for debugging)
	./extension/bin/$$($(GO) env GOOS)-$$($(GO) env GOARCH)/foundry-copilot-sidecar --log-level=debug

run-harness: harness ## Run the standalone TUI
	./extension/bin/$$($(GO) env GOOS)-$$($(GO) env GOARCH)/foundry-copilot chat
