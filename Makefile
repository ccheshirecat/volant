GO ?= go
INSTALL_DIR ?= /usr/local/bin
BIN_DIR ?= bin
ARTIFACTS_DIR ?= build/artifacts
ENGINE_ARTIFACTS ?= $(ARTIFACTS_DIR)/engine
BROWSER_ARTIFACTS ?= $(ARTIFACTS_DIR)/browser
BROWSER_IMAGE_TAG ?= volant-browser-runtime:latest
BROWSER_INITRAMFS ?= browser-runtime-initramfs.cpio.gz

.PHONY: help
help: ## Show available make targets
	@grep -E '^[a-zA-Z_-]+:.*##' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*##"} {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

.PHONY: build
build: build-server build-agent build-cli install-binaries ## Build and install core binaries

.PHONY: build-server
build-server: ## Build the volantd binary
	$(GO) build -o $(BIN_DIR)/volantd ./cmd/volantd

.PHONY: build-agent
build-agent:
	$(GO) build -o $(BIN_DIR)/volary ./cmd/volary

.PHONY: build-cli
build-cli:
	$(GO) build -o $(BIN_DIR)/volar ./cmd/volar

.PHONY: install-binaries
install-binaries: build-server build-cli ## Install volantd and volant to INSTALL_DIR
	mkdir -p $(INSTALL_DIR)
	install -m 0755 $(BIN_DIR)/volantd $(INSTALL_DIR)/volantd
	install -m 0755 $(BIN_DIR)/volar $(INSTALL_DIR)/volar

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

.PHONY: build-artifacts
build-artifacts: build-engine-artifacts build-browser-artifacts ## Produce engine and browser artifact bundles

.PHONY: build-engine-artifacts
build-engine-artifacts: build-server build-cli ## Bundle engine binaries and checksums
	mkdir -p $(ENGINE_ARTIFACTS)
	cp $(BIN_DIR)/volantd $(ENGINE_ARTIFACTS)/
	cp $(BIN_DIR)/volar $(ENGINE_ARTIFACTS)/
	sha256sum $(ENGINE_ARTIFACTS)/volantd $(ENGINE_ARTIFACTS)/volar > $(ENGINE_ARTIFACTS)/checksums.txt
	@echo "Engine artifacts written to $(ENGINE_ARTIFACTS)"

.PHONY: build-browser-artifacts
build-browser-artifacts: build-agent ## Build browser runtime initramfs + kernel snapshot
	mkdir -p $(BROWSER_ARTIFACTS)
	OUTPUT_DIR=$(BROWSER_ARTIFACTS) \
	IMAGE_TAG=$(BROWSER_IMAGE_TAG) \
	INITRAMFS_NAME=$(BROWSER_INITRAMFS) \
		./build/images/build-initramfs.sh $(BIN_DIR)/volary
	cd $(BROWSER_ARTIFACTS) && sha256sum $(BROWSER_INITRAMFS) vmlinux-x86_64 > checksums.txt
	@echo "Browser artifacts written to $(BROWSER_ARTIFACTS)"

.PHONY: build-images
build-images: build-browser-artifacts ## (deprecated) retained for compatibility
	@echo "build-images target now delegates to build-browser-artifacts"

.PHONY: setup
setup: build-cli ## Run volar setup (dry run)
	./bin/volar setup --dry-run