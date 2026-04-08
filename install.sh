#!/bin/sh
# Dalang CLI Installer
# Usage: curl -fsSL https://raw.githubusercontent.com/dalang-io/dalang-cli/main/install.sh | sh

set -e

DOWNLOAD_BASE="https://github.com/dalang-io/dalang-cli/releases/latest/download"
BINARY_NAME="dalang"

INSTALL_DIR="/usr/local/bin"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

info() {
    printf "${BLUE}[INFO]${NC} %s\n" "$1"
}

success() {
    printf "${GREEN}[OK]${NC} %s\n" "$1"
}

warn() {
    printf "${YELLOW}[WARN]${NC} %s\n" "$1"
}

error() {
    printf "${RED}[ERROR]${NC} %s\n" "$1"
    exit 1
}

is_termux() {
    [ -n "${TERMUX_VERSION:-}" ] || [ "${PREFIX:-}" = "/data/data/com.termux/files/usr" ]
}

# Detect OS
detect_os() {
    if is_termux; then
        echo "android"
        return
    fi

    OS="$(uname -s)"
    case "$OS" in
        Linux*)  echo "linux" ;;
        Darwin*) echo "darwin" ;;
        *)       error "Unsupported OS: $OS. Use Windows instructions from README." ;;
    esac
}

# Detect architecture
detect_arch() {
    ARCH="$(uname -m)"
    case "$ARCH" in
        x86_64|amd64)   echo "amd64" ;;
        aarch64|arm64)  echo "arm64" ;;
        *)              error "Unsupported architecture: $ARCH" ;;
    esac
}

# Check for required commands
check_requirements() {
    for cmd in curl uname chmod mktemp; do
        if ! command -v "$cmd" >/dev/null 2>&1; then
            error "Required command not found: $cmd"
        fi
    done
}

get_install_dir() {
    if is_termux; then
        if [ -z "${PREFIX:-}" ]; then
            error "Termux detected but PREFIX is not set."
        fi
        echo "${PREFIX}/bin"
        return
    fi

    echo "/usr/local/bin"
}


main() {
    echo ""
    echo "  ____        _                    "
    echo " |  _ \  __ _| | __ _ _ __   __ _  "
    echo " | | | |/ _\` | |/ _\` | '_ \\ / _\` | "
    echo " | |_| | (_| | | (_| | | | | (_| | "
    echo " |____/ \\__,_|_|\\__,_|_| |_|\\__, | "
    echo "                           |___/   CLI Installer"
    echo ""

    check_requirements

    OS=$(detect_os)
    ARCH=$(detect_arch)
    INSTALL_DIR=$(get_install_dir)

    info "Detected: ${OS}/${ARCH}"

    # Construct download URL
    BINARY="dalang-${OS}-${ARCH}"
    DOWNLOAD_URL="${DOWNLOAD_BASE}/${BINARY}"

    info "Downloading from: ${DOWNLOAD_URL}"

    # Create temp directory
    TMP_DIR=$(mktemp -d)
    TMP_FILE="${TMP_DIR}/${BINARY_NAME}"

    # Download binary
    if ! curl -fsSL "$DOWNLOAD_URL" -o "$TMP_FILE"; then
        rm -rf "$TMP_DIR"
        error "Failed to download binary. Check your internet connection."
    fi

    success "Downloaded successfully"

    # Make executable
    chmod +x "$TMP_FILE"

    # Check if we need sudo
    NEED_SUDO=""
    if is_termux; then
        info "Termux environment detected"
        info "Installing to ${INSTALL_DIR}"
    elif [ ! -w "$INSTALL_DIR" ]; then
        NEED_SUDO="sudo"
        info "Installing to ${INSTALL_DIR} (requires sudo)"
    else
        info "Installing to ${INSTALL_DIR}"
    fi

    # Ensure install directory exists
    if [ ! -d "$INSTALL_DIR" ]; then
        if [ -n "$NEED_SUDO" ]; then
            sudo mkdir -p "$INSTALL_DIR"
        else
            mkdir -p "$INSTALL_DIR"
        fi
    fi

    # Install
    if [ -n "$NEED_SUDO" ]; then
        if ! command -v sudo >/dev/null 2>&1; then
            error "Cannot write to ${INSTALL_DIR} and sudo is not available. Try running as root."
        fi
        sudo mv "$TMP_FILE" "${INSTALL_DIR}/${BINARY_NAME}"
    else
        mv "$TMP_FILE" "${INSTALL_DIR}/${BINARY_NAME}"
    fi

    # Remove macOS quarantine attribute
    if [ "$OS" = "darwin" ]; then
        if [ -n "$NEED_SUDO" ]; then
            sudo xattr -d com.apple.quarantine "${INSTALL_DIR}/${BINARY_NAME}" 2>/dev/null || true
        else
            xattr -d com.apple.quarantine "${INSTALL_DIR}/${BINARY_NAME}" 2>/dev/null || true
        fi
    fi

    # Cleanup
    rm -rf "$TMP_DIR"

    # Verify installation
    if command -v dalang >/dev/null 2>&1; then
        success "Dalang CLI installed successfully!"
        echo ""
        info "Run 'dalang auth' to get started"
        echo ""
    else
        warn "Installed but 'dalang' not found in PATH"
        warn "You may need to add ${INSTALL_DIR} to your PATH"
        echo ""
        echo "Add this to your ~/.bashrc or ~/.zshrc:"
        echo "  export PATH=\"\$PATH:${INSTALL_DIR}\""
        echo ""
    fi
}

main "$@"
