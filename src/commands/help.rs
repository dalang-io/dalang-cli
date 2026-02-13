use crate::output::*;

pub fn print_help() {
    println!(
        "{}Dalang CLI{} - Command-line interface for Dalang.io cloud services",
        BOLD, RESET
    );
    println!();
    println!("{}USAGE:{}", YELLOW, RESET);
    println!("    dalang <command> [subcommand] [options]");
    println!();
    println!("{}COMMANDS:{}", YELLOW, RESET);
    println!();
    println!("  {}Authentication{}", BOLD, RESET);
    println!(
        "    {}auth{}                       Login to your Dalang account",
        CYAN, RESET
    );
    println!(
        "    {}auth status{}                Check authentication status",
        CYAN, RESET
    );
    println!(
        "    {}auth logout{}                Logout and clear credentials",
        CYAN, RESET
    );
    println!();
    println!("  {}Credits & Wallet{}", BOLD, RESET);
    println!(
        "    {}credit{}                     Show current balance",
        CYAN, RESET
    );
    println!(
        "    {}credit history{}             Show transaction history",
        CYAN, RESET
    );
    println!(
        "    {}credit add <amount>{}        Top up credits (in thousands IDR)",
        CYAN, RESET
    );
    println!();
    println!("  {}Service Management{}", BOLD, RESET);
    println!(
        "    {}service list{}               List all services (VPS, containers, apps)",
        CYAN, RESET
    );
    println!(
        "    {}service info <name>{}        Show detailed info about a service",
        CYAN, RESET
    );
    println!(
        "    {}service create{}             Create a new VPS",
        CYAN, RESET
    );
    println!();
    println!("  {}VM Operations{}", BOLD, RESET);
    println!(
        "    {}shell <name>{}               Open persistent shell to VM (reconnects to session)",
        CYAN, RESET
    );
    println!(
        "    {}exec <name> \"cmd\"{}          Execute command in VM session, return output",
        CYAN, RESET
    );
    println!(
        "    {}console <name>{}             Open console connection to VM",
        CYAN, RESET
    );
    println!(
        "    {}start <name>{}               Start a stopped VM",
        CYAN, RESET
    );
    println!(
        "    {}stop <name>{}                Stop a running VM",
        CYAN, RESET
    );
    println!(
        "    {}delete <name>{}              Delete a VM (with confirmation)",
        CYAN, RESET
    );
    println!();
    println!("  {}Custom Domains{}", BOLD, RESET);
    println!(
        "    {}domain enable <vps>{}        Enable custom domain addon",
        CYAN, RESET
    );
    println!(
        "    {}domain list <vps>{}          List custom domains on a VPS",
        CYAN, RESET
    );
    println!(
        "    {}domain add <vps> <domain>{}  Add a custom domain to VPS",
        CYAN, RESET
    );
    println!(
        "    {}domain verify <domain>{}     Verify domain DNS configuration",
        CYAN, RESET
    );
    println!(
        "    {}domain remove <domain>{}     Remove a custom domain",
        CYAN, RESET
    );
    println!();
    println!("  {}Pricing{}", BOLD, RESET);
    println!(
        "    {}price{}                      Show VPS pricing table",
        CYAN, RESET
    );
    println!(
        "    {}price --cpu N --ram NG{}    Calculate price for specific config",
        CYAN, RESET
    );
    println!();
    println!("  {}Other{}", BOLD, RESET);
    println!(
        "    {}update{}                     Update CLI to latest version",
        CYAN, RESET
    );
    println!(
        "    {}version{}                    Show CLI version",
        CYAN, RESET
    );
    println!(
        "    {}help <command>{}             Show help for a specific command",
        CYAN, RESET
    );
    println!();
    println!("{}GLOBAL OPTIONS:{}", YELLOW, RESET);
    println!("    --json          Output in JSON format (for scripting)");
    println!("    --quiet, -q     Minimal output");
    println!("    --yes, -y       Skip confirmation prompts");
    println!("    --verbose, -v   Show debug output");
    println!();
    println!("{}EXAMPLES:{}", YELLOW, RESET);
    println!();
    println!("  # Login to Dalang");
    println!("  {}dalang auth{}", CYAN, RESET);
    println!();
    println!("  # Check balance and top up 100K IDR");
    println!("  {}dalang credit{}", CYAN, RESET);
    println!("  {}dalang credit add 100{}", CYAN, RESET);
    println!();
    println!("  # Create a VPS with 2 vCPU, 2GB RAM, 20GB storage");
    println!(
        "  {}dalang service create --name MyVM --cpu 2 --ram 2G --storage 20G{}",
        CYAN, RESET
    );
    println!();
    println!("  # List services and show details");
    println!("  {}dalang service list{}", CYAN, RESET);
    println!("  {}dalang service info MyVM{}", CYAN, RESET);
    println!();
    println!("  # Connect to VM shell");
    println!("  {}dalang shell MyVM{}", CYAN, RESET);
    println!();
    println!("  # Start/stop VM");
    println!("  {}dalang start MyVM{}", CYAN, RESET);
    println!("  {}dalang stop MyVM{}", CYAN, RESET);
    println!();
    println!("  # Add custom domain");
    println!("  {}dalang domain enable MyVM{}", CYAN, RESET);
    println!("  {}dalang domain add MyVM example.com{}", CYAN, RESET);
    println!("  {}dalang domain verify example.com{}", CYAN, RESET);
    println!();
    println!("  # Check VPS pricing");
    println!("  {}dalang price{}", CYAN, RESET);
    println!(
        "  {}dalang price --cpu 2 --ram 2G --storage 20G --bandwidth 40{}",
        CYAN, RESET
    );
    println!();
    println!("{}TIPS:{}", YELLOW, RESET);
    println!(
        "    - Use {}~.{} (tilde + dot) after Enter to disconnect from shell/console",
        CYAN, RESET
    );
    println!("    - VM names are case-sensitive");
    println!("    - Credits expire 12 months after top-up");
    println!(
        "    - Run {}dalang help <command>{} for detailed command help",
        CYAN, RESET
    );
    println!();
    println!("More info: {}https://dalang.io/docs/cli{}", CYAN, RESET);
}

