# Dalang CLI

Command-line interface for [Dalang.io](https://dalang.io) cloud platform. Manage VPS instances, containers, credits, and more from your terminal.

## Installation

Prebuilt binaries are published on [GitHub Releases](https://github.com/dalang-io/dalang-cli/releases) for tagged versions.

### Linux / macOS

```bash
curl -fsSL https://dalang.io/install.sh | sh
```

Or download manually from [GitHub Releases](https://github.com/dalang-io/dalang-cli/releases):

```bash
# Linux (x86_64)
curl -LO https://github.com/dalang-io/dalang-cli/releases/latest/download/dalang-linux-amd64
chmod +x dalang-linux-amd64
sudo mv dalang-linux-amd64 /usr/local/bin/dalang

# Linux (ARM64)
curl -LO https://github.com/dalang-io/dalang-cli/releases/latest/download/dalang-linux-arm64
chmod +x dalang-linux-arm64
sudo mv dalang-linux-arm64 /usr/local/bin/dalang

# macOS (Apple Silicon)
curl -LO https://github.com/dalang-io/dalang-cli/releases/latest/download/dalang-darwin-arm64
chmod +x dalang-darwin-arm64
sudo mv dalang-darwin-arm64 /usr/local/bin/dalang

# macOS (Intel)
curl -LO https://github.com/dalang-io/dalang-cli/releases/latest/download/dalang-darwin-amd64
chmod +x dalang-darwin-amd64
sudo mv dalang-darwin-amd64 /usr/local/bin/dalang
```

### Windows

```powershell
irm https://dalang.io/install.ps1 | iex
```

Or download manually from [GitHub Releases](https://github.com/dalang-io/dalang-cli/releases).

### Available Release Assets

Tagged releases publish these binaries:

- `dalang-linux-amd64`
- `dalang-linux-arm64`
- `dalang-darwin-amd64`
- `dalang-darwin-arm64`
- `dalang-windows-amd64.exe`
- `checksums.txt`

### Verify Installation

```bash
dalang version
```

## Quick Start

```bash
# 1. Login to your Dalang account
dalang auth

# 2. Check your credit balance
dalang credit

# 3. List your services
dalang service list

# 4. Connect to a VM
dalang shell my-vm
```

## Commands

### Authentication

```bash
dalang auth              # Login via browser
dalang auth logout       # Clear credentials
```

### Credits

```bash
dalang credit            # Show balance
dalang credit history    # Transaction history
dalang credit add 100    # Top up 100K IDR
```

### Services

```bash
dalang service list              # List all services
dalang service info <name>       # Service details
dalang service create            # Create new VPS
```

### VM Operations

```bash
dalang shell <name>      # Interactive shell
dalang console <name>    # Console access
dalang start <name>      # Start VM
dalang stop <name>       # Stop VM
dalang delete <name>     # Delete VM
```

### Custom Domains

```bash
dalang domain enable <vps>           # Enable addon
dalang domain list <vps>             # List domains
dalang domain add <vps> <domain>     # Add domain
dalang domain verify <domain>        # Verify DNS
dalang domain remove <domain>        # Remove domain
```

### Other

```bash
dalang update            # Update to latest version
dalang version           # Show version
dalang help              # Show help
dalang help <command>    # Command-specific help
```

## Global Options

| Flag | Description |
|------|-------------|
| `--json` | Output in JSON format |
| `--quiet`, `-q` | Minimal output |
| `--yes`, `-y` | Skip confirmations |
| `--verbose`, `-v` | Debug output |

## Shell/Console Tips

- Press Enter, then type `~.` (tilde + dot) to disconnect
- Ctrl+C sends to remote VM, not to disconnect
- Terminal is in raw mode for full interactivity

## Updating

```bash
dalang update

# Or with sudo if installed system-wide
sudo dalang update
```

GitHub release assets are available at:

- [https://github.com/dalang-io/dalang-cli/releases](https://github.com/dalang-io/dalang-cli/releases)

## Uninstall

```bash
# Linux/macOS
sudo rm /usr/local/bin/dalang
rm -rf ~/.dalang

# Windows
del %USERPROFILE%\bin\dalang.exe
rmdir /s %USERPROFILE%\.dalang
```

## Configuration

Credentials are stored in:
- Linux/macOS: `~/.dalang/credentials`
- Windows: `%USERPROFILE%\.dalang\credentials`

## Environment Variables

| Variable | Description |
|----------|-------------|
| `DALANG_API_URL` | Override API URL (for development) |

## License

Proprietary - [Dalang.io](https://dalang.io)
