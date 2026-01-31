# Dalang CLI

A command-line interface client for Dalang.io platform. Built with Go standard library.

## Installation

### Linux / macOS

```bash
curl -fsSL https://dalang.io/install.sh | sh
```

This script automatically:
- Detects your OS and architecture (amd64/arm64)
- Downloads the correct binary
- Installs to `/usr/local/bin/dalang`
- Verifies the installation

**Manual installation:**
```bash
# Linux (amd64)
curl -LO https://dalang.io/cli/dalang-linux-amd64
chmod +x dalang-linux-amd64
sudo mv dalang-linux-amd64 /usr/local/bin/dalang

# Linux (arm64)
curl -LO https://dalang.io/cli/dalang-linux-arm64
chmod +x dalang-linux-arm64
sudo mv dalang-linux-arm64 /usr/local/bin/dalang

# macOS (Apple Silicon)
curl -LO https://dalang.io/cli/dalang-darwin-arm64
chmod +x dalang-darwin-arm64
sudo mv dalang-darwin-arm64 /usr/local/bin/dalang

# macOS (Intel)
curl -LO https://dalang.io/cli/dalang-darwin-amd64
chmod +x dalang-darwin-amd64
sudo mv dalang-darwin-amd64 /usr/local/bin/dalang
```

### Windows

**PowerShell (Run as Administrator):**
```powershell
# Download latest binary
Invoke-WebRequest -Uri "https://dalang.io/cli/dalang-windows-amd64.exe" -OutFile "dalang.exe"

# Move to a directory in PATH (create if needed)
New-Item -ItemType Directory -Force -Path "$env:USERPROFILE\bin"
Move-Item -Force dalang.exe "$env:USERPROFILE\bin\dalang.exe"

# Add to PATH (if not already)
$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
if ($userPath -notlike "*$env:USERPROFILE\bin*") {
    [Environment]::SetEnvironmentVariable("Path", "$userPath;$env:USERPROFILE\bin", "User")
}

# Restart terminal, then verify
dalang version
```

