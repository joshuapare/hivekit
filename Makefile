# Makefile for hivekit

.PHONY: help

# Default target
.DEFAULT_GOAL := help

help: ## Show this help message
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Setup

check-go: ## Check Go installation
	@which go > /dev/null || (echo "Error: Go not installed" && exit 1)
	@go version

decompress-hives: ## Decompress test hive files
	@echo "Decompressing test hives..."
	@if [ ! -d testdata/suite ]; then \
		echo " testdata/suite not found. Skipping."; \
	else \
		cd testdata/suite && \
		for f in *.xz; do \
			[ -f "$$f" ] || continue; \
			base=$${f%.xz}; \
			if [ ! -f "$$base" ]; then \
				echo "  Decompressing $$f..."; \
				xz -d -k "$$f" || echo " Failed to decompress $$f"; \
			fi; \
		done; \
		echo "Decompression complete"; \
	fi

install-python-deps: ## Install Python dependencies for benchmarking
	@echo "Setting up Python virtual environment..."
	@which python3 > /dev/null || (echo "Error: python3 not installed" && exit 1)
	@if [ ! -d venv ]; then \
		python3 -m venv venv; \
		echo "Virtual environment created"; \
	fi
	@echo "Installing matplotlib..."
	@./venv/bin/pip install --upgrade pip > /dev/null
	@./venv/bin/pip install matplotlib > /dev/null
	@echo "Python dependencies installed"

setup: check-go decompress-hives ## Complete development environment setup
	@echo "Development environment ready!"

##@ Testing

test: ## Run all tests
	@echo "Running all tests..."
	@go test -v -race -timeout 120s ./...
	@echo "All tests passed!"

test-unit: ## Run unit tests only
	@echo "Running unit tests..."
	@go test -v -race -timeout 30s ./internal/... ./pkg/...

test-integration: ## Run integration tests
	@echo "Running integration tests..."
	@go test -v -timeout 120s ./tests/...

test-hiveexplorer: ## Run hiveexplorer tests
	@echo "Running hiveexplorer tests..."
	@cd cmd/hiveexplorer && go test -v -timeout 120s ./...

test-coverage: ## Generate coverage report
	@go test -v -race -coverprofile=coverage.out -covermode=atomic ./...
	@go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

##@ Build

build: ## Build all packages (library only)
	@echo "Building all packages..."
	@go build ./...
	@echo "Build complete"

build-hiveexplorer: ## Build hiveexplorer TUI binary
	@echo "Building hiveexplorer..."
	@cd cmd/hiveexplorer && go build -o ../../hiveexplorer .
	@echo "hiveexplorer built successfully"
	@echo ""
	@echo "Run './hiveexplorer <hive-file>' to launch the TUI"

install-hiveexplorer: ## Install hiveexplorer TUI to $GOBIN
	@echo "Installing hiveexplorer..."
	@cd cmd/hiveexplorer && go install .
	@echo "Installed to $$(go env GOBIN || go env GOPATH)/bin/hiveexplorer"

install-hivectl: ## Install hivectl CLI to $GOBIN
	@echo "Installing hivectl..."
	@cd cmd/hivectl && go install .
	@echo "Installed to $$(go env GOBIN || go env GOPATH)/bin/hivectl"

##@ Code Quality

lint: ## Run linters
	@if which golangci-lint > /dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		echo "Warning: golangci-lint not installed, using go vet"; \
		go vet ./...; \
	fi

fmt: ## Format code
	@go fmt ./...

##@ Benchmarks

benchmark: ## Run benchmarks
	@echo "Running benchmarks..."
	@go test -bench=. -benchmem -run=^$$ ./tests/integration/benchmarks
	@echo "Benchmarks complete!"

benchmark-compare: ## Run comparison benchmarks (full)
	@./scripts/run_benchmarks.sh

benchmark-quick: ## Run quick benchmarks
	@./scripts/run_benchmarks.sh --quick --bench 'Benchmark(Root|Node|Value|Stat|Detail|Metadata|Last|All|Introspection|FullTreeWalk/.+/(small|medium))'

benchmark-list: ## List benchmark categories
	@./scripts/run_benchmarks.sh --list

benchmark-speedup: ## Run hivexregedit comparison and show speedup table
	@chmod +x scripts/benchmark_comparison.sh
	@./scripts/benchmark_comparison.sh

##@ Documentation

update-readme: ## Update README with tool help outputs and benchmarks
	@./scripts/update_readme.sh

##@ Utilities

clean: ## Clean build artifacts
	@rm -f coverage.out coverage.html hiveexplorer
	@rm -rf tests/integration/output
	@find . -name "*.test" -delete
	@echo "Clean complete"

clean-hives: ## Remove decompressed test hives
	@if [ -d testdata/suite ]; then \
		cd testdata/suite && \
		for f in *.xz; do \
			[ -f "$$f" ] || continue; \
			base=$${f%.xz}; \
			[ -f "$$base" ] && rm -f "$$base"; \
		done; \
		echo "Decompressed hives removed"; \
	fi

list-hives: ## List test hives
	@echo "Core hives (testdata/):"
	@[ -d testdata ] && ls -lh testdata/*.hiv 2>/dev/null | awk '{print "  " $$9 " (" $$5 ")"}' || echo "  (none)"
	@echo ""
	@echo "Windows hives (testdata/suite/):"
	@[ -d testdata/suite ] && ls -lh testdata/suite/ 2>/dev/null | grep -v ".xz" | grep -v ".reg" | grep "^-" | awk '{print "  " $$9 " (" $$5 ")"}' || echo "  (none)"

##@ CI

ci: check-go build test lint ## Run CI pipeline
	@echo "CI pipeline completed successfully!"

.PHONY: check-go decompress-hives install-python-deps setup test test-unit test-integration test-hiveexplorer test-coverage
.PHONY: build build-hiveexplorer install-hiveexplorer lint fmt
.PHONY: benchmark benchmark-compare benchmark-quick benchmark-list
.PHONY: update-readme clean clean-hives list-hives ci
