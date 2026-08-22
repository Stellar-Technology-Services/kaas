# kaas — build and test
#
# Usage: make          # build
#        make test
#        make check    # vet + test
#        make install

.DEFAULT_GOAL := build

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -X github.com/Stellar-Technology-Services/kaas/internal/cli.Version=$(VERSION)

BIN_DIR ?= dist
BIN     := $(BIN_DIR)/kaas
PREFIX  ?= /usr/local

.PHONY: build test vet check install clean run help

help: ## Show targets
	@awk 'BEGIN {FS = ":.*?## "}; /^[a-zA-Z0-9_-]+:.*?## / { \
		printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2 \
	}' $(MAKEFILE_LIST)

build: ## Build ./dist/kaas
	@mkdir -p $(BIN_DIR)
	go build -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/kaas

test: ## Run go test ./...
	go test ./...

vet: ## Run go vet ./...
	go vet ./...

check: vet test ## Vet + test

install: build ## Install kaas to $(PREFIX)/bin
	install -d $(PREFIX)/bin
	install -m 755 $(BIN) $(PREFIX)/bin/kaas

run: build ## Build and print kaas version
	$(BIN) version

clean: ## Remove build artifacts
	rm -rf $(BIN_DIR)
