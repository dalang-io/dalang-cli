package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/dalang-io/dalang-cli/internal/api"
)

func cmdStart(args []string) error {
	if len(args) > 0 && (args[0] == "--help" || args[0] == "-h") {
		printStartHelp()
		return nil
	}
	if len(args) == 0 {
		return fmt.Errorf("missing service name. Usage: dalang start <vm-name>")
	}

	return vmAction(args[0], "start")
}

func cmdStop(args []string) error {
	if len(args) > 0 && (args[0] == "--help" || args[0] == "-h") {
		printStopHelp()
		return nil
	}
	if len(args) == 0 {
		return fmt.Errorf("missing service name. Usage: dalang stop <vm-name>")
	}

	return vmAction(args[0], "stop")
}

func cmdDelete(args []string) error {
	if len(args) > 0 && (args[0] == "--help" || args[0] == "-h") {
		printDeleteHelp()
		return nil
	}
	if len(args) == 0 {
		return fmt.Errorf("missing service name. Usage: dalang delete <vm-name>")
	}

	name := args[0]

	// Confirm deletion
	if !yesFlag {
		printWarn("This will permanently delete '%s' and all its data!", name)
		if !confirmPrompt("Are you sure you want to delete this VPS?") {
			printInfo("Cancelled")
			return nil
		}
	}

	return vmDelete(name)
}

func vmAction(name, action string) error {
	client, err := api.NewAuthenticatedClient()
	if err != nil {
		return err
	}
	client.Verbose = VerboseOutput

	// Find VPS by name
	vpsID, displayName, err := resolveVPSName(client, name)
	if err != nil {
		return err
	}

	printInfo("%sing %s...", strings.ToUpper(action[:1])+action[1:], displayName)

	resp, err := client.Post("/vps/action", map[string]interface{}{
		"id":     vpsID,
		"action": action,
	})

	if err != nil {
		return fmt.Errorf("failed to %s VPS: %w", action, err)
	}

	var result struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}

	if err := json.Unmarshal(resp, &result); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	if jsonOutput {
		fmt.Println(string(resp))
		return nil
	}

	if result.Success {
		printSuccess("VPS %s command sent successfully", action)
	} else {
		return fmt.Errorf("vps %s failed: %s", action, result.Message)
	}

	return nil
}

func vmDelete(name string) error {
	client, err := api.NewAuthenticatedClient()
	if err != nil {
		return err
	}
	client.Verbose = VerboseOutput

	// Find VPS by name
	vpsID, displayName, err := resolveVPSName(client, name)
	if err != nil {
		return err
	}

	printInfo("Deleting %s...", displayName)

	resp, err := client.Delete(fmt.Sprintf("/vps/delete?id=%s", vpsID))
	if err != nil {
		return fmt.Errorf("failed to delete VPS: %w", err)
	}

	var result struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}

	if err := json.Unmarshal(resp, &result); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	if jsonOutput {
		fmt.Println(string(resp))
		return nil
	}

	if result.Success {
		printSuccess("VPS '%s' deleted successfully", displayName)
	} else {
		return fmt.Errorf("delete failed: %s", result.Message)
	}

	return nil
}

func resolveVPSName(client *api.Client, name string) (string, string, error) {
	vps, err := findVPSByName(client, name)
	if err != nil {
		return "", "", err
	}
	displayName := vps.DisplayName
	if displayName == "" {
		displayName = vps.Name
	}
	return vps.ID, displayName, nil
}

func printStartHelp() {
	fmt.Printf(`%sdalang start%s - Start a VM

%sUSAGE:%s
    dalang start <vm-name>

%sDESCRIPTION:%s
    Starts a stopped VM instance. The VM will boot and become available
    for shell/console access.

%sEXAMPLES:%s
    dalang start MyVM              # Start VM named MyVM
    dalang start WebServer         # Start VM named WebServer

%sNOTE:%s
    - VM names are case-sensitive
    - Use 'dalang service list' to see available VMs
    - After starting, use 'dalang shell <name>' to connect
`,
		colorCyan, colorReset,
		colorYellow, colorReset,
		colorYellow, colorReset,
		colorYellow, colorReset,
		colorYellow, colorReset,
	)
}

func printStopHelp() {
	fmt.Printf(`%sdalang stop%s - Stop a VM

%sUSAGE:%s
    dalang stop <vm-name>

%sDESCRIPTION:%s
    Stops a running VM instance. The VM will shut down gracefully.
    All data is preserved and you can restart it later.

%sEXAMPLES:%s
    dalang stop MyVM               # Stop VM named MyVM
    dalang stop WebServer          # Stop VM named WebServer

%sNOTE:%s
    - VM names are case-sensitive
    - Use 'dalang service list' to see running VMs
    - Stopped VMs still incur charges (use 'dalang delete' to remove)
`,
		colorCyan, colorReset,
		colorYellow, colorReset,
		colorYellow, colorReset,
		colorYellow, colorReset,
		colorYellow, colorReset,
	)
}

func printDeleteHelp() {
	fmt.Printf(`%sdalang delete%s - Delete a VM

%sUSAGE:%s
    dalang delete <vm-name>
    dalang delete <vm-name> --yes

%sDESCRIPTION:%s
    Permanently deletes a VM and all its data. This action cannot be undone.
    Requires confirmation unless --yes flag is provided.

%sOPTIONS:%s
    --yes, -y    Skip confirmation prompt

%sEXAMPLES:%s
    dalang delete MyVM             # Will prompt for confirmation
    dalang delete MyVM --yes       # Skip confirmation
    dalang delete WebServer -y     # Short form, skip confirmation

%sWARNING:%s
    This will permanently delete:
    - All data stored on the VM
    - All custom domain configurations
    - Any associated backups
`,
		colorCyan, colorReset,
		colorYellow, colorReset,
		colorYellow, colorReset,
		colorYellow, colorReset,
		colorYellow, colorReset,
		colorYellow, colorReset,
	)
}