pub fn cmd_help_for(command: &str) -> Result<(), String> {
    match command {
        "auth" => print_auth_help(),
        "credit" | "credits" => print_credit_help(),
        "service" | "services" => print_service_help(),
        "shell" => print_shell_help(),
        "exec" => print_exec_help(),
        "console" => print_console_help(),
        "start" => print_start_help(),
        "stop" => print_stop_help(),
        "delete" => print_delete_help(),
        "domain" | "domains" => print_domain_help(),
        "price" | "pricing" => print_price_help(),
        "update" => print_update_help(),
        _ => {
            print_error(&format!("Unknown command: {}", command));
            return Err(format!("unknown command: {}", command));
        }
    }
    Ok(())
}

pub fn print_auth_help() {
    println!(
        "{}dalang auth{} - Authenticate with Dalang\n\
\n\
{}USAGE:{}\n\
    dalang auth              Start authentication flow\n\
    dalang auth status       Check current auth status\n\
    dalang auth logout       Clear stored credentials\n\
\n\
{}DESCRIPTION:{}\n\
    Authenticates the CLI with your Dalang account using Device\n\
    Authorization Grant flow. You'll be given a URL and code to\n\
    enter in your browser.\n\
\n\
    Credentials are stored in ~/.dalang/credentials\n\
\n\
{}EXAMPLES:{}\n\
    dalang auth              # Start authentication\n\
    dalang auth status       # Check if logged in\n\
    dalang auth logout       # Log out",
        CYAN, RESET, YELLOW, RESET, YELLOW, RESET, YELLOW, RESET,
    );
}

