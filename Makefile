# Dalang CLI Makefile

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_DATE := $(shell date -u +%Y-%m-%d)
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")

LDFLAGS := -s -w \
	-X main.Version=$(VERSION) \
	-X main.BuildDate=$(BUILD_DATE) \
	-X main.Commit=$(COMMIT)

BINARY_NAME := dalang
DIST_DIR := dist

# Build targets
PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64

.PHONY: all build clean test dist checksums help

all: build

## Build for current platform
build:
	go build -ldflags "$(LDFLAGS)" -o $(BINARY_NAME) .

## Build with debug info
build-debug:
	go build -ldflags "-X main.Version=$(VERSION) -X main.BuildDate=$(BUILD_DATE) -X main.Commit=$(COMMIT)" -o $(BINARY_NAME) .

## Run tests
test:
	go test -v ./...

## Clean build artifacts
clean:
	rm -rf $(DIST_DIR)
	rm -f $(BINARY_NAME)
	rm -f $(BINARY_NAME)-*
	rm -f checksums.txt

## Build for all platforms
dist: clean
	@mkdir -p $(DIST_DIR)
	@for platform in $(PLATFORMS); do \
		GOOS=$$(echo $$platform | cut -d'/' -f1); \
		GOARCH=$$(echo $$platform | cut -d'/' -f2); \
		output_name=$(BINARY_NAME)-$$GOOS-$$GOARCH; \
		if [ $$GOOS = "windows" ]; then output_name=$$output_name.exe; fi; \
		echo "Building $$output_name..."; \
		GOOS=$$GOOS GOARCH=$$GOARCH go build -trimpath -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$$output_name . || exit 1; \
	done
	@echo "Build complete!"
	@ls -la $(DIST_DIR)/

## Generate checksums
checksums: dist
	@echo "Generating checksums..."
	@cd $(DIST_DIR) && sha256sum dalang-* > checksums.txt
	@cat $(DIST_DIR)/checksums.txt

## Install locally
install: build
	sudo mv $(BINARY_NAME) /usr/local/bin/$(BINARY_NAME)
	@echo "Installed to /usr/local/bin/$(BINARY_NAME)"

## Uninstall
uninstall:
	sudo rm -f /usr/local/bin/$(BINARY_NAME)
	@echo "Uninstalled from /usr/local/bin/$(BINARY_NAME)"

## Show version info
version:
	@echo "Version: $(VERSION)"
	@echo "Build Date: $(BUILD_DATE)"
	@echo "Commit: $(COMMIT)"

## Show help
help:
	@echo "Dalang CLI Makefile"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@echo "  build       Build for current platform"
	@echo "  build-debug Build with debug info"
	@echo "  test        Run tests"
	@echo "  clean       Clean build artifacts"
	@echo "  dist        Build for all platforms"
	@echo "  checksums   Generate SHA256 checksums"
	@echo "  install     Install to /usr/local/bin"
	@echo "  uninstall   Remove from /usr/local/bin"
	@echo "  version     Show version info"
	@echo "  help        Show this help"
