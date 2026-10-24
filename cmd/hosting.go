package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/dalang-io/dalang-cli/internal/api"
)

const defaultHostingURL = "https://hosting.dalang.io"

type hostingAccount struct {
	ID        string  `json:"id"`
	Domain    string  `json:"domain"`
	Plan      string  `json:"plan"`
	Status    string  `json:"status"`
	ExpiresAt *string `json:"expires_at"`
}

type hostingAccountsResponse struct {
	UserID   string           `json:"user_id"`
	Accounts []hostingAccount `json:"accounts"`
}

func hostingURL() string {
	if v := os.Getenv("DALANG_HOSTING_URL"); v != "" {
		return v
	}
	return defaultHostingURL
}

func cmdHosting(args []string) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		printHostingHelp()
		return nil
	}
	switch args[0] {
	case "accounts", "account":
		return hostingAccounts()
	default:
		return fmt.Errorf("unknown hosting command %q (run 'dalang hosting help')", args[0])
	}
}

func hostingAccounts() error {
	c, err := api.NewAuthenticatedClient()
	if err != nil {
		return err
	}
	c.BaseURL = hostingURL()
	body, err := c.Get("/api/v1/hosting/accounts")
	if err != nil {
		return err
	}
	var out hostingAccountsResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return fmt.Errorf("respons hosting tidak sah: %w", err)
	}
	if jsonOutput {
		data, _ := json.MarshalIndent(out, "", "  ")
		fmt.Println(string(data))
		return nil
	}
	if len(out.Accounts) == 0 {
		printInfo("Belum ada akun hosting.")
		return nil
	}
	fmt.Printf("\nHosting accounts (%d)\n", len(out.Accounts))
	fmt.Println("──────────────────────────────────────────────────────────")
	fmt.Printf("%-24s %-10s %-12s %s\n", "DOMAIN", "PLAN", "STATUS", "EXPIRES")
	for _, a := range out.Accounts {
		expires := "-"
		if a.ExpiresAt != nil && *a.ExpiresAt != "" {
			expires = *a.ExpiresAt
		}
		domain := a.Domain
		if domain == "" {
			domain = a.ID
		}
		fmt.Printf("%-24s %-10s %-12s %s\n", domain, a.Plan, a.Status, expires)
	}
	return nil
}

func printHostingHelp() {
	fmt.Print(`Dalang Hosting

USAGE:
    dalang hosting accounts

Read-only commands against hosting.dalang.io. Authentication uses the same
OAuth token as 'dalang auth' (no separate API token).

ENVIRONMENT:
    DALANG_HOSTING_URL   override base URL (default https://hosting.dalang.io)
`)
}
