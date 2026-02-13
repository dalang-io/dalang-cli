package cmd

import (
	"fmt"
	"os"
	"runtime"
	"strings"
)

// Version info - set from main.go
var (
	Version   string
	BuildDate string
	Commit    string
)

// ANSI color codes
const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
	colorCyan   = "\033[36m"
	colorBold   = "\033[1m"
)

// Global flags
var (
	jsonOutput    bool
	quietOutput   bool
	yesFlag       bool
	VerboseOutput bool
)

func printError(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, colorRed+"Error: "+colorReset+format+"\n", args...)
}

func printSuccess(format string, args ...interface{}) {
	if !quietOutput {
		fmt.Printf(colorGreen+"✓ "+colorReset+format+"\n", args...)
	}
}

func printInfo(format string, args ...interface{}) {
	if !quietOutput {
		fmt.Printf(colorBlue+"→ "+colorReset+format+"\n", args...)
	}
}

func printWarn(format string, args ...interface{}) {
	if !quietOutput {
		fmt.Printf(colorYellow+"! "+colorReset+format+"\n", args...)
	}
}

func PrintDebug(format string, args ...interface{}) {
	if VerboseOutput {
		fmt.Printf(colorCyan+"[DEBUG] "+colorReset+format+"\n", args...)
	}
}

func renderBar(pct float64, width int) string {
	if pct > 100 {
		pct = 100
	}
	filled := int(pct / 100 * float64(width))
	if filled > width {
		filled = width
	}
	color := colorGreen
	if pct >= 80 {
		color = colorRed
	} else if pct >= 60 {
		color = colorYellow
	}
	bar := strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
	return color + bar + colorReset
}

func formatBytes(b int64) string {
	const (
		KB = 1024
		MB = 1024 * KB
		GB = 1024 * MB
	)
	switch {
	case b >= GB:
		return fmt.Sprintf("%.1f GB", float64(b)/float64(GB))
	case b >= MB:
		return fmt.Sprintf("%.0f MB", float64(b)/float64(MB))
	case b >= KB:
		return fmt.Sprintf("%.0f KB", float64(b)/float64(KB))
	default:
		return fmt.Sprintf("%d B", b)
	}
}

// Execute runs the CLI
func Execute() error {
	args := os.Args[1:]

	// Android's linker adds executable path as extra argument - skip it
	if runtime.GOOS == "android" && len(args) > 0 {
		if strings.HasPrefix(args[0], "/") && strings.HasSuffix(args[0], "dalang") {
			args = args[1:]
		}
	}

	if len(args) < 1 {
		printHelp()
		return nil
	}

	// Parse global flags first
	args = parseGlobalFlags(args)

	if len(args) == 0 {
		printHelp()
		return nil
	}

	command := args[0]
	cmdArgs := args[1:]

	switch command {
	case "version", "-v", "--version":
		return cmdVersion()
	case "help", "-h", "--help":
		if len(cmdArgs) > 0 {
			return cmdHelpFor(cmdArgs[0])
		}
		printHelp()
		return nil
	case "auth":
		return cmdAuth(cmdArgs)
	case "credit", "credits":
		return cmdCredit(cmdArgs)
	case "service", "services":
		return cmdService(cmdArgs)
	case "shell":
		return cmdShell(cmdArgs)
	case "exec":
		return cmdExec(cmdArgs)
	case "console":
		return cmdConsole(cmdArgs)
	case "start":
		return cmdStart(cmdArgs)
	case "stop":
		return cmdStop(cmdArgs)
	case "delete":
		return cmdDelete(cmdArgs)
	case "domain", "domains":
		return cmdDomain(cmdArgs)
	case "price", "pricing":
		return cmdPrice(cmdArgs)
	case "update":
		return cmdUpdate(cmdArgs)
	default:
		printError("Unknown command: %s", command)
		fmt.Println("\nRun 'dalang help' for usage.")
		return fmt.Errorf("unknown command: %s", command)
	}
}

