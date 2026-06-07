package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/dalang-io/dalang-cli/internal/api"
)

func cmdDomain(args []string) error {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		printDomainHelp()
		return nil
	}

	switch args[0] {
	case "enable":
		if len(args) < 2 {
			printUsage("Usage: dalang domain enable <vps-name>")
			return fmt.Errorf("missing VPS name")
		}
		return domainEnable(args[1])
	case "list", "ls":
		if len(args) < 2 {
			printUsage("Usage: dalang domain list <vps-name>")
			return fmt.Errorf("missing VPS name")
		}
		return domainList(args[1])
	case "add":
		if len(args) < 3 {
			printUsage("Usage: dalang domain add <vps-name> <domain>")
			return fmt.Errorf("missing VPS name or domain")
		}
		return domainAdd(args[1], args[2])
	case "verify":
		if len(args) < 2 {
			printUsage("Usage: dalang domain verify <domain>")
			return fmt.Errorf("missing domain")
		}
		return domainVerify(args[1])
	case "remove", "rm":
		if len(args) < 2 {
			printUsage("Usage: dalang domain remove <domain>")
			return fmt.Errorf("missing domain")
		}
		return domainRemove(args[1])
	default:
		return fmt.Errorf("unknown domain subcommand: %s", args[0])
	}
}

func domainEnable(vpsName string) error {
	client, err := api.NewAuthenticatedClient()
	if err != nil {
		return err
	}
	client.Verbose = VerboseOutput

	// Find VPS by name
	vps, err := findVPSByName(client, vpsName)
	if err != nil {
		return err
	}

	if vps.CustomDomainEnabled == 1 {
		printSuccess("Custom domain is already enabled for %s", vpsName)
		return nil
	}

	if !yesFlag {
		fmt.Printf("\nEnable custom domain for %s%s%s?\n", colorCyan, vpsName, colorReset)
		fmt.Printf("Price: %s%s/month%s\n\n", colorYellow, formatIDR(int64(vps.CustomDomainPrice)), colorReset)
		if !confirmPrompt("Proceed with payment?") {
			printInfo("Cancelled")
			return nil
		}
	}

	printInfo("Creating invoice...")

	resp, err := client.Post("/vps/addon/custom-domain", map[string]interface{}{
		"vps_id": vps.ID,
	})
	if err != nil {
		return fmt.Errorf("failed to enable custom domain: %w", err)
	}

	var result struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		Data    struct {
			InvoiceURL string `json:"invoice_url"`
			Amount     int    `json:"amount"`
		} `json:"data"`
	}

	if err := json.Unmarshal(resp, &result); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	if !result.Success {
		return errors.New(result.Message)
	}

	if jsonOutput {
		fmt.Println(string(resp))
		return nil
	}

	printSuccess("Invoice created!")
	fmt.Printf("\n  Amount: %s\n", formatIDR(int64(result.Data.Amount)))
	fmt.Printf("  Pay at: %s%s%s\n\n", colorCyan, result.Data.InvoiceURL, colorReset)
	printInfo("After payment, custom domain will be automatically enabled")

	return nil
}

func domainList(vpsName string) error {
	client, err := api.NewAuthenticatedClient()
	if err != nil {
		return err
	}
	client.Verbose = VerboseOutput

	// Find VPS by name
	vps, err := findVPSByName(client, vpsName)
	if err != nil {
		return err
	}

	if vps.CustomDomainEnabled != 1 {
		printWarn("Custom domain is not enabled for %s", vpsName)
		fmt.Printf("\nRun '%sdalang domain enable %s%s' to activate\n", colorCyan, vpsName, colorReset)
		return nil
	}

	resp, err := client.Get(fmt.Sprintf("/vps/domains/list?vps_id=%s", vps.ID))
	if err != nil {
		return fmt.Errorf("failed to list domains: %w", err)
	}

	var result struct {
		Success bool `json:"success"`
		Data    []struct {
			ID        int    `json:"id"`
			Domain    string `json:"domain"`
			Status    string `json:"status"`
			CreatedAt string `json:"created_at"`
		} `json:"data"`
	}

	if err := json.Unmarshal(resp, &result); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	if jsonOutput {
		fmt.Println(string(resp))
		return nil
	}

	fmt.Printf("\n%sCustom Domains for %s%s\n", colorBold, vpsName, colorReset)
	fmt.Println(strings.Repeat("─", 60))

	if len(result.Data) == 0 {
		printInfo("No custom domains configured")
		fmt.Printf("\nAdd a domain with: %sdalang domain add %s <your-domain.com>%s\n", colorCyan, vpsName, colorReset)
		return nil
	}

	fmt.Printf("%-30s %-12s %s\n", "DOMAIN", "STATUS", "CREATED")
	fmt.Println(strings.Repeat("─", 60))

	for _, d := range result.Data {
		statusColor := colorYellow
		if d.Status == "active" {
			statusColor = colorGreen
		} else if d.Status == "failed" {
			statusColor = colorRed
		}

		fmt.Printf("%-30s %s%-12s%s %s\n",
			d.Domain,
			statusColor, d.Status, colorReset,
			formatDate(d.CreatedAt),
		)
	}
	fmt.Println()

	return nil
}

