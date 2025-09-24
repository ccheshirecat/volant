GO ?= go
INSTALL_DIR ?= /usr/local/bin
BIN_DIR ?= bin
ARTIFACTS_DIR ?= build/artifacts

.PHONY: help
help: ## Show available make targets
	@grep -E '^[a-zA-Z_-]+:.*##' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*##"} {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

.PHONY: build
build: build-server build-agent build-cli install-binaries ## Build and install core binaries

.PHONY: build-server
build-server: ## Build the viper-server binary
	$(GO) build -o $(BIN_DIR)/viper-server ./cmd/viper-server

.PHONY: build-agent
build-agent:
	$(GO) build -o $(BIN_DIR)/viper-agent ./cmd/viper-agent

.PHONY: build-cli
build-cli:
	$(GO) build -o $(BIN_DIR)/viper ./cmd/viper

.PHONY: install-binaries
install-binaries: build-server build-cli ## Install viper-server and viper to INSTALL_DIR
	mkdir -p $(INSTALL_DIR)
	install -m 0755 $(BIN_DIR)/viper-server $(INSTALL_DIR)/viper-server
	install -m 0755 $(BIN_DIR)/viper $(INSTALL_DIR)/viper

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
	rm -rf $(BIN_DIR) $(ARTIFACTS_DIR)

.PHONY: build-images
build-images: build-agent ## Build initramfs, fetch kernel, verify artifacts, and generate checksums
	mkdir -p $(ARTIFACTS_DIR)
	./build/images/build-initramfs.sh $(BIN_DIR)/viper-agent
	if [ ! -f "$(ARTIFACTS_DIR)/viper-initramfs.cpio.gz" ]; then \
		echo "Error: initramfs not built"; exit 1; \
	fi
	if [ ! -f "$(ARTIFACTS_DIR)/vmlinux-x86_64" ]; then \
		echo "Error: kernel not fetched"; exit 1; \
	fi
	(cd $(ARTIFACTS_DIR) && sha256sum viper-initramfs.cpio.gz vmlinux-x86_64 > checksums.txt)
	@if [ ! -f "$(ARTIFACTS_DIR)/viper-initramfs.cpio.gz" ] || [ ! -f "$(ARTIFACTS_DIR)/vmlinux-x86_64" ]; then \
		echo "Error: Artifacts verification failed"; exit 1; \
	fi
	@echo "Build images complete: $(ARTIFACTS_DIR)/{viper-initramfs.cpio.gz, vmlinux-x86_64, checksums.txt}"

.PHONY: setup
setup: build-cli ## Run viper setup (dry run)
	./bin/viper setup --dry-run