pub fn print_credit_help() {
    println!(
        "{}dalang credit{} - Manage credits/wallet\n\
\n\
{}USAGE:{}\n\
    dalang credit            Show current balance\n\
    dalang credit history    Show recent transactions\n\
    dalang credit add <N>    Top up N thousand IDR (e.g., 50 = 50K)\n\
\n\
{}DESCRIPTION:{}\n\
    Manage your Dalang credits balance. Credits can be used to pay\n\
    for VPS, containers, and app hosting services.\n\
\n\
{}EXAMPLES:{}\n\
    dalang credit            # Check balance\n\
    dalang credit history    # View last 25 transactions\n\
    dalang credit add 100    # Top up 100K IDR\n\
    dalang credit add 500    # Top up 500K IDR\n\
\n\
{}NOTE:{}\n\
    Minimum topup is 50K IDR. Credits expire 12 months after topup.",
        CYAN, RESET, YELLOW, RESET, YELLOW, RESET, YELLOW, RESET, YELLOW, RESET,
    );
}

pub fn print_service_help() {
    println!(
        "{}dalang service{} - Manage services\n\
\n\
{}USAGE:{}\n\
    dalang service list                    List all services\n\
    dalang service info <name>             Show service details\n\
    dalang service create [options]        Create new VPS\n\
    dalang service upgrade <name> [opts]   Upgrade VPS specs\n\
    dalang service extend <name> [opts]    Extend subscription\n\
\n\
{}DESCRIPTION:{}\n\
    Manage your VPS instances, containers, and app deployments.\n\
\n\
{}SUBCOMMANDS:{}\n\
    list      List all your services (VPS, containers, apps)\n\
    info      Show detailed information about a service\n\
    create    Create a new VPS instance\n\
    upgrade   Upgrade VPS CPU/RAM/storage/bandwidth\n\
    extend    Extend VPS subscription period\n\
\n\
{}EXAMPLES:{}\n\
    # List all services\n\
    dalang service list\n\
\n\
    # Show details of a specific VM\n\
    dalang service info MyVM\n\
\n\
    # Create a VPS with custom specs\n\
    dalang service create --name WebServer --cpu 2 --ram 2G --storage 20G\n\
\n\
    # Upgrade VPS specs\n\
    dalang service upgrade MyVM --cpu 4 --ram 4G\n\
\n\
    # Extend subscription by 3 months\n\
    dalang service extend MyVM --months 3\n\
\n\
Run '{}dalang service <command> --help{}' for command-specific options.",
        CYAN, RESET, YELLOW, RESET, YELLOW, RESET, YELLOW, RESET, YELLOW, RESET, CYAN, RESET,
    );
}

pub fn print_service_create_help() {
    println!(
        "{}dalang service create{} - Create new VPS\n\
\n\
{}USAGE:{}\n\
    dalang service create [options]\n\
\n\
{}OPTIONS:{}\n\
    --name, -n <name>        Service name (required)\n\
    --cpu, -c <count>        Number of vCPUs (default: 1)\n\
    --ram, -r <size>         RAM size, e.g., 512M, 1G (default: 512M)\n\
    --storage, -s <size>     Storage size, e.g., 5G, 10G (default: 5G)\n\
    --bandwidth, -b <mbps>   Bandwidth in Mbps (default: 20)\n\
    --image, -i <name>       OS image (default: ubuntu)\n\
    --region <region>        Region (default: ID-BANTEN-02)\n\
\n\
{}PRICING:{}\n\
    vCPU:       Rp 20.000/vCPU/month\n\
    RAM:        Rp 5.000/GB/month\n\
    Storage:    Rp 1.000/GB/month\n\
    Bandwidth:  20 Mbps included FREE, +Rp 20.000 per additional 20 Mbps\n\
\n\
    Run '{}dalang price{}' for detailed pricing info.\n\
\n\
{}IMAGES:{}\n\
    ubuntu           Ubuntu 24.04 (default)\n\
    ubuntu:24.04     Ubuntu 24.04 LTS\n\
    ubuntu:22.04     Ubuntu 22.04 LTS\n\
    ubuntu:20.04     Ubuntu 20.04 LTS\n\
    debian           Debian 12\n\
    debian:12        Debian 12\n\
    debian:11        Debian 11\n\
    centos           CentOS Stream 9\n\
    rocky            Rocky Linux 9\n\
    almalinux        AlmaLinux 9\n\
    fedora           Fedora 40\n\
\n\
{}REGIONS:{}\n\
    ID-BANTEN-02 (default)\n\
\n\
{}EXAMPLES:{}\n\
    dalang service create --name MyVM --cpu 2 --ram 1G --storage 10G\n\
    dalang service create --name WebServer --cpu 1 --ram 1G --image ubuntu:24.04\n\
    dalang service create -n DevBox -c 2 -r 2G -s 20G -i debian:12",
        CYAN, RESET, YELLOW, RESET, YELLOW, RESET, YELLOW, RESET, CYAN, RESET, YELLOW, RESET,
        YELLOW, RESET, YELLOW, RESET,
    );
}