func domainAdd(vpsName, domain string) error {
	client, err := api.NewAuthenticatedClient()
	if err != nil {
		return err
	}
	client.Verbose = VerboseOutput

	// Find VPS by name
	vps, err := findVPSByName(client, vpsName)
	if err != nil {
		return err
	}

	if vps.CustomDomainEnabled != 1 {
		fmt.Printf("Run '%sdalang domain enable %s%s' first\n", colorCyan, vpsName, colorReset)
		return fmt.Errorf("custom domain is not enabled for %s", vpsName)
	}

	printInfo("Adding domain %s...", domain)

	resp, err := client.Post("/vps/domains/add", map[string]interface{}{
		"vps_id": vps.ID,
		"domain": domain,
	})
	if err != nil {
		return fmt.Errorf("failed to add domain: %w", err)
	}

	var result struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		Data    struct {
			Domain       string              `json:"domain"`
			Status       string              `json:"status"`
			DNSRecords   []map[string]string `json:"dns_records"`
			Instructions string              `json:"instructions"`
		} `json:"data"`
	}

	if err := json.Unmarshal(resp, &result); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	if !result.Success {
		return errors.New(result.Message)
	}

	if jsonOutput {
		fmt.Println(string(resp))
		return nil
	}

	printSuccess("Domain added!")
	fmt.Printf("\n%sConfigure these DNS records:%s\n\n", colorBold, colorReset)

	for _, record := range result.Data.DNSRecords {
		fmt.Printf("  %s%-6s%s  %s\n", colorCyan, record["type"], colorReset, record["name"])
		fmt.Printf("          → %s\n", record["value"])
		fmt.Printf("          %s%s%s\n\n", colorYellow, record["description"], colorReset)
	}

	fmt.Printf("After configuring DNS, run: %sdalang domain verify %s%s\n\n", colorCyan, domain, colorReset)

	return nil
}

func domainVerify(domain string) error {
	client, err := api.NewAuthenticatedClient()
	if err != nil {
		return err
	}
	client.Verbose = VerboseOutput

	printInfo("Verifying domain %s...", domain)

	resp, err := client.Post("/vps/domains/verify", map[string]interface{}{
		"domain": domain,
	})
	if err != nil {
		return fmt.Errorf("failed to verify domain: %w", err)
	}

	var result struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		Data    struct {
			Domain     string              `json:"domain"`
			Status     string              `json:"status"`
			SSLStatus  string              `json:"ssl_status"`
			DNSRecords []map[string]string `json:"dns_records"`
		} `json:"data"`
	}

	if err := json.Unmarshal(resp, &result); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	if jsonOutput {
		fmt.Println(string(resp))
		return nil
	}

	if result.Success && result.Data.Status == "active" {
		printSuccess("Domain %s is verified and active!", domain)
		return nil
	}

	printWarn("Domain verification in progress")
	fmt.Printf("\n  Domain: %s\n", result.Data.Domain)
	fmt.Printf("  Status: %s\n", result.Data.Status)
	fmt.Printf("  SSL:    %s\n\n", result.Data.SSLStatus)

	if len(result.Data.DNSRecords) > 0 {
		fmt.Printf("%sEnsure these DNS records are configured:%s\n\n", colorBold, colorReset)
		for _, record := range result.Data.DNSRecords {
			fmt.Printf("  %s%-6s%s  %s\n", colorCyan, record["type"], colorReset, record["name"])
			fmt.Printf("          → %s\n\n", record["value"])
		}
	}

	printInfo("DNS propagation can take up to 24 hours. Try again later.")

	return nil
}

func domainRemove(domain string) error {
	client, err := api.NewAuthenticatedClient()
	if err != nil {
		return err
	}
	client.Verbose = VerboseOutput

	if !yesFlag {
		fmt.Printf("\nRemove domain %s%s%s?\n\n", colorCyan, domain, colorReset)
		if !confirmPrompt("Are you sure?") {
			printInfo("Cancelled")
			return nil
		}
	}

	printInfo("Removing domain %s...", domain)

	resp, err := client.Post("/vps/domains/remove", map[string]interface{}{
		"domain": domain,
	})
	if err != nil {
		return fmt.Errorf("failed to remove domain: %w", err)
	}

	var result struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}

	if err := json.Unmarshal(resp, &result); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	if !result.Success {
		return errors.New(result.Message)
	}

	printSuccess("Domain removed successfully")
	return nil
}

func printDomainHelp() {
	fmt.Printf(`%sdalang domain%s - Manage custom domains

%sUSAGE:%s
    dalang domain enable <vps-name>           Enable custom domain (+10K/month)
    dalang domain list <vps-name>             List custom domains
    dalang domain add <vps-name> <domain>     Add a custom domain
    dalang domain verify <domain>             Verify domain DNS configuration
    dalang domain remove <domain>             Remove a custom domain

%sDESCRIPTION:%s
    Manage custom domains for your VPS. First enable the custom domain
    addon, then add and verify your domains.

%sEXAMPLES:%s
    dalang domain enable MyVM                 # Enable custom domain feature
    dalang domain add MyVM example.com        # Add a domain
    dalang domain verify example.com          # Verify DNS is configured
    dalang domain list MyVM                   # List all domains
    dalang domain remove example.com          # Remove a domain

%sWORKFLOW:%s
    1. Enable custom domain: dalang domain enable <vps>
    2. Pay the invoice
    3. Add your domain: dalang domain add <vps> <domain>
    4. Configure DNS records as shown
    5. Verify: dalang domain verify <domain>
`,
		colorCyan, colorReset,
		colorYellow, colorReset,
		colorYellow, colorReset,
		colorYellow, colorReset,
		colorYellow, colorReset,
	)
}
