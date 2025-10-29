# Makefile for gohivex/bindings - libhivex CGO bindings generator

.PHONY: help generate clean verify test install-tools check-deps

.DEFAULT_GOAL := help

help: ## Display this help message
	@echo "gohivex/bindings - libhivex CGO bindings"
	@echo ""
	@echo "Usage: make <target>"
	@echo ""
	@echo "Targets:"
	@awk 'BEGIN {FS = ":.*##"} /^[a-zA-Z_-]+:.*?##/ { printf "  %-20s %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

check-deps: ## Check required dependencies
	@echo "Checking dependencies..."
	@echo -n "  pkg-config: "
	@which pkg-config > /dev/null && echo "✓" || (echo "✗ (run: brew install pkg-config)" && exit 1)
	@echo -n "  libhivex: "
	@pkg-config --exists hivex && echo "✓" || (echo "✗ (run: brew install hivex)" && exit 1)
	@echo -n "  c-for-go: "
	@which c-for-go > /dev/null && echo "✓" || (echo "✗ (run: make install-tools)" && exit 1)
	@echo ""
	@echo "All dependencies satisfied!"

install-tools: ## Install c-for-go code generator (from go.mod)
	@echo "Installing c-for-go from tools.go..."
	@grep _ tools.go | awk '{print $$2}' | xargs -I {} go install {}
	@echo "✓ c-for-go installed (version from go.mod)"
	@c-for-go -version || echo "Note: c-for-go may not support -version flag"

generate: check-deps ## Generate Go bindings from libhivex C headers
	@echo "Generating libhivex Go bindings..."
	@echo ""
	@echo "Configuration: hivex.yml"
	@echo "Target: libhivex ($(shell pkg-config --modversion hivex 2>/dev/null || echo 'unknown version'))"
	@echo ""
	@c-for-go hivex.yml
	@echo ""
	@echo "✓ Bindings generated successfully!"
	@echo ""
	@echo "Generated files:"
	@ls -lh *.go 2>/dev/null | grep -v "go.mod" | grep -v "go.sum" | awk '{print "  " $$9 " (" $$5 ")"}' || echo "  (no .go files found - generation may have failed)"

clean: ## Remove generated Go files
	@echo "Cleaning generated files..."
	@rm -f hivex.go const.go types.go cgo_helpers.go doc.go
	@echo "✓ Clean complete"

verify: ## Verify generated bindings compile
	@echo "Verifying bindings compile..."
	@if [ ! -f hivex.go ]; then \
		echo "✗ hivex.go not found. Run 'make generate' first."; \
		exit 1; \
	fi
	@CGO_ENABLED=1 go build -v .
	@echo ""
	@echo "✓ Bindings compile successfully!"

test: verify ## Run basic functionality tests
	@echo "Running basic binding tests..."
	@CGO_ENABLED=1 go test -v .
	@echo ""
	@echo "✓ Tests passed!"

fmt: ## Format Go code
	@echo "Formatting code..."
	@go fmt .
	@echo "✓ Code formatted"

info: ## Display configuration information
	@echo "Bindings Configuration"
	@echo "====================="
	@echo ""
	@echo "Module: github.com/joshuapare/gohivex/bindings"
	@echo ""
	@echo "System Information:"
	@echo "  OS: $(shell uname -s)"
	@echo "  Architecture: $(shell uname -m)"
	@echo ""
	@echo "Dependencies:"
	@echo "  libhivex version: $(shell pkg-config --modversion hivex 2>/dev/null || echo 'not found')"
	@echo "  libhivex cflags: $(shell pkg-config --cflags hivex 2>/dev/null || echo 'not found')"
	@echo "  libhivex libs: $(shell pkg-config --libs hivex 2>/dev/null || echo 'not found')"
	@echo ""
	@echo "Go Environment:"
	@echo "  Go version: $(shell go version)"
	@echo "  CGO_ENABLED: $(shell go env CGO_ENABLED)"
	@echo ""
	@echo "Generated Files:"
	@ls -lh *.go 2>/dev/null | grep -v "_test.go" | awk '{print "  " $$9 " (" $$5 ")"}' || echo "  (no generated files - run 'make generate')"

regenerate: clean generate verify ## Clean, regenerate, and verify bindings
	@echo ""
	@echo "✓ Bindings regenerated successfully!"
