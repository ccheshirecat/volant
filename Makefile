GO ?= go
INSTALL_DIR ?= /usr/local/bin
BIN_DIR ?= bin

.PHONY: help
help: ## Show available make targets
	@grep -E '^[a-zA-Z_-]+:.*##' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*##"} {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

.PHONY: build
build: build-server build-agent build-cli ## Build all core binaries into $(BIN_DIR)

.PHONY: build-server
build-server: ## Build the volantd binary
	mkdir -p $(BIN_DIR)
	$(GO) build -o $(BIN_DIR)/volantd ./cmd/volantd

.PHONY: build-agent
build-agent: ## Build the volary agent binary
	mkdir -p $(BIN_DIR)
	$(GO) build -o $(BIN_DIR)/volary ./cmd/volary

.PHONY: build-cli
build-cli: ## Build the volar CLI binary
	mkdir -p $(BIN_DIR)
	$(GO) build -o $(BIN_DIR)/volar ./cmd/volar

.PHONY: build-openapi-export
build-openapi-export: ## Build the openapi-export utility
	mkdir -p $(BIN_DIR)
	$(GO) build -o $(BIN_DIR)/openapi-export ./cmd/openapi-export

.PHONY: openapi:export
openapi\:export: build-openapi-export ## Generate OpenAPI JSON to docs/api-reference/openapi.json
	$(BIN_DIR)/openapi-export -server https://docs.volant.cloud -output docs/api-reference/openapi.json

.PHONY: install
install: build ## Install core binaries into INSTALL_DIR (default: /usr/local/bin)
	mkdir -p $(INSTALL_DIR)
	install -m 0755 $(BIN_DIR)/volantd $(INSTALL_DIR)/volantd
	install -m 0755 $(BIN_DIR)/volary $(INSTALL_DIR)/volary
	install -m 0755 $(BIN_DIR)/volar $(INSTALL_DIR)/volar

.PHONY: test
test: ## Run unit tests
	$(GO) test ./...

.PHONY: fmt
fmt: ## Format Go sources
	$(GO) fmt ./...

.PHONY: vet
vet: ## Run go vet
	$(GO) vet ./...

.PHONY: ci
ci: fmt vet test

.PHONY: tidy
tidy: ## Sync go.mod
	$(GO) mod tidy

.PHONY: clean
clean: ## Remove build artifacts
	rm -rf $(BIN_DIR)