pub fn print_upgrade_help() {
    println!(
        "{}dalang service upgrade{} - Upgrade VPS specs\n\
\n\
{}USAGE:{}\n\
    dalang service upgrade <name> [options]\n\
\n\
{}OPTIONS:{}\n\
    --cpu, -c <count>        New vCPU count (must be >= current)\n\
    --ram, -r <size>         New RAM size, e.g., 4G (must be >= current)\n\
    --storage, -s <size>     New storage size, e.g., 50G (must be >= current)\n\
    --bandwidth, -b <mbps>   New bandwidth in Mbps (must be >= current)\n\
\n\
{}NOTE:{}\n\
    - Upgrades are permanent, downgrade is not allowed\n\
    - Price difference is prorated for remaining subscription days\n\
    - Minimum upgrade cost is Rp 10.000\n\
\n\
{}EXAMPLES:{}\n\
    dalang service upgrade MyVM --cpu 4\n\
    dalang service upgrade MyVM --ram 4G --storage 50G\n\
    dalang service upgrade MyVM -c 4 -r 8G -s 100G -b 100",
        CYAN, RESET, YELLOW, RESET, YELLOW, RESET, YELLOW, RESET, YELLOW, RESET,
    );
}

pub fn print_extend_help() {
    println!(
        "{}dalang service extend{} - Extend VPS subscription\n\
\n\
{}USAGE:{}\n\
    dalang service extend <name> [options]\n\
\n\
{}OPTIONS:{}\n\
    --months, -m <count>     Extension period: 1, 3, 6, or 12 months (default: 1)\n\
\n\
{}BILLING PERIODS:{}\n\
    1 month   - Standard monthly billing\n\
    3 months  - Quarterly billing\n\
    6 months  - Semi-annual billing\n\
    12 months - Annual billing\n\
\n\
{}EXAMPLES:{}\n\
    dalang service extend MyVM              # Extend by 1 month\n\
    dalang service extend MyVM --months 3   # Extend by 3 months\n\
    dalang service extend MyVM -m 12        # Extend by 12 months",
        CYAN, RESET, YELLOW, RESET, YELLOW, RESET, YELLOW, RESET, YELLOW, RESET,
    );
}

