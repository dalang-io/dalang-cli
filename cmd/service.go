package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/dalang-io/dalang-cli/internal/api"
)

// UnifiedService represents a service from any type
type UnifiedService struct {
	Type      string `json:"type"`
	ID        string `json:"id"`
	Name      string `json:"name"`
	Status    string `json:"status"`
	ExpiresAt string `json:"expires_at"`
	Details   string `json:"details,omitempty"`
}

func cmdService(args []string) error {
	if len(args) == 0 {
		return serviceList()
	}

	switch args[0] {
	case "list", "ls":
		return serviceList()
	case "info":
		if len(args) < 2 {
			printError("Usage: dalang service info <name>")
			return fmt.Errorf("missing service name")
		}
		return serviceInfo(args[1])
	case "create":
		return serviceCreate(args[1:])
	case "upgrade":
		if len(args) < 2 {
			printError("Usage: dalang service upgrade <name> [options]")
			return fmt.Errorf("missing service name")
		}
		return serviceUpgrade(args[1], args[2:])
	case "--help", "-h":
		printServiceHelp()
		return nil
	default:
		printError("Unknown service subcommand: %s", args[0])
		return fmt.Errorf("unknown subcommand: %s", args[0])
	}
}

func serviceList() error {
	client, err := api.NewAuthenticatedClient()
	if err != nil {
		return err
	}

	var services []UnifiedService

	// Fetch VPS
	vpsResp, err := client.Get("/vps/list")
	if err == nil {
		var vpsData api.VPSListResponse
		if json.Unmarshal(vpsResp, &vpsData) == nil {
			for _, v := range vpsData.Data.VPS {
				name := v.DisplayName
				if name == "" {
					name = v.Name
				}
				services = append(services, UnifiedService{
					Type:      "vps",
					ID:        v.ID,
					Name:      name,
					Status:    v.Status,
					ExpiresAt: v.ExpiresAt,
					Details:   fmt.Sprintf("%dC/%dMB/%dGB", v.VCPU, v.RAM, v.Storage),
				})
			}
		}
	}

	// Fetch containers
	contResp, err := client.Get("/containers/list")
	if err == nil {
		var contData api.ContainerListResponse
		if json.Unmarshal(contResp, &contData) == nil {
			for _, c := range contData.Data.Containers {
				services = append(services, UnifiedService{
					Type:      "container",
					ID:        c.ID,
					Name:      c.Name,
					Status:    c.Status,
					ExpiresAt: c.ExpiresAt,
					Details:   fmt.Sprintf("%s/%s", c.ContainerType, c.Plan),
				})
			}
		}
	}

	// Fetch deployments/apps
	appResp, err := client.Get("/github/deployments")
	if err == nil {
		var appData api.DeploymentListResponse
		if json.Unmarshal(appResp, &appData) == nil {
			for _, d := range appData.Data.Deployments {
				services = append(services, UnifiedService{
					Type:      "app",
					ID:        d.ID,
					Name:      d.Name,
					Status:    d.Status,
					ExpiresAt: d.ExpiresAt,
					Details:   d.Branch,
				})
			}
		}
	}

	if jsonOutput {
		data, _ := json.MarshalIndent(services, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	if len(services) == 0 {
		printInfo("No services found")
		fmt.Println("\nCreate your first service with:")
		fmt.Printf("  %sdalang service create --name MyVM --cpu 2 --ram 1G%s\n\n", colorCyan, colorReset)
		return nil
	}

	fmt.Printf("\n%sYour Services%s (%d total)\n", colorBold, colorReset, len(services))
	fmt.Println(strings.Repeat("─", 75))
	fmt.Printf("%-10s %-20s %-12s %-15s %s\n", "TYPE", "NAME", "STATUS", "EXPIRES", "DETAILS")
	fmt.Println(strings.Repeat("─", 75))

	for _, s := range services {
		statusColor := colorReset
		switch strings.ToUpper(s.Status) {
		case "RUNNING", "ACTIVE":
			statusColor = colorGreen
		case "STOPPED":
			statusColor = colorYellow
		case "CREATING", "PENDING":
			statusColor = colorCyan
		case "ERROR", "FAILED":
			statusColor = colorRed
		}

		name := s.Name
		if len(name) > 18 {
			name = name[:15] + "..."
		}

		expires := formatDate(s.ExpiresAt)

		fmt.Printf("%-10s %-20s %s%-12s%s %-15s %s\n",
			s.Type,
			name,
			statusColor, s.Status, colorReset,
			expires,
			s.Details,
		)
	}
	fmt.Println()

	return nil
}

func serviceInfo(name string) error {
	client, err := api.NewAuthenticatedClient()
	if err != nil {
		return err
	}

	// Try to find service by name in VPS list first
	vpsResp, err := client.Get("/vps/list")
	if err != nil {
		return fmt.Errorf("failed to fetch services: %w", err)
	}

	var vpsData api.VPSListResponse
	if err := json.Unmarshal(vpsResp, &vpsData); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	// Find VPS by name
	var foundVPS *api.VPS
	for _, v := range vpsData.Data.VPS {
		if v.Name == name || v.DisplayName == name {
			foundVPS = &v
			break
		}
	}

	if foundVPS == nil {
		// TODO: Check containers and apps
		printError("Service '%s' not found", name)
		return fmt.Errorf("service not found: %s", name)
	}

	if jsonOutput {
		data, _ := json.MarshalIndent(foundVPS, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	displayName := foundVPS.DisplayName
	if displayName == "" {
		displayName = foundVPS.Name
	}

	statusColor := colorReset
	switch strings.ToUpper(foundVPS.Status) {
	case "RUNNING":
		statusColor = colorGreen
	case "STOPPED":
		statusColor = colorYellow
	case "CREATING":
		statusColor = colorCyan
	case "ERROR", "FAILED":
		statusColor = colorRed
	}

	fmt.Printf("\n%s%s%s (VPS)\n", colorBold, displayName, colorReset)
	fmt.Println(strings.Repeat("─", 50))
	fmt.Printf("  Status:     %s%s%s\n", statusColor, foundVPS.Status, colorReset)
	fmt.Printf("  ID:         %s\n", foundVPS.ID)
	fmt.Printf("  Region:     %s\n", foundVPS.Region)
	fmt.Printf("  Image:      %s\n", foundVPS.Image)
	fmt.Println()
	fmt.Printf("  %sSpecs:%s\n", colorBold, colorReset)
	fmt.Printf("    CPU:       %d vCPU\n", foundVPS.VCPU)
	fmt.Printf("    RAM:       %d MB\n", foundVPS.RAM)
	fmt.Printf("    Storage:   %d GB (%s)\n", foundVPS.Storage, foundVPS.StorageType)
	fmt.Printf("    Bandwidth: %d Mbps\n", foundVPS.Bandwidth)
	fmt.Println()
	fmt.Printf("  %sNetwork:%s\n", colorBold, colorReset)
	fmt.Printf("    Public IP: %s%s%s\n", colorCyan, foundVPS.PublicIP, colorReset)
	fmt.Printf("    Local IP:  %s\n", foundVPS.LocalIP)
	fmt.Println()
	fmt.Printf("  %sSubscription:%s\n", colorBold, colorReset)
	fmt.Printf("    Price:     %s/month\n", formatIDR(int64(foundVPS.Price)))
	fmt.Printf("    Expires:   %s\n", formatDate(foundVPS.ExpiresAt))
	fmt.Println()

	return nil
}

func serviceCreate(args []string) error {
	// Parse flags
	var name, image, region string
	var cpu, ram, storage, bandwidth int

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--name", "-n":
			if i+1 < len(args) {
				name = args[i+1]
				i++
			}
		case "--cpu", "-c":
			if i+1 < len(args) {
				fmt.Sscanf(args[i+1], "%d", &cpu)
				i++
			}
		case "--ram", "-r":
			if i+1 < len(args) {
				ram = parseSize(args[i+1])
				i++
			}
		case "--storage", "-s":
			if i+1 < len(args) {
				storage = parseSize(args[i+1]) / 1024 // Convert MB to GB
				i++
			}
		case "--bandwidth", "-b":
			if i+1 < len(args) {
				bandwidth = parseSize(args[i+1])
				i++
			}
		case "--image", "-i":
			if i+1 < len(args) {
				image = args[i+1]
				i++
			}
		case "--region":
			if i+1 < len(args) {
				region = args[i+1]
				i++
			}
		case "--help", "-h":
			printServiceCreateHelp()
			return nil
		}
	}

	// Validate required fields
	if name == "" {
		printError("--name is required")
		return fmt.Errorf("missing required flag: --name")
	}

	// Set defaults
	if cpu == 0 {
		cpu = 1
	}
	if ram == 0 {
		ram = 512
	}
	if storage == 0 {
		storage = 5
	}
	if bandwidth == 0 {
		bandwidth = 20
	}
	if image == "" {
		image = "ubuntu"
	}
	if region == "" {
		region = "ID-BANTEN-01"
	}

	// Show summary and confirm
	fmt.Printf("\n%sCreate VPS%s\n", colorBold, colorReset)
	fmt.Println(strings.Repeat("─", 40))
	fmt.Printf("  Name:      %s\n", name)
	fmt.Printf("  CPU:       %d vCPU\n", cpu)
	fmt.Printf("  RAM:       %d MB\n", ram)
	fmt.Printf("  Storage:   %d GB\n", storage)
	fmt.Printf("  Bandwidth: %d Mbps\n", bandwidth)
	fmt.Printf("  Image:     %s\n", image)
	fmt.Printf("  Region:    %s\n", region)
	fmt.Println()

	if !yesFlag {
		if !confirmPrompt("Create this VPS?") {
			printInfo("Cancelled")
			return nil
		}
	}

	client, err := api.NewAuthenticatedClient()
	if err != nil {
		return err
	}

	printInfo("Creating VPS...")

	// Call VPS order endpoint
	resp, err := client.Post("/vps/order", map[string]interface{}{
		"name":       name,
		"cpu":        cpu,
		"ram":        ram,
		"storage":    storage,
		"bandwidth":  bandwidth,
		"image":      image,
		"region":     region,
		"pay_method": "credits",
	})

	if err != nil {
		return fmt.Errorf("failed to create VPS: %w", err)
	}

	var orderResp struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		Data    struct {
			OrderID    string `json:"order_id"`
			Price      int    `json:"price"`
			InvoiceURL string `json:"invoice_url,omitempty"`
			Status     string `json:"status"`
		} `json:"data"`
	}

	if err := json.Unmarshal(resp, &orderResp); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	if jsonOutput {
		fmt.Println(string(resp))
		return nil
	}

	if orderResp.Data.Status == "paid" || orderResp.Data.Status == "provisioning" {
		printSuccess("VPS creation initiated!")
		fmt.Printf("  Order ID: %s\n", orderResp.Data.OrderID)
		fmt.Printf("  Price: %s/month\n", formatIDR(int64(orderResp.Data.Price)))
		printInfo("VPS will be ready shortly. Check status with 'dalang service info %s'", name)
	} else if orderResp.Data.InvoiceURL != "" {
		printSuccess("Invoice created!")
		fmt.Printf("  Price: %s/month\n", formatIDR(int64(orderResp.Data.Price)))
		fmt.Printf("  Payment URL: %s%s%s\n", colorCyan, orderResp.Data.InvoiceURL, colorReset)
	}

	return nil
}

func serviceUpgrade(name string, args []string) error {
	printInfo("Upgrading %s...", name)

	// Parse upgrade flags
	var cpu, ram, storage, bandwidth int
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--cpu":
			if i+1 < len(args) {
				fmt.Sscanf(args[i+1], "%d", &cpu)
				i++
			}
		case "--ram":
			if i+1 < len(args) {
				ram = parseSize(args[i+1])
				i++
			}
		case "--storage":
			if i+1 < len(args) {
				storage = parseSize(args[i+1]) / 1024
				i++
			}
		case "--bandwidth":
			if i+1 < len(args) {
				bandwidth = parseSize(args[i+1])
				i++
			}
		}
	}

	if cpu == 0 && ram == 0 && storage == 0 && bandwidth == 0 {
		printError("No upgrade options specified")
		fmt.Println("Usage: dalang service upgrade <name> --cpu 4 --ram 2G")
		return fmt.Errorf("no upgrade options")
	}

	// TODO: Implement upgrade via API
	printWarn("Service upgrade not yet implemented")
	return nil
}

