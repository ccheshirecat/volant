GO ?= go
BIN_DIR ?= bin

.PHONY: build
build: ## Build all Viper binaries
	$(GO) build ./...

.PHONY: build-server
build-server: ## Build the viper-server binary
	$(GO) build -o $(BIN_DIR)/viper-server ./cmd/viper-server

.PHONY: build-agent
build-agent:
	$(GO) build -o $(BIN_DIR)/viper-agent ./cmd/viper-agent

.PHONY: build-cli
build-cli:
	$(GO) build -o $(BIN_DIR)/viper ./cmd/viper

.PHONY: test
test:
	$(GO) test ./...

.PHONY: fmt
fmt:
	$(GO) fmt ./...

.PHONY: vet
vet:
	$(GO) vet ./...

.PHONY: ci
ci: fmt vet test

.PHONY: tidy
tidy:
	$(GO) mod tidy

.PHONY: clean
clean:
	rm -rf $(BIN_DIR)

.PHONY: image
image: build-agent ## Build initramfs and fetch kernel
	./build/images/build-initramfs.sh

.PHONY: setup
setup: build-cli ## Run viper setup (dry run)
	./bin/viper setup --dry-run
