# Dalang CLI

Command-line interface for [Dalang.io](https://dalang.io) cloud platform. Manage VPS instances, containers, credits, and more from your terminal.

## Installation

Prebuilt binaries are published on [GitHub Releases](https://github.com/dalang-io/dalang-cli/releases) for tagged versions.

### Linux / macOS / Termux

```bash
curl -fsSL https://raw.githubusercontent.com/dalang-io/dalang-cli/main/install.sh | sh
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

# Android / Termux (ARM64)
curl -LO https://github.com/dalang-io/dalang-cli/releases/latest/download/dalang-android-arm64
chmod +x dalang-android-arm64
mv dalang-android-arm64 $PREFIX/bin/dalang

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
irm https://raw.githubusercontent.com/dalang-io/dalang-cli/main/install.ps1 | iex
```

Or download manually from [GitHub Releases](https://github.com/dalang-io/dalang-cli/releases).

### Termux Notes

The install script is the recommended path on Termux — it auto-detects the
environment, installs to `$PREFIX/bin` (so `dalang` lands on your `$PATH`),
sets the executable bit, and skips `sudo`:

```bash
curl -fsSL https://raw.githubusercontent.com/dalang-io/dalang-cli/main/install.sh | bash
```

If you prefer manual install, use the **Android / Termux (ARM64)** block
above verbatim. Common failure modes when downloading by hand:

- Forgot `chmod +x` → `Permission denied` on first run.
- Moved the binary to `~/bin` or `$HOME` instead of `$PREFIX/bin` → not on
  Termux's `$PATH`, so `dalang` is "not found" even though it exists.
- Used `sudo` → not available in stock Termux; the install script
  intentionally skips it.

### Available Release Assets

Tagged releases publish these binaries:

- `dalang-linux-amd64`
- `dalang-linux-arm64`
- `dalang-android-arm64`
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
dalang auth              # Login via browser (device flow)
dalang auth status       # Check current login
dalang auth logout       # Clear credentials
```

Your session is refreshed automatically as it nears expiry, so you rarely need
to log in again.

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
dalang shell <name>          # Interactive shell (shows uptime/CPU/mem/disk on connect)
dalang exec <name> "cmd"     # Run a command and return its output
dalang console <name>        # Console access
dalang start <name>          # Start VM
dalang stop <name>           # Stop VM
dalang delete <name>         # Delete VM
```

VM names are matched case-insensitively, and a close match is suggested if the
name isn't found (e.g. `dalang shell binuss` → "Did you mean: binus?").

### File Transfer (scp-style)

```bash
# Upload a single file
dalang scp ./app.tar.gz MyVM:/opt/app.tar.gz

# Download a single file
dalang scp MyVM:/etc/nginx/nginx.conf ./nginx.conf

# Multiple sources to a directory
dalang scp file1.txt file2.txt MyVM:/tmp/

# Recursive upload of a project tree
dalang scp -r ./project MyVM:/srv/project

# Recursive download with mode/mtime preserved
dalang scp -r -p MyVM:/var/log ./vm-logs

# Quiet mode (no progress bars)
dalang scp -q ./big.tar.gz MyVM:/tmp/big.tar.gz
```

`scp` accepts the same `<vps-name>:<absolute-path>` syntax as OpenSSH's `scp`.
Direction is inferred from which operand carries the host prefix; the last
positional argument is always the destination. Authorization mirrors `dalang
shell`/`exec` — owner, group-shared, or admin.

The legacy `dalang upload` and `dalang download` commands still work but are
single-file only; new code should prefer `scp`.

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
dalang update            # Update to latest version (shows a download progress bar)
dalang version           # Show version (also: dalang -V)
dalang help              # Show help
dalang help <command>    # Command-specific help
```

## Global Options

| Flag | Description |
|------|-------------|
| `--json` | Output in JSON format |
| `--quiet`, `-q` | Minimal output |
| `--yes`, `-y` | Skip confirmations |
| `--no-color` | Disable colored output |
| `--verbose`, `-v` | Debug output |
| `-V`, `--version` | Show version |

Colors are automatically disabled when output is piped/redirected or when the
`NO_COLOR` environment variable is set.

> Note: `-v` means **verbose**. Use `-V` or `dalang version` for the version.

## Shell/Console Tips

- Type `exit` to end the session and return to your local terminal
- Alternatively press Enter, then type `~.` (tilde + dot) to force-disconnect
- On connect, the shell prints a resource panel (uptime, CPU load, memory, disk)
- Ctrl+C sends to the remote VM; `~.` or `exit` is how you leave
- Terminal is in raw mode for full interactivity

## Updating

```bash
dalang update

# Or with sudo if installed system-wide
sudo dalang update

# In Termux, sudo is not needed
dalang update
```

GitHub release assets are available at:

- [https://github.com/dalang-io/dalang-cli/releases](https://github.com/dalang-io/dalang-cli/releases)

## Uninstall

```bash
# Linux/macOS
sudo rm /usr/local/bin/dalang
rm -rf ~/.dalang

# Termux
rm -f $PREFIX/bin/dalang
rm -rf ~/.dalang

# Windows
del %USERPROFILE%\bin\dalang.exe
rmdir /s %USERPROFILE%\.dalang
```

## Configuration

Credentials are stored in:
- Linux/macOS: `~/.dalang/credentials`
- Android/Termux: `~/.dalang/credentials`
- Windows: `%USERPROFILE%\.dalang\credentials`

## Environment Variables

| Variable | Description |
|----------|-------------|
| `DALANG_API_URL` | Override API URL (for development) |
| `NO_COLOR` | Disable colored output when set (any value) |

## License

Proprietary - [Dalang.io](https://dalang.io)
