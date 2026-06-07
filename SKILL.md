# Dalang CLI - AI Skill Guide

This document helps AI assistants understand and use the Dalang CLI tool effectively.

_Up to date with Dalang CLI **v1.11.x**._

## Overview

Dalang CLI is a single-binary command-line interface for managing cloud services on Dalang.io: VPS instances, containers, credits, custom domains, and file transfer. It talks to `api.dalang.io` over HTTPS and authenticates with a bearer token stored in `~/.dalang/`.

## AI Agent Autonomy & Self-Replication

Dalang CLI is designed for autonomous AI agents that need to provision and manage their own infrastructure. Key capabilities:

### Self-Replication Pattern
```bash
# 1. Create a new VM for the replica
dalang service create --name agent-replica-001 --cpu 2 --ram 2G --storage 20G --image ubuntu:24.04 -y

# 2. Wait until it is RUNNING
until [ "$(dalang service info agent-replica-001 --json | jq -r .status)" = "RUNNING" ]; do sleep 5; done

# 3. Install dependencies and clone yourself — non-interactively, via exec
dalang exec agent-replica-001 "apt-get update && apt-get install -y curl git python3 python3-pip"
dalang exec agent-replica-001 "git clone https://github.com/your-org/your-agent.git /opt/agent"
dalang exec agent-replica-001 "cd /opt/agent && pip3 install -r requirements.txt"

# 4. Start the agent process (detached)
dalang exec agent-replica-001 "cd /opt/agent && nohup python3 main.py > /var/log/agent.log 2>&1 &"
```

> Use `dalang exec` (not `dalang shell`) for unattended automation. `shell` opens an
> interactive raw-mode terminal; `exec` runs a single command in a persistent session
> and returns its output, which is what scripts and AI agents want.

### Autonomous Infrastructure Control
- **Create VMs programmatically**: Spawn new instances without human intervention
- **Self-healing**: Detect failed instances and recreate them
- **Horizontal scaling**: Create multiple replicas based on workload
- **Resource management**: Monitor and adjust VM specs as needed (`dalang service upgrade`)

### Example: OpenClaw-style Self-Replication
```bash
#!/bin/bash
# Autonomous agent self-replication script

REPLICA_NAME="agent-$(date +%s)"

# Create new VM
dalang service create --name "$REPLICA_NAME" --cpu 2 --ram 2G --storage 20G -y

# Wait for provisioning
until [ "$(dalang service info "$REPLICA_NAME" --json | jq -r .status)" = "RUNNING" ]; do sleep 5; done

# Deploy agent to the new VM via exec
dalang exec "$REPLICA_NAME" "apt-get update && apt-get install -y python3 python3-pip git"
dalang exec "$REPLICA_NAME" "pip3 install openai anthropic"
dalang exec "$REPLICA_NAME" "git clone https://github.com/your-org/your-agent.git /opt/agent"
dalang exec "$REPLICA_NAME" "cd /opt/agent && nohup python3 main.py > /var/log/agent.log 2>&1 &"

echo "Replica $REPLICA_NAME deployed successfully"
```

### JSON Output for Programmatic Control
```bash
# Most commands support --json for easy parsing
dalang service list --json | jq '.[] | select(.status == "RUNNING")'
dalang service info MyVM --json | jq -r '.public_ip'
dalang credit --json | jq -r '.balance'
dalang exec MyVM "df -h /" --json | jq -r '.output'
```

### Automated Credential Management
Credentials are stored in `~/.dalang/credentials` (mode 0600) and persist across sessions.
The CLI **refreshes the session automatically** as the token nears expiry, so an agent
authenticates once and keeps operating without re-login.

## Authentication

Before using any command, the user must be authenticated:

```bash
dalang auth              # Start authentication flow (device/browser)
dalang auth status       # Show current login state
dalang auth logout       # Logout and clear credentials
```

Authentication uses an OAuth-style device authorization flow — the user gets a code to
enter at dalang.io/auth/cli. Tokens are refreshed proactively near expiry.

## Common Workflows

### Check Account Status
```bash
dalang credit            # Show balance
dalang credit history    # Show transactions
dalang service list      # List all services
```