pub fn print_shell_help() {
    println!(
        "{}dalang shell{} - Open persistent shell to VM\n\
\n\
{}USAGE:{}\n\
    dalang shell <vm-name>\n\
\n\
{}DESCRIPTION:{}\n\
    Opens an interactive shell session to the specified VM using a\n\
    persistent tmux session. If you disconnect (network drop, Ctrl+C),\n\
    the session keeps running and you can reconnect later.\n\
\n\
    This is similar to mosh - your session state (running processes,\n\
    environment variables, current directory) persists across disconnects.\n\
\n\
{}EXAMPLES:{}\n\
    dalang shell MyVM              # Connect to VM named MyVM\n\
    dalang shell WebServer         # Connect to VM named WebServer\n\
\n\
{}PERSISTENCE:{}\n\
    # Session persists across disconnects\n\
    dalang shell myvm\n\
    export FOO=bar\n\
    # Disconnect (Ctrl+C or network drop)\n\
\n\
    dalang shell myvm              # Reconnects to same session\n\
    echo $FOO                      # Outputs \"bar\" - state preserved!\n\
\n\
{}DISCONNECT:{}\n\
    Press Enter, then type ~. (tilde followed by period) to disconnect.\n\
    Or press Ctrl+C.\n\
    Note: 'exit' will end the tmux session (not just disconnect).\n\
\n\
{}RELATED:{}\n\
    dalang exec <name> \"cmd\"       # Execute command without interactive shell\n\
\n\
{}NOTE:{}\n\
    - The VM must be in RUNNING state to connect\n\
    - Use 'dalang start <name>' if the VM is stopped\n\
    - VM names are case-sensitive\n\
    - tmux is auto-installed if missing on first connect",
        CYAN, RESET, YELLOW, RESET, YELLOW, RESET, YELLOW, RESET, YELLOW, RESET, YELLOW, RESET,
        YELLOW, RESET, YELLOW, RESET,
    );
}

pub fn print_console_help() {
    println!(
        "{}dalang console{} - Open console to VM\n\
\n\
{}USAGE:{}\n\
    dalang console <vm-name>\n\
\n\
{}DESCRIPTION:{}\n\
    Opens a console session to the specified VM.\n\
    Similar to shell but connects to the VM's virtual console directly.\n\
    Useful when shell access is not available.\n\
\n\
{}EXAMPLES:{}\n\
    dalang console MyVM            # Connect to VM named MyVM\n\
    dalang console WebServer       # Connect to VM named WebServer\n\
\n\
{}DISCONNECT:{}\n\
    Press Enter, then type ~. (tilde followed by period) to disconnect.\n\
\n\
{}NOTE:{}\n\
    - The VM must be in RUNNING state to connect\n\
    - Use 'dalang start <name>' if the VM is stopped\n\
    - VM names are case-sensitive",
        CYAN, RESET, YELLOW, RESET, YELLOW, RESET, YELLOW, RESET, YELLOW, RESET, YELLOW, RESET,
    );
}

pub fn print_exec_help() {
    println!(
        "{}dalang exec{} - Execute command in VPS session\n\
\n\
{}USAGE:{}\n\
    dalang exec <vm-name> \"command\"\n\
\n\
{}DESCRIPTION:{}\n\
    Executes a command in the VPS's persistent tmux session and returns\n\
    the output. The session state (environment variables, current directory)\n\
    persists between exec calls and shell connections.\n\
\n\
{}ARGUMENTS:{}\n\
    <vm-name>     Name of the VPS to execute the command on\n\
    \"command\"     Command to execute (quoted if contains spaces)\n\
\n\
{}OPTIONS:{}\n\
    --json        Output in JSON format for automation\n\
\n\
{}EXAMPLES:{}\n\
    # Simple command\n\
    dalang exec MyVM \"whoami\"\n\
\n\
    # Set environment variable (persists in session)\n\
    dalang exec MyVM \"export PATH=/custom:$PATH\"\n\
\n\
    # Change directory and run command\n\
    dalang exec MyVM \"cd /app && npm install\"\n\
\n\
    # JSON output for scripting\n\
    dalang exec MyVM \"ls -la\" --json\n\
\n\
    # Multiple commands in one call\n\
    dalang exec MyVM \"cd /var/log && tail -n 5 syslog\"\n\
\n\
{}SESSION PERSISTENCE:{}\n\
    Commands execute in a persistent tmux session named 'dalang_shell'.\n\
    This means:\n\
    - Environment variables set with 'export' persist\n\
    - Directory changes with 'cd' persist\n\
    - Session survives disconnects from 'dalang shell'\n\
\n\
    To reset the session, use: dalang exec MyVM \"tmux kill-session -t dalang_shell\"\n\
\n\
{}NOTE:{}\n\
    - The VPS must be in RUNNING state\n\
    - Commands have a 30-second timeout\n\
    - Use 'dalang shell' for interactive sessions",
        CYAN, RESET, YELLOW, RESET, YELLOW, RESET, YELLOW, RESET, YELLOW, RESET, YELLOW, RESET,
        YELLOW, RESET, YELLOW, RESET,
    );
}

