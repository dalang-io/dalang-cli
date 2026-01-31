package cmd

import (
	"fmt"
	"os"
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

// Execute runs the CLI
func Execute() error {
	if len(os.Args) < 2 {
		printHelp()
		return nil
	}

	// Parse global flags first
	args := parseGlobalFlags(os.Args[1:])

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
	fmt.Println(colorYellow + "AUTHENTICATION:" + colorReset)
	fmt.Println("    " + colorCyan + "auth" + colorReset + "                       Login to your Dalang account")
	fmt.Println("    " + colorCyan + "auth logout" + colorReset + "                Logout and clear credentials")
	fmt.Println()
	fmt.Println(colorYellow + "CREDITS & WALLET:" + colorReset)
	fmt.Println("    " + colorCyan + "credit" + colorReset + "                     Show current balance")
	fmt.Println("    " + colorCyan + "credit history" + colorReset + "             Show transaction history")
	fmt.Println("    " + colorCyan + "credit add <amount>" + colorReset + "        Top up credits (in thousands, e.g., 50 = 50K IDR)")
	fmt.Println()
	fmt.Println(colorYellow + "SERVICE MANAGEMENT:" + colorReset)
	fmt.Println("    " + colorCyan + "service list" + colorReset + "               List all your services (VPS, containers, apps)")
	fmt.Println("    " + colorCyan + "service info <name>" + colorReset + "        Show detailed info about a service")
	fmt.Println("    " + colorCyan + "service create" + colorReset + "             Create a new VPS (interactive)")
	fmt.Println()
	fmt.Println(colorYellow + "VM OPERATIONS:" + colorReset)
	fmt.Println("    " + colorCyan + "shell <name>" + colorReset + "               Open interactive shell to VM")
	fmt.Println("    " + colorCyan + "console <name>" + colorReset + "             Open console connection to VM")
	fmt.Println("    " + colorCyan + "start <name>" + colorReset + "               Start a stopped VM")
	fmt.Println("    " + colorCyan + "stop <name>" + colorReset + "                Stop a running VM")
	fmt.Println("    " + colorCyan + "delete <name>" + colorReset + "              Delete a VM (with confirmation)")
	fmt.Println()
	fmt.Println(colorYellow + "CUSTOM DOMAINS:" + colorReset)
	fmt.Println("    " + colorCyan + "domain enable <vps>" + colorReset + "        Enable custom domain addon for VPS")
	fmt.Println("    " + colorCyan + "domain list <vps>" + colorReset + "          List custom domains on a VPS")
	fmt.Println("    " + colorCyan + "domain add <vps> <domain>" + colorReset + "  Add a custom domain to VPS")
	fmt.Println("    " + colorCyan + "domain verify <domain>" + colorReset + "     Verify domain DNS configuration")
	fmt.Println("    " + colorCyan + "domain remove <domain>" + colorReset + "     Remove a custom domain")
	fmt.Println()
	fmt.Println(colorYellow + "OTHER:" + colorReset)
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
	fmt.Println(colorYellow + "QUICK START:" + colorReset)
	fmt.Println("    1. Login:           dalang auth")
	fmt.Println("    2. Check balance:   dalang credit")
	fmt.Println("    3. List services:   dalang service list")
	fmt.Println("    4. Connect to VM:   dalang shell <vm-name>")
	fmt.Println()
	fmt.Println(colorYellow + "EXAMPLES:" + colorReset)
	fmt.Println("    dalang auth                              # Login to Dalang")
	fmt.Println("    dalang credit add 100                    # Top up 100K IDR")
	fmt.Println("    dalang service list                      # List all services")
	fmt.Println("    dalang service info MyVM                 # Show VM details")
	fmt.Println("    dalang shell MyVM                        # SSH into VM")
	fmt.Println("    dalang domain add MyVM example.com       # Add custom domain")
	fmt.Println()
	fmt.Println(colorYellow + "TIPS:" + colorReset)
	fmt.Println("    - Use " + colorCyan + "~." + colorReset + " (tilde + dot) after Enter to disconnect from shell/console")
	fmt.Println("    - VM names are case-sensitive")
	fmt.Println("    - Credits expire 12 months after top-up")
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
	case "update":
		printUpdateHelp()
	default:
		printError("Unknown command: %s", command)
		return fmt.Errorf("unknown command: %s", command)
	}
	return nil
}
