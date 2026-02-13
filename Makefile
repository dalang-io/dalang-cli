# Dalang CLI Makefile (Rust)

BINARY_NAME := dalang
DIST_DIR := dist

# Cross-compilation targets
# Requires: rustup target add <triple> for each target
TARGETS := \
	x86_64-unknown-linux-musl \
	aarch64-unknown-linux-musl \
	aarch64-linux-android \
	x86_64-apple-darwin \
	aarch64-apple-darwin \
	x86_64-pc-windows-gnu

# Map Rust triples to Go-style names for binary output
define target_to_name
$(if $(findstring x86_64-unknown-linux-musl,$1),linux-amd64,\
$(if $(findstring aarch64-unknown-linux-musl,$1),linux-arm64,\
$(if $(findstring aarch64-linux-android,$1),android-arm64,\
$(if $(findstring x86_64-apple-darwin,$1),darwin-amd64,\
$(if $(findstring aarch64-apple-darwin,$1),darwin-arm64,\
$(if $(findstring x86_64-pc-windows-gnu,$1),windows-amd64,unknown))))))
endef

define target_ext
$(if $(findstring windows,$1),.exe,)
endef

.PHONY: all build build-debug clean test dist checksums install uninstall version help

all: build

## Build for current platform (release)
build:
	cargo build --release
	cp target/release/$(BINARY_NAME) $(BINARY_NAME)
	@if command -v upx >/dev/null 2>&1; then upx --best --lzma $(BINARY_NAME); fi

## Build with debug info
build-debug:
	cargo build
	cp target/debug/$(BINARY_NAME) $(BINARY_NAME)

## Run tests
test:
	cargo test

## Clean build artifacts
clean:
	cargo clean
	rm -rf $(DIST_DIR)
	rm -f $(BINARY_NAME)
	rm -f $(BINARY_NAME)-*
	rm -f checksums.txt

## Build for all platforms
dist: clean
	@mkdir -p $(DIST_DIR)
	@for target in $(TARGETS); do \
		name=$$(echo $$target | sed \
			-e 's/x86_64-unknown-linux-musl/linux-amd64/' \
			-e 's/aarch64-unknown-linux-musl/linux-arm64/' \
			-e 's/aarch64-linux-android/android-arm64/' \
			-e 's/x86_64-apple-darwin/darwin-amd64/' \
			-e 's/aarch64-apple-darwin/darwin-arm64/' \
			-e 's/x86_64-pc-windows-gnu/windows-amd64/'); \
		ext=""; \
		if echo "$$target" | grep -q windows; then ext=".exe"; fi; \
		output_name=$(BINARY_NAME)-$$name$$ext; \
		echo "Building $$output_name ($$target)..."; \
		cargo build --release --target $$target || exit 1; \
		cp target/$$target/release/$(BINARY_NAME)$$ext $(DIST_DIR)/$$output_name || exit 1; \
		if command -v upx >/dev/null 2>&1; then upx --best --lzma $(DIST_DIR)/$$output_name 2>/dev/null || true; fi; \
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
	@cargo run --release -- version

## Show help
help:
	@echo "Dalang CLI Makefile (Rust)"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@echo "  build       Build for current platform (release)"
	@echo "  build-debug Build with debug info"
	@echo "  test        Run tests"
	@echo "  clean       Clean build artifacts"
	@echo "  dist        Build for all platforms"
	@echo "  checksums   Generate SHA256 checksums"
	@echo "  install     Install to /usr/local/bin"
	@echo "  uninstall   Remove from /usr/local/bin"
	@echo "  version     Show version info"
	@echo "  help        Show this help"
	@echo ""
	@echo "Cross-compilation targets:"
	@echo "  x86_64-unknown-linux-musl   (linux/amd64)"
	@echo "  aarch64-unknown-linux-musl  (linux/arm64)"
	@echo "  aarch64-linux-android       (android/arm64)"
	@echo "  x86_64-apple-darwin         (darwin/amd64)"
	@echo "  aarch64-apple-darwin        (darwin/arm64)"
	@echo "  x86_64-pc-windows-gnu       (windows/amd64)"
	@echo ""
	@echo "Add targets with: rustup target add <triple>"