pub fn print_start_help() {
    println!(
        "{}dalang start{} - Start a VM\n\
\n\
{}USAGE:{}\n\
    dalang start <vm-name>\n\
\n\
{}DESCRIPTION:{}\n\
    Starts a stopped VM instance. The VM will boot and become available\n\
    for shell/console access.\n\
\n\
{}EXAMPLES:{}\n\
    dalang start MyVM              # Start VM named MyVM\n\
    dalang start WebServer         # Start VM named WebServer\n\
\n\
{}NOTE:{}\n\
    - VM names are case-sensitive\n\
    - Use 'dalang service list' to see available VMs\n\
    - After starting, use 'dalang shell <name>' to connect",
        CYAN, RESET, YELLOW, RESET, YELLOW, RESET, YELLOW, RESET, YELLOW, RESET,
    );
}

pub fn print_stop_help() {
    println!(
        "{}dalang stop{} - Stop a VM\n\
\n\
{}USAGE:{}\n\
    dalang stop <vm-name>\n\
\n\
{}DESCRIPTION:{}\n\
    Stops a running VM instance. The VM will shut down gracefully.\n\
    All data is preserved and you can restart it later.\n\
\n\
{}EXAMPLES:{}\n\
    dalang stop MyVM               # Stop VM named MyVM\n\
    dalang stop WebServer          # Stop VM named WebServer\n\
\n\
{}NOTE:{}\n\
    - VM names are case-sensitive\n\
    - Use 'dalang service list' to see running VMs\n\
    - Stopped VMs still incur charges (use 'dalang delete' to remove)",
        CYAN, RESET, YELLOW, RESET, YELLOW, RESET, YELLOW, RESET, YELLOW, RESET,
    );
}

pub fn print_delete_help() {
    println!(
        "{}dalang delete{} - Delete a VM\n\
\n\
{}USAGE:{}\n\
    dalang delete <vm-name>\n\
    dalang delete <vm-name> --yes\n\
\n\
{}DESCRIPTION:{}\n\
    Permanently deletes a VM and all its data. This action cannot be undone.\n\
    Requires confirmation unless --yes flag is provided.\n\
\n\
{}OPTIONS:{}\n\
    --yes, -y    Skip confirmation prompt\n\
\n\
{}EXAMPLES:{}\n\
    dalang delete MyVM             # Will prompt for confirmation\n\
    dalang delete MyVM --yes       # Skip confirmation\n\
    dalang delete WebServer -y     # Short form, skip confirmation\n\
\n\
{}WARNING:{}\n\
    This will permanently delete:\n\
    - All data stored on the VM\n\
    - All custom domain configurations\n\
    - Any associated backups",
        CYAN, RESET, YELLOW, RESET, YELLOW, RESET, YELLOW, RESET, YELLOW, RESET, YELLOW, RESET,
    );
}

