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
build-server: ## Build the hyped binary
	$(GO) build -o $(BIN_DIR)/hyped ./cmd/hyped

.PHONY: build-agent
build-agent:
	$(GO) build -o $(BIN_DIR)/hype-agent ./cmd/hype-agent

.PHONY: build-cli
build-cli:
	$(GO) build -o $(BIN_DIR)/hype ./cmd/hype

.PHONY: install-binaries
install-binaries: build-server build-cli ## Install hyped and overhyped to INSTALL_DIR
	mkdir -p $(INSTALL_DIR)
	install -m 0755 $(BIN_DIR)/hyped $(INSTALL_DIR)/hyped
	install -m 0755 $(BIN_DIR)/hype $(INSTALL_DIR)/hype

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
	./build/images/build-initramfs.sh $(BIN_DIR)/hype-agent
	if [ ! -f "$(ARTIFACTS_DIR)/overhyped-initramfs.cpio.gz" ]; then \
		echo "Error: initramfs not built"; exit 1; \
	fi
	if [ ! -f "$(ARTIFACTS_DIR)/vmlinux-x86_64" ]; then \
		echo "Error: kernel not fetched"; exit 1; \
	fi
	(cd $(ARTIFACTS_DIR) && sha256sum overhyped-initramfs.cpio.gz vmlinux-x86_64 > checksums.txt)
	@if [ ! -f "$(ARTIFACTS_DIR)/overhyped-initramfs.cpio.gz" ] || [ ! -f "$(ARTIFACTS_DIR)/vmlinux-x86_64" ]; then \
		echo "Error: Artifacts verification failed"; exit 1; \
	fi
	@echo "Build images complete: $(ARTIFACTS_DIR)/{overhyped-initramfs.cpio.gz, vmlinux-x86_64, checksums.txt}"

.PHONY: setup
setup: build-cli ## Run hype setup (dry run)
	./bin/hype setup --dry-run