### Manage VPS
```bash
# List and inspect
dalang service list
dalang service info <vps-name>

# Control VM state
dalang start <vps-name>
dalang stop <vps-name>
dalang delete <vps-name>

# Connect to / run commands on the VM
dalang shell <vps-name>            # Interactive shell (persistent tmux session)
dalang exec <vps-name> "command"   # Run one command, return its output
dalang console <vps-name>          # Console connection
```

VM names are matched **case-insensitively**, and a close match is suggested when a
name isn't found (e.g. `dalang shell binuss` → "Did you mean: binus?").

### Check VPS Pricing
```bash
# Show pricing table
dalang price

# Calculate price for a specific configuration
dalang price --cpu 2 --ram 2G --storage 20G --bandwidth 40
```

### Create New VPS
```bash
# Basic VM with Ubuntu 24.04 (default)
dalang service create --name MyVM --cpu 2 --ram 1G --storage 10G

# Specify OS version
dalang service create --name WebServer --cpu 1 --ram 1G --image ubuntu:24.04
dalang service create --name DevBox --cpu 2 --ram 2G --image ubuntu:22.04
dalang service create --name Database --cpu 2 --ram 4G --image debian:12
```

### Upgrade or Extend a VPS
```bash
# Upgrade specs (new values must be higher than current; paid from credits)
dalang service upgrade MyVM --cpu 4 --ram 4G --storage 50G

# Extend the subscription by N months (paid from credits)
dalang service extend MyVM --months 3
```

Both commands create an invoice that is settled from the account's credit balance and
prompt for confirmation (skip with `-y`).

## Available OS Images

All images support incus-agent for shell/console access.

| Image Name | Description | Cloud-Init |
|------------|-------------|------------|
| `ubuntu` | Ubuntu 24.04 LTS (default) | Yes |
| `ubuntu:24.04` | Ubuntu 24.04 LTS | Yes |
| `ubuntu:22.04` | Ubuntu 22.04 LTS | Yes |
| `debian` | Debian 12 (bookworm) | Yes |
| `debian:12` | Debian 12 (bookworm) | Yes |
| `debian:11` | Debian 11 (bullseye) | Yes |
| `centos` | CentOS Stream 9 | Yes |
| `rocky` | Rocky Linux 9 | Yes |
| `almalinux` | AlmaLinux 9 | Yes |

**Note**: All VPS images include incus-agent for `dalang shell`, `dalang exec`, and `dalang console` access.

## VPS Pricing

Pay-as-you-go pricing based on resources:

| Resource | Price |
|----------|-------|
| vCPU | Rp 20.000/vCPU/month |
| RAM | Rp 5.000/GB/month |
| Storage (SSD) | Rp 1.000/GB/month |
| Bandwidth | 20 Mbps included FREE |
| | +Rp 20.000 per additional 20 Mbps |

**Formula**: `Price = (vCPU × 20K) + (RAM_GB × 5K) + (Storage_GB × 1K) + Extra bandwidth`

**Example Configurations**:
- Starter (1 vCPU, 1GB, 5GB, 20Mbps) → Rp 30.000/month
- Basic (1 vCPU, 1GB, 10GB, 20Mbps) → Rp 35.000/month
- Standard (2 vCPU, 2GB, 20GB, 40Mbps) → Rp 90.000/month
- Pro (4 vCPU, 4GB, 50GB, 100Mbps) → Rp 230.000/month

## Run Commands (exec)

`dalang exec` runs a single command inside the VM and returns its output, without
opening an interactive terminal. It runs inside a **persistent tmux session**, so
state (working directory, exported environment variables) survives between calls.

```bash
dalang exec MyVM "whoami"
dalang exec MyVM "cd /app && npm install"      # cwd persists to the next exec
dalang exec MyVM "export TOKEN=abc"            # env persists to the next exec
dalang exec MyVM "df -h /" --json              # machine-readable output

# Reset the persistent session if it gets into a bad state
dalang exec MyVM "tmux kill-session -t dalang_shell"
```

## File Transfer (scp)

`dalang scp` uses the same `<vps-name>:<absolute-path>` syntax as OpenSSH's `scp`.
Direction is inferred from which operand carries the host prefix; the **last positional
argument is always the destination**.

