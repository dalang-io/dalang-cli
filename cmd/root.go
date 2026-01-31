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
	fmt.Printf(`%sDalang CLI%s - Command-line interface for Dalang.io

%sUSAGE:%s
    dalang <command> [options]

%sCOMMANDS:%s
    %sversion%s              Show CLI version
    %sauth%s                 Authenticate with Dalang
    %scredit%s               Manage credits/wallet
    %sservice%s              Manage services (VPS, containers, apps)
    %sshell%s <name>         Open shell to VM
    %sconsole%s <name>       Open console to VM
    %sstart%s <name>         Start a VM
    %sstop%s <name>          Stop a VM
    %sdelete%s <name>        Delete a VM

%sGLOBAL OPTIONS:%s
    --json               Output in JSON format
    --quiet, -q          Minimal output
    --yes, -y            Skip confirmation prompts
    --verbose, -v        Show debug output

%sEXAMPLES:%s
    dalang auth                          # Authenticate
    dalang credit                        # Check balance
    dalang service list                  # List all services
    dalang shell MyVM                    # Connect to VM
    dalang service create --name MyVM --cpu 2 --ram 1G

Run '%sdalang <command> --help%s' for more information on a command.
`,
		colorBold, colorReset,
		colorYellow, colorReset,
		colorYellow, colorReset,
		colorCyan, colorReset,
		colorCyan, colorReset,
		colorCyan, colorReset,
		colorCyan, colorReset,
		colorCyan, colorReset,
		colorCyan, colorReset,
		colorCyan, colorReset,
		colorCyan, colorReset,
		colorCyan, colorReset,
		colorYellow, colorReset,
		colorYellow, colorReset,
		colorCyan, colorReset,
	)
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
	default:
		printError("Unknown command: %s", command)
		return fmt.Errorf("unknown command: %s", command)
	}
	return nil
}
