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
- [ ] Create `handlers/cli_auth.go`
- [ ] `POST /cli/auth/init` - Generate device_code and user_code
- [ ] `GET /cli/auth/poll` - Poll for authorization status
- [ ] `POST /cli/auth/refresh` - Refresh expired access token
- [ ] Create `cli_auth_codes` database table
- [ ] Add web page `/auth/cli` for user to enter code
- [ ] Add routes to `main.go`

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

### Phase 2: CLI Core (Go)

#### Project Setup
- [ ] Initialize Go module (`go mod init dalang`)
- [ ] Create project structure:
  ```
  dalang/
  ├── main.go
  ├── cmd/
  │   ├── root.go
  │   ├── auth.go
  │   ├── credit.go
  │   ├── service.go
  │   └── shell.go
  ├── internal/
  │   ├── api/
  │   │   └── client.go
  │   ├── auth/
  │   │   └── store.go
  │   ├── config/
  │   │   └── config.go
  │   └── terminal/
  │       └── websocket.go
  └── go.mod
  ```

#### Configuration
- [ ] Config file location: `~/.dalang/config.json`
- [ ] Credentials file: `~/.dalang/credentials` (mode 0600)
- [ ] Create directories with proper permissions (0700)
- [ ] Support `DALANG_API_URL` env var for development

#### API Client
- [ ] HTTP client with TLS enforcement
- [ ] Automatic `Authorization: Bearer <token>` header
- [ ] Handle 401 (trigger re-auth)
- [ ] Handle 429 (rate limit with backoff)
- [ ] JSON request/response helpers

### Phase 3: CLI Commands

#### General Commands
- [ ] `dalang version` - Show version, build date, commit hash
  - [ ] Embed version via ldflags at build time
  - [ ] Format: `dalang version 1.0.0 (build 2024-01-15, commit abc1234)`
- [ ] `dalang help` - Show help and available commands

#### Auth Commands
- [ ] `dalang auth` - Device authorization flow
  - [ ] Call `POST /cli/auth/init`
  - [ ] Display verification URL and user code
  - [ ] Poll `GET /cli/auth/poll` every 5 seconds
  - [ ] Store tokens on success
  - [ ] Handle timeout (10 minutes)
- [ ] `dalang auth status` - Show current auth state
- [ ] `dalang auth logout` - Clear stored credentials

#### Credit Commands
- [ ] `dalang credit` - Call `GET /credits/balance`, display formatted
- [ ] `dalang credit history` - Call `GET /credits/transactions?limit=25`
- [ ] `dalang credit add <amount>` - Call `POST /credits/topup`
  - [ ] Validate minimum 50K
  - [ ] Open payment URL in browser or display

#### Service Commands
- [ ] `dalang service list` - Fetch and merge VPS/containers/apps
  - [ ] Call `GET /vps/list`
  - [ ] Call `GET /containers/list`
  - [ ] Call `GET /github/deployments`
  - [ ] Format as table with type, name, status, expiry
- [ ] `dalang service info <name>` - Show detailed info
  - [ ] Resolve name to UUID
  - [ ] Call appropriate detail endpoint
  - [ ] Display formatted output
- [ ] `dalang service create` - Create new VPS
  - [ ] Parse flags: --name, --cpu, --ram, --storage, --image, --bandwidth, --region
  - [ ] Calculate/display price
  - [ ] Confirm before creating
  - [ ] Call `POST /vps/order`
- [ ] `dalang service upgrade <name>` - Upgrade VPS
  - [ ] Parse upgrade flags
  - [ ] Show price difference
  - [ ] Confirm before upgrading
  - [ ] Call `POST /vps/upgrade`

#### VM Operation Commands
- [ ] `dalang start <name>` - Call `POST /vps/action {action: "start"}`
- [ ] `dalang stop <name>` - Call `POST /vps/action {action: "stop"}`
- [ ] `dalang delete <name>` - Delete with confirmation
  - [ ] Require `--yes` flag or interactive confirmation
  - [ ] Call `DELETE /vps/delete`

#### Terminal Commands
- [ ] `dalang shell <name>` - WebSocket terminal (shell mode)
  - [ ] Resolve name to UUID
  - [ ] Connect to `wss://api.dalang.io/vps/terminal?uuid=X&token=Y&mode=shell`
  - [ ] Use `golang.org/x/term` for raw mode
  - [ ] Handle SIGWINCH for resize
  - [ ] Send resize messages as JSON
  - [ ] Restore terminal on exit
- [ ] `dalang console <name>` - WebSocket terminal (console mode)
  - [ ] Same as shell but `mode=console`

### Phase 4: Security & Polish

#### Security Implementation
- [ ] Secure token storage (file permissions 0600)
- [ ] Token refresh before expiry
- [ ] TLS certificate validation (no skip verify)
- [ ] Input validation for service names
- [ ] Sanitize terminal output (escape sequences)
- [ ] Confirmation prompts for destructive actions
- [ ] `--yes` flag to skip confirmations

#### User Experience
- [ ] Colored output (success=green, error=red, warning=yellow)
- [ ] Progress indicators for long operations
- [ ] `--json` flag for machine-readable output
- [ ] `--quiet` flag for minimal output
- [ ] Helpful error messages with suggestions
- [ ] Per-command help (`dalang <command> --help`)

#### Testing
- [ ] Unit tests for API client
- [ ] Unit tests for auth storage
- [ ] Integration tests with mock server
- [ ] Manual testing checklist

### Phase 5: Distribution

#### Build & Release
- [ ] Makefile with build targets
- [ ] Cross-compilation (linux/darwin/windows, amd64/arm64)
- [ ] Version embedding via ldflags:
  ```bash
  go build -ldflags "-s -w -X main.Version=1.0.0 -X main.BuildDate=$(date -u +%Y-%m-%d) -X main.Commit=$(git rev-parse --short HEAD)"
  ```
- [ ] Strip debug symbols (`-s -w`)
- [ ] Reproducible builds (`-trimpath`)

#### Release Artifacts
- [ ] `dalang-linux-amd64`
- [ ] `dalang-linux-arm64`
- [ ] `dalang-darwin-amd64`
- [ ] `dalang-darwin-arm64`
- [ ] `dalang-windows-amd64.exe`
- [ ] `checksums.txt` (SHA256)

#### Hosting (dalang.io)
- [ ] Host binaries at `https://dalang.io/cli/<binary-name>`
- [ ] Host `install.sh` at `https://dalang.io/install.sh`
- [ ] Host `checksums.txt` at `https://dalang.io/cli/checksums.txt`
- [ ] Add download page at `https://dalang.io/cli/` (optional)

#### Documentation
- [ ] Installation instructions
- [ ] Quick start guide
- [ ] Command reference
- [ ] Troubleshooting guide

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