func parseGlobalFlags(args []string) []string {
	var remaining []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			jsonOutput = true
		case "--quiet", "-q":
			quietOutput = true
		case "--yes", "-y":
			yesFlag = true
		case "--verbose", "-v":
			VerboseOutput = true
		default:
			remaining = append(remaining, args[i])
		}
	}
	return remaining
}

func printHelp() {
	fmt.Println(colorBold + "Dalang CLI" + colorReset + " - Command-line interface for Dalang.io cloud services")
	fmt.Println()
	fmt.Println(colorYellow + "USAGE:" + colorReset)
	fmt.Println("    dalang <command> [subcommand] [options]")
	fmt.Println()
	fmt.Println(colorYellow + "COMMANDS:" + colorReset)
	fmt.Println()
	fmt.Printf("  %sAuthentication%s\n", colorBold, colorReset)
	fmt.Println("    " + colorCyan + "auth" + colorReset + "                       Login to your Dalang account")
	fmt.Println("    " + colorCyan + "auth status" + colorReset + "                Check authentication status")
	fmt.Println("    " + colorCyan + "auth logout" + colorReset + "                Logout and clear credentials")
	fmt.Println()
	fmt.Printf("  %sCredits & Wallet%s\n", colorBold, colorReset)
	fmt.Println("    " + colorCyan + "credit" + colorReset + "                     Show current balance")
	fmt.Println("    " + colorCyan + "credit history" + colorReset + "             Show transaction history")
	fmt.Println("    " + colorCyan + "credit add <amount>" + colorReset + "        Top up credits (in thousands IDR)")
	fmt.Println()
	fmt.Printf("  %sService Management%s\n", colorBold, colorReset)
	fmt.Println("    " + colorCyan + "service list" + colorReset + "               List all services (VPS, containers, apps)")
	fmt.Println("    " + colorCyan + "service info <name>" + colorReset + "        Show detailed info about a service")
	fmt.Println("    " + colorCyan + "service create" + colorReset + "             Create a new VPS")
	fmt.Println()
	fmt.Printf("  %sVM Operations%s\n", colorBold, colorReset)
	fmt.Println("    " + colorCyan + "shell <name>" + colorReset + "               Open persistent shell to VM (reconnects to session)")
	fmt.Println("    " + colorCyan + "exec <name> \"cmd\"" + colorReset + "          Execute command in VM session, return output")
	fmt.Println("    " + colorCyan + "console <name>" + colorReset + "             Open console connection to VM")
	fmt.Println("    " + colorCyan + "start <name>" + colorReset + "               Start a stopped VM")
	fmt.Println("    " + colorCyan + "stop <name>" + colorReset + "                Stop a running VM")
	fmt.Println("    " + colorCyan + "delete <name>" + colorReset + "              Delete a VM (with confirmation)")
	fmt.Println()
	fmt.Printf("  %sCustom Domains%s\n", colorBold, colorReset)
	fmt.Println("    " + colorCyan + "domain enable <vps>" + colorReset + "        Enable custom domain addon")
	fmt.Println("    " + colorCyan + "domain list <vps>" + colorReset + "          List custom domains on a VPS")
	fmt.Println("    " + colorCyan + "domain add <vps> <domain>" + colorReset + "  Add a custom domain to VPS")
	fmt.Println("    " + colorCyan + "domain verify <domain>" + colorReset + "     Verify domain DNS configuration")
	fmt.Println("    " + colorCyan + "domain remove <domain>" + colorReset + "     Remove a custom domain")
	fmt.Println()
	fmt.Printf("  %sPricing%s\n", colorBold, colorReset)
	fmt.Println("    " + colorCyan + "price" + colorReset + "                      Show VPS pricing table")
	fmt.Println("    " + colorCyan + "price --cpu N --ram NG" + colorReset + "    Calculate price for specific config")
	fmt.Println()
	fmt.Printf("  %sOther%s\n", colorBold, colorReset)
	fmt.Println("    " + colorCyan + "update" + colorReset + "                     Update CLI to latest version")
	fmt.Println("    " + colorCyan + "version" + colorReset + "                    Show CLI version")
	fmt.Println("    " + colorCyan + "help <command>" + colorReset + "             Show help for a specific command")
	fmt.Println()
	fmt.Println(colorYellow + "GLOBAL OPTIONS:" + colorReset)
	fmt.Println("    --json          Output in JSON format (for scripting)")
	fmt.Println("    --quiet, -q     Minimal output")
	fmt.Println("    --yes, -y       Skip confirmation prompts")
	fmt.Println("    --verbose, -v   Show debug output")
	fmt.Println()
	fmt.Println(colorYellow + "EXAMPLES:" + colorReset)
	fmt.Println()
	fmt.Println("  # Login to Dalang")
	fmt.Println("  " + colorCyan + "dalang auth" + colorReset)
	fmt.Println()
	fmt.Println("  # Check balance and top up 100K IDR")
	fmt.Println("  " + colorCyan + "dalang credit" + colorReset)
	fmt.Println("  " + colorCyan + "dalang credit add 100" + colorReset)
	fmt.Println()
	fmt.Println("  # Create a VPS with 2 vCPU, 2GB RAM, 20GB storage")
	fmt.Println("  " + colorCyan + "dalang service create --name MyVM --cpu 2 --ram 2G --storage 20G" + colorReset)
	fmt.Println()
	fmt.Println("  # List services and show details")
	fmt.Println("  " + colorCyan + "dalang service list" + colorReset)
	fmt.Println("  " + colorCyan + "dalang service info MyVM" + colorReset)
	fmt.Println()
	fmt.Println("  # Connect to VM shell")
	fmt.Println("  " + colorCyan + "dalang shell MyVM" + colorReset)
	fmt.Println()
	fmt.Println("  # Start/stop VM")
	fmt.Println("  " + colorCyan + "dalang start MyVM" + colorReset)
	fmt.Println("  " + colorCyan + "dalang stop MyVM" + colorReset)
	fmt.Println()
	fmt.Println("  # Add custom domain")
	fmt.Println("  " + colorCyan + "dalang domain enable MyVM" + colorReset)
	fmt.Println("  " + colorCyan + "dalang domain add MyVM example.com" + colorReset)
	fmt.Println("  " + colorCyan + "dalang domain verify example.com" + colorReset)
	fmt.Println()
	fmt.Println("  # Check VPS pricing")
	fmt.Println("  " + colorCyan + "dalang price" + colorReset)
	fmt.Println("  " + colorCyan + "dalang price --cpu 2 --ram 2G --storage 20G --bandwidth 40" + colorReset)
	fmt.Println()
	fmt.Println(colorYellow + "TIPS:" + colorReset)
	fmt.Println("    - Use " + colorCyan + "~." + colorReset + " (tilde + dot) after Enter to disconnect from shell/console")
	fmt.Println("    - VM names are case-sensitive")
	fmt.Println("    - Credits expire 12 months after top-up")
	fmt.Println("    - Run " + colorCyan + "dalang help <command>" + colorReset + " for detailed command help")
	fmt.Println()
	fmt.Println("More info: " + colorCyan + "https://dalang.io/docs/cli" + colorReset)
}

func cmdHelpFor(command string) error {
	switch command {
	case "auth":
		printAuthHelp()
	case "credit", "credits":
		printCreditHelp()
	case "service", "services":
		printServiceHelp()
	case "shell":
		printShellHelp()
	case "exec":
		printExecHelp()
	case "console":
		printConsoleHelp()
	case "start":
		printStartHelp()
	case "stop":
		printStopHelp()
	case "delete":
		printDeleteHelp()
	case "domain", "domains":
		printDomainHelp()
	case "price", "pricing":
		printPriceHelp()
	case "update":
		printUpdateHelp()
	default:
		printError("Unknown command: %s", command)
		return fmt.Errorf("unknown command: %s", command)
	}
	return nil
}
