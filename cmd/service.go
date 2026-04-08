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
	case "extend":
		if len(args) < 2 {
			printError("Usage: dalang service extend <name> [--months N]")
			return fmt.Errorf("missing service name")
		}
		return serviceExtend(args[1], args[2:])
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
	client.Verbose = VerboseOutput

	var services []UnifiedService
	var fetchFailures []string

	// Fetch VPS
	PrintDebug("Fetching VPS list...")
	vpsResp, err := client.Get("/vps/list")
	if err != nil {
		PrintDebug("VPS fetch error: %v", err)
		fetchFailures = append(fetchFailures, fmt.Sprintf("vps: %v", err))
	} else {
		var vpsData api.VPSListResponse
		if err := json.Unmarshal(vpsResp, &vpsData); err != nil {
			PrintDebug("VPS parse error: %v", err)
			fetchFailures = append(fetchFailures, fmt.Sprintf("vps parse: %v", err))
		} else {
			PrintDebug("Found %d VPS", len(vpsData.Data))
			for _, v := range vpsData.Data {
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
	PrintDebug("Fetching containers list...")
	contResp, err := client.Get("/containers/list")
	if err != nil {
		PrintDebug("Containers fetch error: %v", err)
		fetchFailures = append(fetchFailures, fmt.Sprintf("containers: %v", err))
	} else {
		var contData api.ContainerListResponse
		if err := json.Unmarshal(contResp, &contData); err != nil {
			PrintDebug("Containers parse error: %v", err)
			fetchFailures = append(fetchFailures, fmt.Sprintf("containers parse: %v", err))
		} else {
			PrintDebug("Found %d containers", len(contData.Data.Containers))
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
	PrintDebug("Fetching deployments list...")
	appResp, err := client.Get("/github/deployments")
	if err != nil {
		PrintDebug("Deployments fetch error: %v", err)
		fetchFailures = append(fetchFailures, fmt.Sprintf("deployments: %v", err))
	} else {
		var appData api.DeploymentListResponse
		if err := json.Unmarshal(appResp, &appData); err != nil {
			PrintDebug("Deployments parse error: %v", err)
			fetchFailures = append(fetchFailures, fmt.Sprintf("deployments parse: %v", err))
		} else {
			PrintDebug("Found %d deployments", len(appData.Data))
			for _, d := range appData.Data {
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

	if len(services) == 0 && len(fetchFailures) > 0 {
		return fmt.Errorf("failed to fetch services: %s", strings.Join(fetchFailures, "; "))
	}

	if jsonOutput {
		data, _ := json.MarshalIndent(services, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	if len(fetchFailures) > 0 {
		printWarn("Some services could not be loaded")
		PrintDebug("Partial fetch failures: %s", strings.Join(fetchFailures, "; "))
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
	client.Verbose = VerboseOutput

	// Find VPS by name first
	vps, err := findVPSByName(client, name)
	if err != nil {
		printError("Service '%s' not found", name)
		return fmt.Errorf("service not found: %s", name)
	}

	// Sync this specific VPS's specs/status from incus
	syncResp, err := client.Post("/vps/sync-specs", map[string]interface{}{
		"vps_id": vps.ID,
	})
	if err == nil {
		// Re-fetch the VPS to get updated data
		var syncResult struct {
			Success bool `json:"success"`
		}
		if json.Unmarshal(syncResp, &syncResult) == nil && syncResult.Success {
			if updated, err := findVPSByName(client, name); err == nil {
				vps = updated
			}
		}
	}

	foundVPS := vps

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
	if foundVPS.Node != "" {
		fmt.Printf("  Node:       %s\n", foundVPS.Node)
	}
	fmt.Println()
	fmt.Printf("  %sSpecs:%s\n", colorBold, colorReset)
	fmt.Printf("    CPU:       %d vCPU\n", foundVPS.VCPU)
	fmt.Printf("    RAM:       %d MB\n", foundVPS.RAM)
	fmt.Printf("    Storage:   %d GB (%s)\n", foundVPS.Storage, foundVPS.StorageType)
	fmt.Printf("    Bandwidth: %d Mbps\n", foundVPS.Bandwidth)
	fmt.Println()
	fmt.Printf("  %sNetwork:%s\n", colorBold, colorReset)
	publicIP := foundVPS.PublicIP
	if publicIP == "" {
		publicIP = "N/A"
	}
	localIP := foundVPS.LocalIP
	if localIP == "" {
		localIP = "N/A"
	}
	fmt.Printf("    Public IP: %s%s%s\n", colorCyan, publicIP, colorReset)
	fmt.Printf("    Local IP:  %s\n", localIP)
	fmt.Println()
	fmt.Printf("  %sDomains:%s\n", colorBold, colorReset)
	if foundVPS.Domain != "" {
		fmt.Printf("    Public:    %s%s%s (free)\n", colorCyan, foundVPS.Domain, colorReset)
	}
	if foundVPS.CustomDomainEnabled == 1 {
		fmt.Printf("    Custom:    %s%s%s (+%s/month)\n", colorGreen, "Enabled", colorReset, formatIDR(int64(foundVPS.CustomDomainPrice)))
		fmt.Printf("               Run '%sdalang domain list %s%s' to see custom domains\n", colorCyan, name, colorReset)
	} else {
		fmt.Printf("    Custom:    %sDisabled%s\n", colorYellow, colorReset)
		fmt.Printf("               Run '%sdalang domain enable %s%s' to activate (+%s/month)\n", colorCyan, name, colorReset, formatIDR(int64(foundVPS.CustomDomainPrice)))
	}
	fmt.Println()
	// Fetch resource usage if running
	if strings.ToUpper(foundVPS.Status) == "RUNNING" {
		usageResp, err := client.Get(fmt.Sprintf("/vps/usage?vps_id=%s", foundVPS.ID))
		if err == nil {
			var usage struct {
				Success bool `json:"success"`
				Data    struct {
					CPUSeconds  float64 `json:"cpu_seconds"`
					MemoryUsed  int64   `json:"memory_used"`
					MemoryTotal int64   `json:"memory_total"`
					DiskUsed    int64   `json:"disk_used"`
					DiskTotal   int64   `json:"disk_total"`
				} `json:"data"`
			}
			if json.Unmarshal(usageResp, &usage) == nil && usage.Success {
				d := usage.Data
				fmt.Printf("  %sResource Usage:%s\n", colorBold, colorReset)
				if d.MemoryTotal > 0 {
					pct := float64(d.MemoryUsed) / float64(d.MemoryTotal) * 100
					fmt.Printf("    Memory:    %s  %.0f%% (%s / %s)\n",
						renderBar(pct, 20), pct, formatBytes(d.MemoryUsed), formatBytes(d.MemoryTotal))
				}
				if d.DiskTotal > 0 {
					pct := float64(d.DiskUsed) / float64(d.DiskTotal) * 100
					fmt.Printf("    Disk:      %s  %.0f%% (%s / %s)\n",
						renderBar(pct, 20), pct, formatBytes(d.DiskUsed), formatBytes(d.DiskTotal))
				}
				if d.CPUSeconds > 0 {
					fmt.Printf("    CPU Time:  %.1f hours\n", d.CPUSeconds/3600)
				}
				fmt.Println()
			}
		}
	}

	fmt.Printf("  %sSubscription:%s\n", colorBold, colorReset)
	fmt.Printf("    Price:     %s/month\n", formatIDR(int64(foundVPS.Price)))
	fmt.Printf("    Expires:   %s\n", formatExpiryWithDays(foundVPS.ExpiresAt))
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
		region = "ID-BANTEN-02"
	}

	// Calculate price
	price := CalculateVPSPrice(cpu, ram, storage, bandwidth)

	// Show summary and confirm
	fmt.Printf("\n%sCreate VPS%s\n", colorBold, colorReset)
	fmt.Println(strings.Repeat("─", 45))
	fmt.Printf("  Name:      %s\n", name)
	fmt.Printf("  CPU:       %d vCPU\n", cpu)
	fmt.Printf("  RAM:       %d MB\n", ram)
	fmt.Printf("  Storage:   %d GB\n", storage)
	fmt.Printf("  Bandwidth: %d Mbps\n", bandwidth)
	fmt.Printf("  Image:     %s\n", image)
	fmt.Printf("  Region:    %s\n", region)
	fmt.Println(strings.Repeat("─", 45))
	fmt.Printf("  %sEstimated Price: %s%s/month%s\n", colorBold, colorGreen, formatIDR(int64(price)), colorReset)
	fmt.Println()
	fmt.Printf("  %sTip:%s Run '%sdalang price%s' to see pricing details\n", colorYellow, colorReset, colorCyan, colorReset)
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
	client.Verbose = VerboseOutput

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
	// Find VPS first
	client, err := api.NewAuthenticatedClient()
	if err != nil {
		return err
	}
	client.Verbose = VerboseOutput

	vpsResp, err := client.Get("/vps/list")
	if err != nil {
		return fmt.Errorf("failed to fetch services: %w", err)
	}

	var vpsData api.VPSListResponse
	if err := json.Unmarshal(vpsResp, &vpsData); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	var foundVPS *api.VPS
	for _, v := range vpsData.Data {
		if v.Name == name || v.DisplayName == name {
			foundVPS = &v
			break
		}
	}

	if foundVPS == nil {
		printError("VPS '%s' not found", name)
		return fmt.Errorf("VPS not found: %s", name)
	}

	// Parse upgrade flags - start with current values
	cpu := foundVPS.VCPU
	ram := foundVPS.RAM / 1024 // Convert MB to GB for display/input
	storage := foundVPS.Storage
	bandwidth := foundVPS.Bandwidth

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--cpu", "-c":
			if i+1 < len(args) {
				fmt.Sscanf(args[i+1], "%d", &cpu)
				i++
			}
		case "--ram", "-r":
			if i+1 < len(args) {
				ramMB := parseSize(args[i+1]) // always returns MB
				ram = ramMB / 1024
				if ramMB > 0 && ram == 0 {
					ram = 1 // minimum 1 GB for sub-GB values like 512M
				}
				i++
			}
		case "--storage", "-s":
			if i+1 < len(args) {
				storageMB := parseSize(args[i+1])
				storage = storageMB / 1024
				if storageMB > 0 && storage == 0 {
					storage = 1 // minimum 1 GB
				}
				i++
			}
		case "--bandwidth", "-b":
			if i+1 < len(args) {
				fmt.Sscanf(args[i+1], "%d", &bandwidth)
				i++
			}
		case "--help", "-h":
			printUpgradeHelp()
			return nil
		}
	}

	// Check if any upgrade is requested
	currentRAM := foundVPS.RAM / 1024
	if cpu <= foundVPS.VCPU && ram <= currentRAM && storage <= foundVPS.Storage && bandwidth <= foundVPS.Bandwidth {
		printError("No upgrade specified. New values must be higher than current.")
		fmt.Printf("\nCurrent specs:\n")
		fmt.Printf("  CPU:       %d vCPU\n", foundVPS.VCPU)
		fmt.Printf("  RAM:       %d GB\n", currentRAM)
		fmt.Printf("  Storage:   %d GB\n", foundVPS.Storage)
		fmt.Printf("  Bandwidth: %d Mbps\n", foundVPS.Bandwidth)
		fmt.Println("\nUsage: dalang service upgrade <name> --cpu 4 --ram 4G --storage 50G")
		return fmt.Errorf("no upgrade specified")
	}

	// Calculate prices
	currentPrice := CalculateVPSPrice(foundVPS.VCPU, foundVPS.RAM, foundVPS.Storage, foundVPS.Bandwidth)
	newPrice := CalculateVPSPrice(cpu, ram*1024, storage, bandwidth) // ram in MB for price calc
	priceDiff := newPrice - currentPrice

	// Show upgrade summary
	displayName := foundVPS.DisplayName
	if displayName == "" {
		displayName = foundVPS.Name
	}

	fmt.Printf("\n%sUpgrade VPS: %s%s\n", colorBold, displayName, colorReset)
	fmt.Println(strings.Repeat("─", 50))
	fmt.Printf("  %-12s %s%-8s%s → %s%s%s\n", "CPU:", colorYellow, fmt.Sprintf("%d vCPU", foundVPS.VCPU), colorReset, colorGreen, fmt.Sprintf("%d vCPU", cpu), colorReset)
	fmt.Printf("  %-12s %s%-8s%s → %s%s%s\n", "RAM:", colorYellow, fmt.Sprintf("%d GB", currentRAM), colorReset, colorGreen, fmt.Sprintf("%d GB", ram), colorReset)
	fmt.Printf("  %-12s %s%-8s%s → %s%s%s\n", "Storage:", colorYellow, fmt.Sprintf("%d GB", foundVPS.Storage), colorReset, colorGreen, fmt.Sprintf("%d GB", storage), colorReset)
	fmt.Printf("  %-12s %s%-8s%s → %s%s%s\n", "Bandwidth:", colorYellow, fmt.Sprintf("%d Mbps", foundVPS.Bandwidth), colorReset, colorGreen, fmt.Sprintf("%d Mbps", bandwidth), colorReset)
	fmt.Println(strings.Repeat("─", 50))
	fmt.Printf("  Current price: %s/month\n", formatIDR(int64(currentPrice)))
	fmt.Printf("  New price:     %s/month\n", formatIDR(int64(newPrice)))
	fmt.Printf("  %sDifference:    +%s/month%s\n", colorGreen, formatIDR(int64(priceDiff)), colorReset)
	fmt.Println()

	if !yesFlag {
		if !confirmPrompt("Proceed with upgrade?") {
			printInfo("Cancelled")
			return nil
		}
	}

	printInfo("Creating upgrade invoice...")

	// Call upgrade API with credits payment
	resp, err := client.Post("/vps/upgrade", map[string]interface{}{
		"vps_id":     foundVPS.ID,
		"vcpu":       cpu,
		"ram":        ram, // API expects GB
		"storage":    storage,
		"bandwidth":  bandwidth,
		"pay_method": "credits",
	})

	if err != nil {
		return fmt.Errorf("failed to create upgrade invoice: %w", err)
	}

	var upgradeResp struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		Error   string `json:"error"`
		Data    struct {
			InvoiceURL string `json:"invoice_url"`
			Price      int    `json:"price"`
		} `json:"data"`
	}

	if err := json.Unmarshal(resp, &upgradeResp); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	if jsonOutput {
		fmt.Println(string(resp))
		return nil
	}

	if !upgradeResp.Success {
		errMsg := upgradeResp.Error
		if errMsg == "" {
			errMsg = upgradeResp.Message
		}
		if errMsg == "" {
			errMsg = "Failed to upgrade VPS"
		}
		return fmt.Errorf("%s", errMsg)
	}

	price := upgradeResp.Data.Price
	if price == 0 {
		price = priceDiff
	}
	printSuccess("VPS upgraded successfully!")
	fmt.Printf("  Amount paid: %s (from credits)\n", formatIDR(int64(price)))
	fmt.Println()

	return nil
}

func serviceExtend(name string, args []string) error {
	// Find VPS first
	client, err := api.NewAuthenticatedClient()
	if err != nil {
		return err
	}
	client.Verbose = VerboseOutput

	vpsResp, err := client.Get("/vps/list")
	if err != nil {
		return fmt.Errorf("failed to fetch services: %w", err)
	}

	var vpsData api.VPSListResponse
	if err := json.Unmarshal(vpsResp, &vpsData); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	var foundVPS *api.VPS
	for _, v := range vpsData.Data {
		if v.Name == name || v.DisplayName == name {
			foundVPS = &v
			break
		}
	}

	if foundVPS == nil {
		printError("VPS '%s' not found", name)
		return fmt.Errorf("VPS not found: %s", name)
	}

	// Parse months flag
	months := 1 // default 1 month
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--months", "-m":
			if i+1 < len(args) {
				fmt.Sscanf(args[i+1], "%d", &months)
				i++
			}
		case "--help", "-h":
			printExtendHelp()
			return nil
		}
	}

	// Validate months
	validMonths := map[int]bool{1: true, 3: true, 6: true, 12: true}
	if !validMonths[months] {
		printError("Invalid billing period. Use 1, 3, 6, or 12 months.")
		return fmt.Errorf("invalid billing period: %d", months)
	}

	// Calculate price
	monthlyPrice := foundVPS.Price
	if monthlyPrice == 0 {
		monthlyPrice = CalculateVPSPrice(foundVPS.VCPU, foundVPS.RAM, foundVPS.Storage, foundVPS.Bandwidth)
	}
	totalPrice := monthlyPrice * months

	// Show extension summary
	displayName := foundVPS.DisplayName
	if displayName == "" {
		displayName = foundVPS.Name
	}

	periodLabel := fmt.Sprintf("%d month", months)
	if months > 1 {
		periodLabel += "s"
	}

	fmt.Printf("\n%sExtend Subscription: %s%s\n", colorBold, displayName, colorReset)
	fmt.Println(strings.Repeat("─", 50))
	fmt.Printf("  Current expiry: %s\n", formatDate(foundVPS.ExpiresAt))
	fmt.Printf("  Extension:      %s\n", periodLabel)
	fmt.Printf("  Monthly price:  %s\n", formatIDR(int64(monthlyPrice)))
	fmt.Println(strings.Repeat("─", 50))
	fmt.Printf("  %sTotal: %s%s\n", colorBold, formatIDR(int64(totalPrice)), colorReset)
	fmt.Println()

	if !yesFlag {
		if !confirmPrompt("Proceed with extension?") {
			printInfo("Cancelled")
			return nil
		}
	}

	printInfo("Creating extension invoice...")

	// Call extend API with credits payment
	resp, err := client.Post("/bills/extend-subscription", map[string]interface{}{
		"vps_id":         foundVPS.ID,
		"billing_period": months,
		"pay_method":     "credits",
	})

	if err != nil {
		return fmt.Errorf("failed to create extension invoice: %w", err)
	}

	var extendResp struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		Error   string `json:"error"`
		Data    struct {
			InvoiceURL string `json:"invoice_url"`
			Price      int    `json:"price"`
			NewExpiry  string `json:"new_expiry"`
		} `json:"data"`
	}

	if err := json.Unmarshal(resp, &extendResp); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	if jsonOutput {
		fmt.Println(string(resp))
		return nil
	}

	if !extendResp.Success {
		errMsg := extendResp.Error
		if errMsg == "" {
			errMsg = extendResp.Message
		}
		if errMsg == "" {
			errMsg = "Failed to extend subscription"
		}
		return fmt.Errorf("%s", errMsg)
	}

	printSuccess("Subscription extended successfully!")
	fmt.Printf("  Amount paid: %s (from credits)\n", formatIDR(int64(totalPrice)))
	if extendResp.Data.NewExpiry != "" {
		fmt.Printf("  New expiry: %s\n", formatDate(extendResp.Data.NewExpiry))
	}
	fmt.Println()

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
    dalang service list                    List all services
    dalang service info <name>             Show service details
    dalang service create [options]        Create new VPS
    dalang service upgrade <name> [opts]   Upgrade VPS specs
    dalang service extend <name> [opts]    Extend subscription

%sDESCRIPTION:%s
    Manage your VPS instances, containers, and app deployments.

%sSUBCOMMANDS:%s
    list      List all your services (VPS, containers, apps)
    info      Show detailed information about a service
    create    Create a new VPS instance
    upgrade   Upgrade VPS CPU/RAM/storage/bandwidth
    extend    Extend VPS subscription period

%sEXAMPLES:%s
    # List all services
    dalang service list

    # Show details of a specific VM
    dalang service info MyVM

    # Create a VPS with custom specs
    dalang service create --name WebServer --cpu 2 --ram 2G --storage 20G

    # Upgrade VPS specs
    dalang service upgrade MyVM --cpu 4 --ram 4G

    # Extend subscription by 3 months
    dalang service extend MyVM --months 3

Run '%sdalang service <command> --help%s' for command-specific options.
`,
		colorCyan, colorReset,
		colorYellow, colorReset,
		colorYellow, colorReset,
		colorYellow, colorReset,
		colorYellow, colorReset,
		colorCyan, colorReset,
	)
}

func printUpgradeHelp() {
	fmt.Printf(`%sdalang service upgrade%s - Upgrade VPS specs

%sUSAGE:%s
    dalang service upgrade <name> [options]

%sOPTIONS:%s
    --cpu, -c <count>        New vCPU count (must be >= current)
    --ram, -r <size>         New RAM size, e.g., 4G (must be >= current)
    --storage, -s <size>     New storage size, e.g., 50G (must be >= current)
    --bandwidth, -b <mbps>   New bandwidth in Mbps (must be >= current)

%sNOTE:%s
    - Upgrades are permanent, downgrade is not allowed
    - Price difference is prorated for remaining subscription days
    - Minimum upgrade cost is Rp 10.000

%sEXAMPLES:%s
    dalang service upgrade MyVM --cpu 4
    dalang service upgrade MyVM --ram 4G --storage 50G
    dalang service upgrade MyVM -c 4 -r 8G -s 100G -b 100
`,
		colorCyan, colorReset,
		colorYellow, colorReset,
		colorYellow, colorReset,
		colorYellow, colorReset,
		colorYellow, colorReset,
	)
}

func printExtendHelp() {
	fmt.Printf(`%sdalang service extend%s - Extend VPS subscription

%sUSAGE:%s
    dalang service extend <name> [options]

%sOPTIONS:%s
    --months, -m <count>     Extension period: 1, 3, 6, or 12 months (default: 1)

%sBILLING PERIODS:%s
    1 month   - Standard monthly billing
    3 months  - Quarterly billing
    6 months  - Semi-annual billing
    12 months - Annual billing

%sEXAMPLES:%s
    dalang service extend MyVM              # Extend by 1 month
    dalang service extend MyVM --months 3   # Extend by 3 months
    dalang service extend MyVM -m 12        # Extend by 12 months
`,
		colorCyan, colorReset,
		colorYellow, colorReset,
		colorYellow, colorReset,
		colorYellow, colorReset,
		colorYellow, colorReset,
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
    --region <region>        Region (default: ID-BANTEN-02)

%sPRICING:%s
    vCPU:       Rp 20.000/vCPU/month
    RAM:        Rp 5.000/GB/month
    Storage:    Rp 1.000/GB/month
    Bandwidth:  20 Mbps included FREE, +Rp 20.000 per additional 20 Mbps

    Run '%sdalang price%s' for detailed pricing info.

%sIMAGES:%s
    ubuntu           Ubuntu 24.04 (default)
    ubuntu:24.04     Ubuntu 24.04 LTS
    ubuntu:22.04     Ubuntu 22.04 LTS
    ubuntu:20.04     Ubuntu 20.04 LTS
    debian           Debian 12
    debian:12        Debian 12
    debian:11        Debian 11
    centos           CentOS Stream 9
    rocky            Rocky Linux 9
    almalinux        AlmaLinux 9
    fedora           Fedora 40

%sREGIONS:%s
    ID-BANTEN-02 (default)

%sEXAMPLES:%s
    dalang service create --name MyVM --cpu 2 --ram 1G --storage 10G
    dalang service create --name WebServer --cpu 1 --ram 1G --image ubuntu:24.04
    dalang service create -n DevBox -c 2 -r 2G -s 20G -i debian:12
`,
		colorCyan, colorReset,
		colorYellow, colorReset,
		colorYellow, colorReset,
		colorYellow, colorReset,
		colorCyan, colorReset,
		colorYellow, colorReset,
		colorYellow, colorReset,
		colorYellow, colorReset,
	)
}
