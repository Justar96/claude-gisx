.PHONY: help build test vet fmt fmt-check tidy clean dist install run

BIN     := claude-gisx
VERSION ?= $(shell git describe --tags --abbrev=0 2>/dev/null | sed 's/^v//' || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

help: ## Show this help
	@awk 'BEGIN{FS=":.*##"; printf "Usage: make \033[36m<target>\033[0m\n\nTargets:\n"} /^[a-zA-Z_-]+:.*?##/ {printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: ## Build for host platform into ./$(BIN)
	go build -trimpath -ldflags="$(LDFLAGS)" -o $(BIN) .

test: ## Run tests with race detector
	go test -race -count=1 ./...

vet: ## go vet
	go vet ./...

fmt: ## Apply gofmt -w
	gofmt -w .

fmt-check: ## Fail if gofmt would change anything
	@out=$$(gofmt -d .); \
	if [ -n "$$out" ]; then echo "$$out"; echo "✗ gofmt found unformatted files"; exit 1; fi
	@echo "✓ gofmt clean"

tidy: ## go mod tidy
	go mod tidy

check: fmt-check vet test ## fmt-check + vet + test

clean: ## Remove build artifacts
	rm -rf dist $(BIN)

dist: ## Cross-compile all release targets into dist/
	VERSION=$(VERSION) ./scripts/build.sh

install: build ## Build and run setup against ~/.claude/settings.json
	./$(BIN) setup --force

run: build ## Build and render with a fixture on stdin
	@echo '{"model":{"display_name":"Opus"},"context_window":{"used_percentage":15,"context_window_size":200000}}' | ./$(BIN)
