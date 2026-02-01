# Dalang CLI - AI Skill Guide

This document helps AI assistants understand and use the Dalang CLI tool effectively.

## Overview

Dalang CLI is a command-line interface for managing cloud services on Dalang.io, including VPS instances, containers, and app deployments.

## Authentication

Before using any command, the user must be authenticated:

```bash
dalang auth              # Start authentication flow
dalang auth logout       # Logout and clear credentials
```

Authentication uses Device Authorization Grant flow - user gets a code to enter on dalang.io/auth/cli.

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

# Connect to VM
dalang shell <vps-name>     # Interactive shell (lxc exec)
dalang console <vps-name>   # Console connection
```

### Create New VPS
```bash
# Basic VM with Ubuntu 24.04 (default)
dalang service create --name MyVM --cpu 2 --ram 1G --storage 10G

# Specify OS version
dalang service create --name WebServer --cpu 1 --ram 1G --image ubuntu:24.04
dalang service create --name DevBox --cpu 2 --ram 2G --image ubuntu:22.04
dalang service create --name Legacy --cpu 1 --ram 512M --image debian:11

# Available images:
#   ubuntu, ubuntu:24.04, ubuntu:22.04, ubuntu:20.04
#   debian, debian:12, debian:11
#   centos, rocky, almalinux, fedora
```

### Custom Domains
```bash
# Enable custom domain addon (paid feature)
dalang domain enable <vps-name>

# After payment, add domains
dalang domain add <vps-name> example.com
dalang domain verify example.com
dalang domain list <vps-name>
dalang domain remove example.com
```

### Top Up Credits
```bash
dalang credit add 50      # Top up 50K IDR
dalang credit add 100     # Top up 100K IDR
dalang credit add 500     # Top up 500K IDR
```

## Command Reference

| Command | Description |
|---------|-------------|
| `dalang auth` | Authenticate with Dalang |
| `dalang auth logout` | Clear stored credentials |
| `dalang credit` | Show current balance |
| `dalang credit history` | Show transaction history |
| `dalang credit add <N>` | Top up N thousand IDR |
| `dalang service list` | List all services |
| `dalang service info <name>` | Show service details |
| `dalang service create` | Create new VPS |
| `dalang shell <name>` | Open shell to VM |
| `dalang console <name>` | Open console to VM |
| `dalang start <name>` | Start a VM |
| `dalang stop <name>` | Stop a VM |
| `dalang delete <name>` | Delete a VM |
| `dalang domain enable <vps>` | Enable custom domains |
| `dalang domain list <vps>` | List custom domains |
| `dalang domain add <vps> <domain>` | Add custom domain |
| `dalang domain verify <domain>` | Verify DNS setup |
| `dalang domain remove <domain>` | Remove custom domain |
| `dalang update` | Update CLI to latest version |
| `dalang version` | Show CLI version |
| `dalang help <command>` | Show command help |

## Global Options

| Option | Description |
|--------|-------------|
| `--json` | Output in JSON format (useful for scripting) |
| `--quiet, -q` | Minimal output |
| `--yes, -y` | Skip confirmation prompts |
| `--verbose, -v` | Show debug output |

## Environment Variables

| Variable | Description |
|----------|-------------|
| `DALANG_API_URL` | Override API URL (default: https://api.dalang.io) |

## Service Info Fields

When running `dalang service info <name>`, you get:

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

- Type commands normally as if using SSH
- **Disconnect**: Press Enter, then type `~.` (tilde followed by period)
- Ctrl+C sends to the remote VM, not to disconnect
- Terminal is in raw mode for full interactivity

## Error Handling

Common errors and solutions:

| Error | Solution |
|-------|----------|
| "not authenticated" | Run `dalang auth` first |
| "VPS not found" | Check name with `dalang service list` |
| "VPS not running" | Start with `dalang start <name>` |
| "insufficient credits" | Top up with `dalang credit add <amount>` |
| "custom domain not enabled" | Run `dalang domain enable <vps>` first |

## AI Usage Tips

1. **Always check auth first**: If user gets auth errors, suggest `dalang auth`
2. **Use --json for parsing**: When you need to parse output, add `--json` flag
3. **Service names are case-sensitive**: Use exact names from `dalang service list`
4. **Credits are in IDR**: 50 = Rp 50.000 (50K IDR)
5. **Expiration warnings**: Alert user if service expires within 7 days

## Example AI Interactions

### User: "Connect to my server"
```bash
# First list available services
dalang service list

# Then connect (assuming VM is running)
dalang shell <vm-name>
```

### User: "Check my balance and top up if low"
```bash
# Check balance
dalang credit

# If low, suggest top up
dalang credit add 100  # 100K IDR
```

### User: "Set up custom domain for my VPS"
```bash
# 1. Enable custom domain addon
dalang domain enable <vps-name>
# (User pays invoice)

# 2. Add domain
dalang domain add <vps-name> example.com

# 3. User configures DNS records as shown

# 4. Verify
dalang domain verify example.com
```

### User: "Show me everything about my VM"
```bash
dalang service info <vm-name>
```

## Scripting Example

```bash
#!/bin/bash
# Check if VM is running, start if not

STATUS=$(dalang service info MyVM --json | jq -r '.status')

if [ "$STATUS" != "RUNNING" ]; then
    dalang start MyVM --yes
    echo "Starting VM..."
    sleep 10
fi

dalang shell MyVM
```