**Or manually:**
1. Download `dalang-windows-amd64.exe` from [dalang.io/cli](https://dalang.io/cli/)
2. Rename to `dalang.exe`
3. Move to `C:\Windows\System32\` or add to PATH

### Verify Download (Optional)

```bash
# Download checksums
curl -LO https://dalang.io/cli/checksums.txt

# Verify (Linux/macOS)
sha256sum -c checksums.txt --ignore-missing

# Verify (Windows PowerShell)
Get-FileHash dalang.exe -Algorithm SHA256
# Compare with checksums.txt manually
```

### Uninstall

```bash
# Linux/macOS
sudo rm /usr/local/bin/dalang
rm -rf ~/.dalang

# Windows (PowerShell)
Remove-Item "$env:USERPROFILE\bin\dalang.exe"
Remove-Item -Recurse "$env:USERPROFILE\.dalang"
```

---

## Overview

Dalang CLI provides terminal-based access to Dalang services including:
- VPS/VM management
- Container services
- App hosting
- Credits/wallet management
- Interactive shell/console access

## Quick Start

```bash
# 1. Authenticate
dalang auth

# 2. Check your credit balance
dalang credit

# 3. List your services
dalang service list

# 4. Connect to a VM
dalang shell MyVM
```

## Authentication

Similar to Claude CLI, uses Device Authorization Grant flow:

```bash
# Step 1: Initiate auth
dalang auth
# Output: Visit https://dalang.io/auth/cli and enter code: ABCD-1234

# Step 2: User logs in via browser, enters code

# Step 3: CLI automatically detects authorization (polling)
# Output: Successfully authenticated as user@example.com
```

Your credentials are stored securely in `~/.dalang/credentials` (Linux/macOS) or `%USERPROFILE%\.dalang\credentials` (Windows).

## Commands

### General
```bash
dalang version           # Show CLI version and build info
dalang help              # Show help
```

### Authentication
```bash
dalang auth              # Start authentication flow
dalang auth status       # Check current auth status
dalang auth logout       # Clear stored credentials
```

### Credits/Wallet
```bash
dalang credit            # Show current credit balance
dalang credit history    # Show last 25 credit transactions
dalang credit add 50     # Top up 50K IDR (returns payment URL)
```

### Service Management
```bash
dalang service list      # List all services (VPS, containers, apps)
dalang service info MyVM # Show service details
dalang service create --name MyVM --cpu 2 --ram 512M --storage 5G --image ubuntu --bandwidth 20M
dalang service upgrade MyVM --cpu 4 --ram 1G --storage 8G --bandwidth 30M
```

### VM Operations
```bash
dalang shell MyVM        # Interactive shell (lxc exec)
dalang console MyVM      # VM console access
dalang start MyVM        # Start VM
dalang stop MyVM         # Stop VM
dalang delete MyVM       # Delete VM (requires confirmation)
```

## Security

All commands that modify or access services are restricted to resources owned by the authenticated user.

See [security.md](security.md) for security analysis and recommendations.
See [api-comparison.md](api-comparison.md) for API compatibility analysis.

---

---

## Backward Compatibility

All API changes are designed to be **backward compatible**. The frontend will continue to work without modifications.

### New Endpoints (No Frontend Impact)

| Endpoint | Purpose | Impact |
|----------|---------|--------|
| `POST /cli/auth/init` | Device auth - generate codes | None (CLI only) |
| `GET /cli/auth/poll` | Device auth - poll status | None (CLI only) |
| `POST /cli/auth/refresh` | Refresh expired token | None (CLI only) |
| `GET /vps/resolve` | Name → UUID lookup | None (new endpoint) |
| `GET /services/list` | Unified service list | None (new endpoint) |
| `POST /vps/order` | Direct VPS creation | None (new endpoint) |

### Modified Endpoints (Additive Changes Only)

| Endpoint | Change | Frontend Impact |
|----------|--------|-----------------|
| `GET /vps/detail` | Add optional `name` param | None - `id` still works |
| `POST /vps/action` | Add optional `name` param | None - existing params work |
| `DELETE /vps/delete` | Add optional `name` param | None - `id` still works |

### Implementation Rules

1. **Never remove parameters** - only add new optional ones
2. **Never change response structure** - only add new fields
3. **Use `/cli/*` prefix** for CLI-specific endpoints
4. **Keep existing auth flow** - CLI auth is separate from OAuth2
5. **New DB table only** - `cli_auth_codes` (no modifications to existing tables)

---

## Implementation Tasks

### Phase 1: API Endpoints (Backend)

#### Authentication Endpoints
- [x] Create `handlers/cli_auth.go`
- [x] `POST /cli/auth/init` - Generate device_code and user_code
- [x] `GET /cli/auth/poll` - Poll for authorization status
- [x] `POST /cli/auth/refresh` - Refresh expired access token
- [x] Create `cli_auth_codes` database table
- [ ] Add web page `/auth/cli` for user to enter code
- [x] Add routes to `main.go`

#### Name Resolution (Backward Compatible)
- [ ] `GET /vps/resolve?name=<name>` - Resolve display name to UUID (new endpoint)
- [ ] Add optional `name` parameter to `/vps/detail` (keep `id` working)
- [ ] Add optional `name` parameter to `/vps/action` (keep existing params)
- [ ] Add optional `name` parameter to `/vps/delete` (keep `id` working)

#### VPS Creation (Direct)
- [ ] `POST /vps/order` - Create VPS order with specs
- [ ] Support `pay_method: "credits"` for instant provisioning
- [ ] Support `pay_method: "xendit"` for invoice flow
- [ ] Trigger provisioning on credit payment

#### Unified Services
- [ ] `GET /services/list` - Return all services (VPS + containers + apps)

### Phase 2: CLI Core (Go) ✅

#### Project Setup
- [x] Initialize Go module (`go mod init dalang`)
- [x] Create project structure

#### Configuration
- [x] Config file location: `~/.dalang/config.json`
- [x] Credentials file: `~/.dalang/credentials` (mode 0600)
- [x] Create directories with proper permissions (0700)
- [x] Support `DALANG_API_URL` env var for development

#### API Client
- [x] HTTP client with TLS enforcement
- [x] Automatic `Authorization: Bearer <token>` header
- [x] Handle 401 (trigger re-auth)
- [x] Handle 429 (rate limit with backoff)
- [x] JSON request/response helpers

### Phase 3: CLI Commands ✅

#### General Commands
- [x] `dalang version` - Show version, build date, commit hash
- [x] `dalang help` - Show help and available commands

#### Auth Commands
- [x] `dalang auth` - Device authorization flow
- [x] `dalang auth status` - Show current auth state
- [x] `dalang auth logout` - Clear stored credentials

#### Credit Commands
- [x] `dalang credit` - Show balance
- [x] `dalang credit history` - Show transactions
- [x] `dalang credit add <amount>` - Top up credits

#### Service Commands
- [x] `dalang service list` - List all services
- [x] `dalang service info <name>` - Show detailed info
- [x] `dalang service create` - Create new VPS (requires API endpoint)
- [ ] `dalang service upgrade <name>` - Upgrade VPS (requires API endpoint)

#### VM Operation Commands
- [x] `dalang start <name>` - Start VM
- [x] `dalang stop <name>` - Stop VM
- [x] `dalang delete <name>` - Delete with confirmation

#### Terminal Commands
- [x] `dalang shell <name>` - WebSocket terminal (shell mode)
- [x] `dalang console <name>` - WebSocket terminal (console mode)

### Phase 4: Security & Polish ✅

#### Security Implementation
- [x] Secure token storage (file permissions 0600)
- [x] TLS certificate validation (no skip verify)
- [x] Confirmation prompts for destructive actions
- [x] `--yes` flag to skip confirmations

#### User Experience
- [x] Colored output (success=green, error=red, warning=yellow)
- [x] `--json` flag for machine-readable output
- [x] `--quiet` flag for minimal output
- [x] Per-command help (`dalang <command> --help`)

### Phase 5: Distribution ✅

#### Build & Release
- [x] Makefile with build targets
- [x] Cross-compilation (linux/darwin/windows, amd64/arm64)
- [x] Version embedding via ldflags
- [x] Strip debug symbols (`-s -w`)
- [x] Reproducible builds (`-trimpath`)

#### Release Artifacts
- [x] `dalang-linux-amd64`
- [x] `dalang-linux-arm64`
- [x] `dalang-darwin-amd64`
- [x] `dalang-darwin-arm64`
- [x] `dalang-windows-amd64.exe`
- [x] `checksums.txt` (SHA256)

#### Hosting (dalang.io)
- [ ] Host binaries at `https://dalang.io/cli/<binary-name>`
- [ ] Host `install.sh` at `https://dalang.io/install.sh`
- [ ] Host `checksums.txt` at `https://dalang.io/cli/checksums.txt`

#### Documentation
- [x] Installation instructions
- [x] Quick start guide
- [x] Command reference

---

## API Endpoints Required

### New Endpoints (To Implement)

| Endpoint | Priority | Breaking? |
|----------|----------|-----------|
| `POST /cli/auth/init` | P1 | No - new |
| `GET /cli/auth/poll` | P1 | No - new |
| `POST /cli/auth/refresh` | P1 | No - new |
| `GET /vps/resolve` | P2 | No - new |
| `POST /vps/order` | P3 | No - new |
| `GET /services/list` | P3 | No - new |

### Existing Endpoints (Ready to Use)

| Endpoint | Status | CLI Usage |
|----------|--------|-----------|
| `GET /credits/balance` | Ready | `dalang credit` |
| `GET /credits/transactions` | Ready | `dalang credit history` |
| `POST /credits/topup` | Ready | `dalang credit add` |
| `GET /vps/list` | Ready | `dalang service list` |
| `GET /vps/detail` | Ready (add `name` param) | `dalang service info` |
| `POST /vps/action` | Ready (add `name` param) | `dalang start/stop` |
| `DELETE /vps/delete` | Ready (add `name` param) | `dalang delete` |
| `WS /vps/terminal` | Ready | `dalang shell/console` |
| `GET /containers/list` | Ready | `dalang service list` |
| `GET /github/deployments` | Ready | `dalang service list` |

---

## Development

```bash
# Build
go build -o dalang .

# Run
./dalang --help

# Test
go test ./...
```

## License

Proprietary - Dalang.io
