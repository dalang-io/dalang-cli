package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"strings"

	"github.com/dalang-io/dalang-cli/internal/api"
	"github.com/dalang-io/dalang-cli/internal/config"
	"github.com/dalang-io/dalang-cli/internal/terminal"
)

func cmdShell(args []string) error {
	if len(args) > 0 && (args[0] == "--help" || args[0] == "-h") {
		printShellHelp()
		return nil
	}
	if len(args) == 0 {
		return fmt.Errorf("missing service name. Usage: dalang shell <vm-name>")
	}

	return connectTerminal(args[0], "shell")
}

func cmdConsole(args []string) error {
	if len(args) > 0 && (args[0] == "--help" || args[0] == "-h") {
		printConsoleHelp()
		return nil
	}
	if len(args) == 0 {
		return fmt.Errorf("missing service name. Usage: dalang console <vm-name>")
	}

	return connectTerminal(args[0], "console")
}

func connectTerminal(name, mode string) error {
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

	// Load credentials for token
	creds, err := config.LoadCredentials()
	if err != nil {
		return fmt.Errorf("failed to load credentials: %w", err)
	}

	// Build WebSocket URL
	apiURL := config.GetAPIURL()
	wsURL := strings.Replace(apiURL, "https://", "wss://", 1)
	wsURL = strings.Replace(wsURL, "http://", "ws://", 1)

	wsURL = fmt.Sprintf("%s/vps/terminal?uuid=%s&mode=%s", wsURL, foundVPS.ID, mode)
	// force=true takes over an existing console session; only meaningful for
	// console mode, not the persistent shell.
	if mode == "console" {
		wsURL += "&force=true"
	}

	PrintDebug("WebSocket URL: %s", wsURL)

	displayName := foundVPS.DisplayName
	if displayName == "" {
		displayName = foundVPS.Name
	}

	// Show resource usage before connecting
	if strings.ToUpper(foundVPS.Status) == "RUNNING" {
		usageResp, err := client.Get(fmt.Sprintf("/vps/usage?vps_id=%s", foundVPS.ID))
		if err == nil {
			var usage struct {
				Success bool `json:"success"`
				Data    struct {
					CPUSeconds  float64 `json:"cpu_seconds"`
					CPUPercent  float64 `json:"cpu_percent"`
					MemoryUsed  int64   `json:"memory_used"`
					MemoryTotal int64   `json:"memory_total"`
					DiskUsed    int64   `json:"disk_used"`
					DiskTotal   int64   `json:"disk_total"`
				} `json:"data"`
			}
			if json.Unmarshal(usageResp, &usage) == nil && usage.Success {
				d := usage.Data
				// Live uptime + CPU load come from inside the VM (the usage
				// endpoint doesn't expose them); best-effort, never blocks.
				metrics, haveMetrics := fetchVMMetrics(client, foundVPS.ID)
				if d.MemoryTotal > 0 || d.DiskTotal > 0 || haveMetrics {
					fmt.Printf("\n%s%s%s — Resource Usage\n", colorBold, displayName, colorReset)
					// CPU utilization is computed server-side (/vps/usage →
					// cpu_percent); the in-VM load average is shown only as a hint.
					cpuPct := d.CPUPercent
					if cpuPct > 0 && haveMetrics {
						printVMUsageCPU(cpuPct, metrics)
					} else if haveMetrics {
						printVMMetrics(metrics)
					}
					if d.MemoryTotal > 0 {
						pct := float64(d.MemoryUsed) / float64(d.MemoryTotal) * 100
						fmt.Printf("  Memory: %s %.0f%% (%s / %s)\n",
							renderBar(pct, 20), pct, formatBytes(d.MemoryUsed), formatBytes(d.MemoryTotal))
					}
					if d.DiskTotal > 0 {
						pct := float64(d.DiskUsed) / float64(d.DiskTotal) * 100
						fmt.Printf("  Disk:   %s %.0f%% (%s / %s)\n",
							renderBar(pct, 20), pct, formatBytes(d.DiskUsed), formatBytes(d.DiskTotal))
					}
					fmt.Println()
				}
			}
		}
	}

	printInfo("Connecting to %s (%s mode)...", displayName, mode)

	// Connect to terminal
	term, err := terminal.NewTerminal(wsURL, creds.AccessToken)
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}
	defer term.Close()

	printSuccess("Connected.\n")

	// Set up signal handling (cross-platform)
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt)

	// Handle resize on Unix systems
	if runtime.GOOS != "windows" {
		setupResizeHandler(sigChan, term)
	}

	// Handle interrupt — Ctrl+C triggers graceful close
	go func() {
		<-sigChan
		term.Close()
		fmt.Println("\r\nDisconnected.")
	}()

	// Run terminal (blocks until connection closes or escape sequence)
	if err := term.Run(); err != nil {
		if err.Error() != "websocket: close 1000 (normal)" {
			return fmt.Errorf("terminal error: %w", err)
		}
	}
	return nil
}

func printShellHelp() {
	fmt.Printf(`%sdalang shell%s - Open persistent shell to VM

%sUSAGE:%s
    dalang shell <vm-name>

%sDESCRIPTION:%s
    Opens an interactive shell session to the specified VM using a
    persistent tmux session. If you disconnect (network drop, Ctrl+C),
    the session keeps running and you can reconnect later.

    This is similar to mosh - your session state (running processes,
    environment variables, current directory) persists across disconnects.

%sEXAMPLES:%s
    dalang shell MyVM              # Connect to VM named MyVM
    dalang shell WebServer         # Connect to VM named WebServer

%sPERSISTENCE:%s
    # Session persists across disconnects
    dalang shell myvm
    export FOO=bar
    # Disconnect (Ctrl+C or network drop)

    dalang shell myvm              # Reconnects to same session
    echo $FOO                      # Outputs "bar" - state preserved!

%sDISCONNECT:%s
    Press Enter, then type ~. (tilde followed by period) to disconnect.
    Or press Ctrl+C.
    Note: 'exit' will end the tmux session (not just disconnect).

%sRELATED:%s
    dalang exec <name> "cmd"       # Execute command without interactive shell

%sNOTE:%s
    - The VM must be in RUNNING state to connect
    - Use 'dalang start <name>' if the VM is stopped
    - VM names are case-sensitive
    - tmux is auto-installed if missing on first connect
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

func printConsoleHelp() {
	fmt.Printf(`%sdalang console%s - Open console to VM

%sUSAGE:%s
    dalang console <vm-name>

%sDESCRIPTION:%s
    Opens a console session to the specified VM.
    Similar to shell but connects to the VM's virtual console directly.
    Useful when shell access is not available.

%sEXAMPLES:%s
    dalang console MyVM            # Connect to VM named MyVM
    dalang console WebServer       # Connect to VM named WebServer

%sDISCONNECT:%s
    Press Enter, then type ~. (tilde followed by period) to disconnect.

%sNOTE:%s
    - The VM must be in RUNNING state to connect
    - Use 'dalang start <name>' if the VM is stopped
    - VM names are case-sensitive
`,
		colorCyan, colorReset,
		colorYellow, colorReset,
		colorYellow, colorReset,
		colorYellow, colorReset,
		colorYellow, colorReset,
		colorYellow, colorReset,
	)
}
