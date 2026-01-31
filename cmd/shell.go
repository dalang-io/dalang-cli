package cmd

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/signal"
	"runtime"
	"strings"

	"github.com/dalang-io/dalang-cli/internal/api"
	"github.com/dalang-io/dalang-cli/internal/config"
	"github.com/dalang-io/dalang-cli/internal/terminal"
)

func cmdShell(args []string) error {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		printShellHelp()
		if len(args) == 0 {
			return fmt.Errorf("missing service name")
		}
		return nil
	}

	return connectTerminal(args[0], "shell")
}

func cmdConsole(args []string) error {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		printConsoleHelp()
		if len(args) == 0 {
			return fmt.Errorf("missing service name")
		}
		return nil
	}

	return connectTerminal(args[0], "console")
}

func connectTerminal(name, mode string) error {
	client, err := api.NewAuthenticatedClient()
	if err != nil {
		return err
	}

	// Find VPS by name
	vpsResp, err := client.Get("/vps/list")
	if err != nil {
		return fmt.Errorf("failed to fetch services: %w", err)
	}

	var vpsData api.VPSListResponse
	if err := json.Unmarshal(vpsResp, &vpsData); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	var foundVPS *api.VPS
	for _, v := range vpsData.Data.VPS {
		if v.Name == name || v.DisplayName == name {
			foundVPS = &v
			break
		}
	}

	if foundVPS == nil {
		printError("VPS '%s' not found", name)
		return fmt.Errorf("VPS not found: %s", name)
	}

	if strings.ToUpper(foundVPS.Status) != "RUNNING" {
		printError("VPS '%s' is not running (status: %s)", name, foundVPS.Status)
		return fmt.Errorf("VPS not running")
	}

	// Load credentials for token
	creds, err := config.LoadCredentials()
	if err != nil {
		return fmt.Errorf("failed to load credentials: %w", err)
	}

	// Build WebSocket URL
	apiURL := config.GetAPIURL()
	wsURL := strings.Replace(apiURL, "https://", "wss://", 1)
	wsURL = strings.Replace(wsURL, "http://", "ws://", 1)

	wsURL = fmt.Sprintf("%s/vps/terminal?uuid=%s&token=%s&mode=%s&force=true",
		wsURL,
		foundVPS.ID,
		url.QueryEscape(creds.AccessToken),
		mode,
	)

	displayName := foundVPS.DisplayName
	if displayName == "" {
		displayName = foundVPS.Name
	}

	printInfo("Connecting to %s (%s mode)...", displayName, mode)

	// Connect to terminal
	term, err := terminal.NewTerminal(wsURL)
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}
	defer term.Close()

	printSuccess("Connected! Press Ctrl+C to disconnect.\n")

	// Set up signal handling (cross-platform)
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt)

	// Handle resize on Unix systems
	if runtime.GOOS != "windows" {
		setupResizeHandler(sigChan, term)
	}

	// Handle interrupt
	go func() {
		<-sigChan
		term.Close()
	}()

	// Run terminal
	if err := term.Run(); err != nil {
		if err.Error() != "websocket: close 1000 (normal)" {
			return fmt.Errorf("terminal error: %w", err)
		}
	}

	fmt.Println("\nConnection closed.")
	return nil
}

func printShellHelp() {
	fmt.Printf(`%sdalang shell%s - Open shell to VM

%sUSAGE:%s
    dalang shell <name>

%sDESCRIPTION:%s
    Opens an interactive shell session to the specified VM.
    Uses lxc exec under the hood for direct shell access.

%sEXAMPLES:%s
    dalang shell MyVM

%sNOTE:%s
    The VM must be in RUNNING state to connect.
    Press Ctrl+C or type 'exit' to disconnect.
`,
		colorCyan, colorReset,
		colorYellow, colorReset,
		colorYellow, colorReset,
		colorYellow, colorReset,
		colorYellow, colorReset,
	)
}

func printConsoleHelp() {
	fmt.Printf(`%sdalang console%s - Open console to VM

%sUSAGE:%s
    dalang console <name>

%sDESCRIPTION:%s
    Opens a console session to the specified VM.
    Similar to shell but connects to the VM console directly.

%sEXAMPLES:%s
    dalang console MyVM

%sNOTE:%s
    The VM must be in RUNNING state to connect.
    Press Ctrl+C to disconnect.
`,
		colorCyan, colorReset,
		colorYellow, colorReset,
		colorYellow, colorReset,
		colorYellow, colorReset,
		colorYellow, colorReset,
	)
}