func parseSize(s string) int {
	s = strings.ToUpper(strings.TrimSpace(s))
	var value int
	var unit string

	fmt.Sscanf(s, "%d%s", &value, &unit)

	switch unit {
	case "G", "GB":
		return value * 1024 // Return in MB
	case "M", "MB":
		return value
	default:
		return value
	}
}

func confirmPrompt(message string) bool {
	fmt.Printf("%s [y/N]: ", message)
	var response string
	fmt.Scanln(&response)
	return strings.ToLower(strings.TrimSpace(response)) == "y"
}

func printServiceHelp() {
	fmt.Printf(`%sdalang service%s - Manage services

%sUSAGE:%s
    dalang service list              List all services
    dalang service info <name>       Show service details
    dalang service create [options]  Create new VPS
    dalang service upgrade <name>    Upgrade service

%sDESCRIPTION:%s
    Manage your VPS instances, containers, and app deployments.

%sEXAMPLES:%s
    dalang service list
    dalang service info MyVM
    dalang service create --name MyVM --cpu 2 --ram 1G
    dalang service upgrade MyVM --cpu 4

Run '%sdalang service create --help%s' for create options.
`,
		colorCyan, colorReset,
		colorYellow, colorReset,
		colorYellow, colorReset,
		colorYellow, colorReset,
		colorCyan, colorReset,
	)
}

