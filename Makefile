GO ?= go
BIN_DIR ?= bin
ARTIFACTS_DIR ?= build/artifacts

.PHONY: build
build: build-server build-agent build-cli ## Build all Viper binaries

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
	rm -rf $(BIN_DIR) $(ARTIFACTS_DIR)

.PHONY: build-images
build-images: build-agent ## Build initramfs, fetch kernel, verify artifacts, and generate checksums
	mkdir -p $(ARTIFACTS_DIR)
	./build/images/build-initramfs.sh $(BIN_DIR)/viper-agent
	if [ -f "build/images/viper-initramfs.cpio.gz" ]; then \
		mv build/images/viper-initramfs.cpio.gz $(ARTIFACTS_DIR)/; \
	else \
		echo "Error: initramfs not built"; exit 1; \
	fi
	if [ -f "build/images/vmlinuz-x86_64" ]; then \
		mv build/images/vmlinuz-x86_64 $(ARTIFACTS_DIR)/; \
	else \
		echo "Error: kernel not fetched"; exit 1; \
	fi
	(cd $(ARTIFACTS_DIR) && sha256sum viper-initramfs.cpio.gz vmlinuz-x86_64 > checksums.txt)
	@if [ ! -f "$(ARTIFACTS_DIR)/viper-initramfs.cpio.gz" ] || [ ! -f "$(ARTIFACTS_DIR)/vmlinuz-x86_64" ]; then \
		echo "Error: Artifacts verification failed"; exit 1; \
	fi
	@echo "Build images complete: $(ARTIFACTS_DIR)/{viper-initramfs.cpio.gz, vmlinuz-x86_64, checksums.txt}"

.PHONY: setup
setup: build-cli ## Run viper setup (dry run)
	./bin/viper setup --dry-run