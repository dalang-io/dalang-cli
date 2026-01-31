package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/dalang-io/dalang-cli/internal/api"
)

func cmdStart(args []string) error {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		printStartHelp()
		if len(args) == 0 {
			return fmt.Errorf("missing service name")
		}
		return nil
	}

	return vmAction(args[0], "start")
}

func cmdStop(args []string) error {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		printStopHelp()
		if len(args) == 0 {
			return fmt.Errorf("missing service name")
		}
		return nil
	}

	return vmAction(args[0], "stop")
}

func cmdDelete(args []string) error {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		printDeleteHelp()
		if len(args) == 0 {
			return fmt.Errorf("missing service name")
		}
		return nil
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

	// Find VPS by name
	vpsID, displayName, err := resolveVPSName(client, name)
	if err != nil {
		return err
	}

	printInfo("%sing %s...", strings.Title(action), displayName)

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
		printError("Failed: %s", result.Message)
	}

	return nil
}

func vmDelete(name string) error {
	client, err := api.NewAuthenticatedClient()
	if err != nil {
		return err
	}

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
		printError("Failed: %s", result.Message)
	}

	return nil
}

func resolveVPSName(client *api.Client, name string) (string, string, error) {
	vpsResp, err := client.Get("/vps/list")
	if err != nil {
		return "", "", fmt.Errorf("failed to fetch services: %w", err)
	}

	var vpsData api.VPSListResponse
	if err := json.Unmarshal(vpsResp, &vpsData); err != nil {
		return "", "", fmt.Errorf("failed to parse response: %w", err)
	}

	for _, v := range vpsData.Data.VPS {
		if v.Name == name || v.DisplayName == name {
			displayName := v.DisplayName
			if displayName == "" {
				displayName = v.Name
			}
			return v.ID, displayName, nil
		}
	}

	return "", "", fmt.Errorf("VPS '%s' not found", name)
}

func printStartHelp() {
	fmt.Printf(`%sdalang start%s - Start a VM

%sUSAGE:%s
    dalang start <name>

%sDESCRIPTION:%s
    Starts a stopped VM instance.

%sEXAMPLES:%s
    dalang start MyVM
`,
		colorCyan, colorReset,
		colorYellow, colorReset,
		colorYellow, colorReset,
		colorYellow, colorReset,
	)
}

func printStopHelp() {
	fmt.Printf(`%sdalang stop%s - Stop a VM

%sUSAGE:%s
    dalang stop <name>

%sDESCRIPTION:%s
    Stops a running VM instance.

%sEXAMPLES:%s
    dalang stop MyVM
`,
		colorCyan, colorReset,
		colorYellow, colorReset,
		colorYellow, colorReset,
		colorYellow, colorReset,
	)
}

func printDeleteHelp() {
	fmt.Printf(`%sdalang delete%s - Delete a VM

%sUSAGE:%s
    dalang delete <name>
    dalang delete <name> --yes

%sDESCRIPTION:%s
    Permanently deletes a VM and all its data.
    Requires confirmation unless --yes flag is provided.

%sOPTIONS:%s
    --yes, -y    Skip confirmation prompt

%sEXAMPLES:%s
    dalang delete MyVM           # Will prompt for confirmation
    dalang delete MyVM --yes     # Skip confirmation
`,
		colorCyan, colorReset,
		colorYellow, colorReset,
		colorYellow, colorReset,
		colorYellow, colorReset,
		colorYellow, colorReset,
	)
}