```bash
# Upload a single file
dalang scp ./app.tar.gz MyVM:/opt/app.tar.gz

# Download a single file
dalang scp MyVM:/etc/nginx/nginx.conf ./nginx.conf

# Multiple sources into a directory
dalang scp file1.txt file2.txt MyVM:/tmp/

# Recursive upload of a directory tree
dalang scp -r ./project MyVM:/srv/project

# Recursive download, preserving mode + mtime
dalang scp -r -p MyVM:/var/log ./vm-logs

# Quiet mode (no progress bars)
dalang scp -q ./big.tar.gz MyVM:/tmp/big.tar.gz
```

Authorization mirrors `dalang shell`/`exec` — owner, group-shared, or admin. `cp` is an
alias for `scp`. The VM must be running.

### Legacy single-file transfer
The older single-file commands still work but `scp` is preferred for new use:
```bash
dalang upload <vps-name> ./app.tar.gz /opt/app.tar.gz   # alias: push
dalang download <vps-name> /var/log/app.log ./app.log   # alias: pull
```
- Maximum upload size: 500 MB
- A progress bar is shown during transfer
- The VPS must be running

## Custom Domains
```bash
# Enable custom domain addon (paid feature)
dalang domain enable <vps-name>

# After payment, add domains
dalang domain add <vps-name> example.com
dalang domain verify example.com
dalang domain list <vps-name>
dalang domain remove example.com
```

## Top Up Credits
```bash
dalang credit add 50      # Top up 50K IDR
dalang credit add 100     # Top up 100K IDR
dalang credit add 500     # Top up 500K IDR
```

## Command Reference

| Command | Description |
|---------|-------------|
| `dalang auth` | Authenticate with Dalang |
| `dalang auth status` | Show current login state |
| `dalang auth logout` | Clear stored credentials |
| `dalang credit` | Show current balance |
| `dalang credit history` | Show transaction history |
| `dalang credit add <N>` | Top up N thousand IDR |
| `dalang price` | Show VPS pricing table |
| `dalang price --cpu X --ram XG` | Calculate price for a config |
| `dalang service list` | List all services |
| `dalang service info <name>` | Show service details |
| `dalang service create` | Create new VPS |
| `dalang service upgrade <name>` | Upgrade specs (paid from credits) |
| `dalang service extend <name> --months N` | Extend subscription |
| `dalang shell <name>` | Open interactive shell to VM |
| `dalang exec <name> "cmd"` | Run a command, return output |
| `dalang console <name>` | Open console to VM |
| `dalang start <name>` | Start a VM |
| `dalang stop <name>` | Stop a VM |
| `dalang delete <name>` | Delete a VM |
| `dalang domain enable <vps>` | Enable custom domains |
| `dalang domain list <vps>` | List custom domains |
| `dalang domain add <vps> <domain>` | Add custom domain |
| `dalang domain verify <domain>` | Verify DNS setup |
| `dalang domain remove <domain>` | Remove custom domain |
| `dalang scp <src> <dst>` | Copy files to/from a VM (scp-style) |
| `dalang upload <vps> <local> <remote>` | Upload a single file (legacy) |
| `dalang download <vps> <remote> [local]` | Download a single file (legacy) |
| `dalang update` | Update CLI to latest version |
| `dalang version` | Show CLI version (also `dalang -V`) |
| `dalang help <command>` | Show command help |

## Global Options

| Option | Description |
|--------|-------------|
| `--json` | Output in JSON format (useful for scripting) |
| `--quiet`, `-q` | Minimal output |
| `--yes`, `-y` | Skip confirmation prompts |
| `--no-color` | Disable colored output |
| `--verbose`, `-v` | Show debug output |
| `-V`, `--version` | Show version |

> Note: `-v` means **verbose**, not version — use `-V` or `dalang version` for the
> version. Colors are also disabled automatically when output is piped/redirected or
> when the `NO_COLOR` environment variable is set.

## Environment Variables

