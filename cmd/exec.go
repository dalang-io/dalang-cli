package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/dalang-io/dalang-cli/internal/api"
	"golang.org/x/term"
)

func cmdExec(args []string) error {
	if len(args) > 0 && (args[0] == "--help" || args[0] == "-h") {
		printExecHelp()
		return nil
	}
	if len(args) < 2 {
		printExecHelp()
		return fmt.Errorf("missing arguments")
	}

	vmName := args[0]
	command := args[1]

	// Handle additional args as part of the command
	if len(args) > 2 {
		command = strings.Join(args[1:], " ")
	}

	return executeCommand(vmName, command)
}

func executeCommand(name, command string) error {
	client, err := api.NewAuthenticatedClient()
	if err != nil {
		return err
	}
	client.Verbose = VerboseOutput

	// Find VPS by name
	foundVPS, err := findVPSByName(client, name)
	if err != nil {
		return err
	}

	if strings.ToUpper(foundVPS.Status) != "RUNNING" {
		return fmt.Errorf("VPS '%s' is not running (status: %s)", name, foundVPS.Status)
	}

	PrintDebug("Executing command on VPS %s: %s", foundVPS.ID, command)

	// Show a spinner while the (blocking) exec runs so it's clear the CLI is
	// working and not stuck. Only on an interactive stderr, and never in quiet
	// or JSON mode so piped/parsed output stays clean.
	var sp *spinner
	if !quietOutput && !jsonOutput && term.IsTerminal(int(os.Stderr.Fd())) {
		sp = startSpinner(fmt.Sprintf("Running on %s...", name))
	}

	// Execute command via API. Use a timeout above the server-side command
	// timeout so the CLI doesn't cut the request short for slow commands.
	execResp, err := client.PostWithTimeout("/vps/session/exec", map[string]string{
		"uuid":    foundVPS.ID,
		"command": command,
	}, 60*time.Second)

	if sp != nil {
		sp.Stop()
	}
	if err != nil {
		return fmt.Errorf("failed to execute command: %w", err)
	}

	var result struct {
		Success bool `json:"success"`
		Data    struct {
			Output   string `json:"output"`
			ExitCode int    `json:"exit_code"`
		} `json:"data"`
		Message string `json:"message"`
	}

	if err := json.Unmarshal(execResp, &result); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	if !result.Success {
		return fmt.Errorf("command failed: %s", result.Message)
	}

	// Output based on format
	if jsonOutput {
		// JSON output for automation
		jsonResult, _ := json.MarshalIndent(map[string]interface{}{
			"output":    result.Data.Output,
			"exit_code": result.Data.ExitCode,
		}, "", "  ")
		fmt.Println(string(jsonResult))
	} else {
		// Plain output
		if result.Data.Output != "" {
			fmt.Print(result.Data.Output)
			// Ensure trailing newline
			if !strings.HasSuffix(result.Data.Output, "\n") {
				fmt.Println()
			}
		}
	}

	// Return error if non-zero exit code
	if result.Data.ExitCode != 0 {
		return fmt.Errorf("command exited with code %d", result.Data.ExitCode)
	}

	return nil
}

func printExecHelp() {
	fmt.Printf(`%sdalang exec%s - Execute command in VPS session

%sUSAGE:%s
    dalang exec <vm-name> "command"

%sDESCRIPTION:%s
    Executes a command in the VPS's persistent tmux session and returns
    the output. The session state (environment variables, current directory)
    persists between exec calls and shell connections.

%sARGUMENTS:%s
    <vm-name>     Name of the VPS to execute the command on
    "command"     Command to execute (quoted if contains spaces)

%sOPTIONS:%s
    --json        Output in JSON format for automation

%sEXAMPLES:%s
    # Simple command
    dalang exec MyVM "whoami"

    # Set environment variable (persists in session)
    dalang exec MyVM "export PATH=/custom:\$PATH"

    # Change directory and run command
    dalang exec MyVM "cd /app && npm install"

    # JSON output for scripting
    dalang exec MyVM "ls -la" --json

    # Multiple commands in one call
    dalang exec MyVM "cd /var/log && tail -n 5 syslog"

%sSESSION PERSISTENCE:%s
    Commands execute in a persistent tmux session named 'dalang_shell'.
    This means:
    - Environment variables set with 'export' persist
    - Directory changes with 'cd' persist
    - Session survives disconnects from 'dalang shell'

    To reset the session, use: dalang exec MyVM "tmux kill-session -t dalang_shell"

%sNOTE:%s
    - The VPS must be in RUNNING state
    - Commands have a 30-second timeout
    - Use 'dalang shell' for interactive sessions
`,
		colorCyan, colorReset,
		colorYellow, colorReset,
		colorYellow, colorReset,
		colorYellow, colorReset,
		colorYellow, colorReset,
		colorYellow, colorReset,
		colorYellow, colorReset,
		colorYellow, colorReset,
	)
}