pub fn print_domain_help() {
    println!(
        "{}dalang domain{} - Manage custom domains\n\
\n\
{}USAGE:{}\n\
    dalang domain enable <vps-name>           Enable custom domain (+10K/month)\n\
    dalang domain list <vps-name>             List custom domains\n\
    dalang domain add <vps-name> <domain>     Add a custom domain\n\
    dalang domain verify <domain>             Verify domain DNS configuration\n\
    dalang domain remove <domain>             Remove a custom domain\n\
\n\
{}DESCRIPTION:{}\n\
    Manage custom domains for your VPS. First enable the custom domain\n\
    addon, then add and verify your domains.\n\
\n\
{}EXAMPLES:{}\n\
    dalang domain enable MyVM                 # Enable custom domain feature\n\
    dalang domain add MyVM example.com        # Add a domain\n\
    dalang domain verify example.com          # Verify DNS is configured\n\
    dalang domain list MyVM                   # List all domains\n\
    dalang domain remove example.com          # Remove a domain\n\
\n\
{}WORKFLOW:{}\n\
    1. Enable custom domain: dalang domain enable <vps>\n\
    2. Pay the invoice\n\
    3. Add your domain: dalang domain add <vps> <domain>\n\
    4. Configure DNS records as shown\n\
    5. Verify: dalang domain verify <domain>",
        CYAN, RESET, YELLOW, RESET, YELLOW, RESET, YELLOW, RESET, YELLOW, RESET,
    );
}

pub fn print_price_help() {
    println!(
        "{}dalang price{} - Calculate VPS pricing\n\
\n\
{}USAGE:{}\n\
    dalang price                    Show pricing table\n\
    dalang price [options]          Calculate price for specific config\n\
\n\
{}OPTIONS:{}\n\
    --cpu, -c <count>        Number of vCPUs (default: 1)\n\
    --ram, -r <size>         RAM size (e.g., 512M, 1G, 2G) (default: 512M)\n\
    --storage, -s <size>     Storage size (e.g., 10G, 20G) (default: 5G)\n\
    --bandwidth, -b <mbps>   Bandwidth in Mbps (default: 20)\n\
\n\
{}PRICING:{}\n\
    vCPU:       Rp 20.000/vCPU/month\n\
    RAM:        Rp 5.000/GB/month\n\
    Storage:    Rp 1.000/GB/month\n\
    Bandwidth:  20 Mbps included FREE\n\
                +Rp 20.000 per additional 20 Mbps\n\
\n\
{}EXAMPLES:{}\n\
    # Show pricing table\n\
    dalang price\n\
\n\
    # Calculate price for 2 vCPU, 2GB RAM, 20GB storage\n\
    dalang price --cpu 2 --ram 2G --storage 20G\n\
\n\
    # Calculate price with extra bandwidth (40 Mbps)\n\
    dalang price --cpu 2 --ram 2G --storage 20G --bandwidth 40\n\
\n\
    # Short form\n\
    dalang price -c 4 -r 4G -s 50G -b 100\n\
\n\
{}PRICE FORMULA:{}\n\
    Price = (vCPU x 20K) + (RAM_GB x 5K) + (Storage_GB x 1K)\n\
          + ((Bandwidth - 20) / 20) x 20K\n\
\n\
{}EXAMPLE CONFIGS:{}\n\
    Starter:  1 vCPU, 1GB RAM, 5GB    = Rp 30.000/month\n\
    Basic:    1 vCPU, 1GB RAM, 10GB   = Rp 35.000/month\n\
    Standard: 2 vCPU, 2GB RAM, 20GB   = Rp 70.000/month\n\
    Pro:      4 vCPU, 4GB RAM, 50GB   = Rp 150.000/month",
        CYAN, RESET, YELLOW, RESET, YELLOW, RESET, YELLOW, RESET, YELLOW, RESET, YELLOW, RESET,
        YELLOW, RESET,
    );
}

pub fn print_update_help() {
    println!(
        "{}dalang update{} - Update CLI to latest version\n\
\n\
{}USAGE:{}\n\
    dalang update\n\
\n\
{}DESCRIPTION:{}\n\
    Downloads and installs the latest version of Dalang CLI.\n\
    On Linux/macOS, may require sudo if installed in /usr/local/bin.\n\
\n\
{}EXAMPLES:{}\n\
    dalang update              # Update to latest version\n\
    sudo dalang update         # Update with elevated permissions\n\
\n\
{}NOTE:{}\n\
    Current binary will be replaced in-place.\n\
    Your credentials and config are preserved.",
        CYAN, RESET, YELLOW, RESET, YELLOW, RESET, YELLOW, RESET, YELLOW, RESET,
    );
}