| Variable | Description |
|----------|-------------|
| `DALANG_API_URL` | Override API URL (default: https://api.dalang.io) |
| `NO_COLOR` | Disable colored output when set (any value) |

## Service Info Fields

When running `dalang service info <name>`, you get:

- **Auto-sync**: Automatically syncs specs and status from the cluster before displaying
- **Status**: RUNNING, STOPPED, CREATING, UNAVAILABLE
- **ID**: UUID of the VPS
- **Region**: ID-BANTEN-02
- **Node**: Physical server node (if assigned)
- **Specs**: CPU, RAM, Storage, Bandwidth
- **Network**: Public IP, Local IP
- **Domains**: Free public domain, Custom domain status
- **Subscription**: Price per month, Expiration date with days left

## Shell/Console Usage

When connected via `dalang shell` or `dalang console`:

- On connect, the shell prints a **resource panel** (uptime, CPU load, memory, disk)
- The shell runs in a **persistent tmux session** — if you disconnect (network drop,
  Ctrl+C, or `~.`), the session keeps running and reattaches on the next connect
- **Disconnect (keep session alive)**: press Enter, then type `~.` (tilde + period)
- **End the session entirely**: type `exit` (this kills the tmux session, not just the link)
- Ctrl+C is sent to the remote VM, not used to disconnect
- Terminal is in raw mode for full interactivity

## Installation & Updates

Linux / macOS / Termux (auto-detects the environment):
```bash
curl -fsSL https://dalang.io/install.sh | bash
```

Windows (PowerShell):
```powershell
irm https://dalang.io/install.ps1 | iex
```

Update in place:
```bash
dalang update            # downloads the latest release with a progress bar
sudo dalang update       # if installed system-wide; sudo not needed on Termux
```

Released binaries: `dalang-linux-amd64`, `dalang-linux-arm64`, `dalang-android-arm64`
(Termux), `dalang-darwin-amd64`, `dalang-darwin-arm64`, `dalang-windows-amd64.exe`.
On Windows, UTF-8 glyphs and ANSI colors render correctly in PowerShell/conhost.

## Error Handling

Common errors and solutions:

| Error | Solution |
|-------|----------|
| "not authenticated" | Run `dalang auth` first |
| "VPS not found" | Check name with `dalang service list` (names are fuzzy-matched) |
| "VPS not running" | Start with `dalang start <name>` |
| "insufficient credits" | Top up with `dalang credit add <amount>` |
| "custom domain not enabled" | Run `dalang domain enable <vps>` first |
| "new values must be higher than current" | `service upgrade` only scales up |

## AI Usage Tips

1. **Always check auth first**: If the user gets auth errors, suggest `dalang auth`
2. **Use --json for parsing**: When you need to parse output, add the `--json` flag
3. **Prefer `exec` over `shell` for automation**: `shell` is interactive; `exec` returns output
4. **Service names are case-insensitive**: the CLI suggests the closest match on a typo
5. **Credits are in IDR**: 50 = Rp 50.000 (50K IDR)
6. **Expiration warnings**: Alert the user if a service expires within 7 days

## Example AI Interactions

### User: "Connect to my server"
```bash
# First list available services
dalang service list

# Then connect (assuming the VM is running)
dalang shell <vm-name>
```

### User: "Run a command on my server"
```bash
dalang exec <vm-name> "uptime"
dalang exec <vm-name> "systemctl status nginx"
```

### User: "Check my balance and top up if low"
```bash
dalang credit
dalang credit add 100  # 100K IDR
```

### User: "Set up custom domain for my VPS"
```bash
dalang domain enable <vps-name>      # (user pays the invoice)
dalang domain add <vps-name> example.com
# user configures DNS records as shown
dalang domain verify example.com
```

### User: "Upload my app to the server"
```bash
# Copy the archive up, then extract it via exec
dalang scp ./app.tar.gz <vps-name>:/opt/app.tar.gz
dalang exec <vps-name> "cd /opt && tar xzf app.tar.gz"

# Or copy a whole project tree recursively
dalang scp -r ./project <vps-name>:/srv/project
```

### User: "Download logs from my VM"
```bash
dalang scp <vps-name>:/var/log/syslog ./syslog.txt
```

### User: "Give my VM more power"
```bash
dalang service upgrade <vm-name> --cpu 4 --ram 4G --storage 50G
```

### User: "Show me everything about my VM"
```bash
dalang service info <vm-name>
```

## Scripting Example

```bash
#!/bin/bash
# Check if a VM is running, start it if not, then run a command

STATUS=$(dalang service info MyVM --json | jq -r '.status')

if [ "$STATUS" != "RUNNING" ]; then
    dalang start MyVM --yes
    echo "Starting VM..."
    until [ "$(dalang service info MyVM --json | jq -r .status)" = "RUNNING" ]; do sleep 5; done
fi

dalang exec MyVM "uptime"
```