func printServiceCreateHelp() {
	fmt.Printf(`%sdalang service create%s - Create new VPS

%sUSAGE:%s
    dalang service create [options]

%sOPTIONS:%s
    --name, -n <name>        Service name (required)
    --cpu, -c <count>        Number of vCPUs (default: 1)
    --ram, -r <size>         RAM size, e.g., 512M, 1G (default: 512M)
    --storage, -s <size>     Storage size, e.g., 5G, 10G (default: 5G)
    --bandwidth, -b <mbps>   Bandwidth in Mbps (default: 20)
    --image, -i <name>       OS image (default: ubuntu)
    --region <region>        Region (default: ID-BANTEN-01)

%sIMAGES:%s
    ubuntu, debian, centos, rocky, almalinux

%sREGIONS:%s
    ID-BANTEN-01, ID-BANTEN-02, ID-JAKARTA-01, ID-JAKARTA-02

%sEXAMPLES:%s
    dalang service create --name MyVM --cpu 2 --ram 1G --storage 10G
    dalang service create -n WebServer -c 4 -r 4G -s 50G -i ubuntu
`,
		colorCyan, colorReset,
		colorYellow, colorReset,
		colorYellow, colorReset,
		colorYellow, colorReset,
		colorYellow, colorReset,
		colorYellow, colorReset,
	)
}
