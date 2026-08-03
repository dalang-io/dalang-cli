package cmd

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/dalang-io/dalang-cli/internal/api"
)

func cmdCredit(args []string) error {
	if len(args) == 0 {
		return creditBalance()
	}

	switch args[0] {
	case "history":
		return creditHistory()
	case "add":
		if len(args) < 2 {
			printUsage("Usage: dalang credit add <amount>")
			return fmt.Errorf("missing amount")
		}
		return creditAdd(args[1])
	case "--help", "-h":
		printCreditHelp()
		return nil
	default:
		// Check if it's a number (shorthand for add)
		if _, err := strconv.Atoi(args[0]); err == nil {
			return creditAdd(args[0])
		}
		return fmt.Errorf("unknown credit subcommand: %s", args[0])
	}
}

func creditBalance() error {
	client, err := api.NewAuthenticatedClient()
	if err != nil {
		return err
	}
	client.Verbose = VerboseOutput

	resp, err := client.Get("/credits/balance")
	if err != nil {
		return fmt.Errorf("failed to fetch balance: %w", err)
	}

	var balanceResp api.BalanceResponse
	if err := json.Unmarshal(resp, &balanceResp); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	if jsonOutput {
		data, _ := json.MarshalIndent(balanceResp.Data, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	fmt.Printf("\n%sCredit Balance%s\n", colorBold, colorReset)
	fmt.Println(strings.Repeat("─", 40))
	fmt.Printf("  Balance:     %s%s%s\n", colorGreen+colorBold, formatIDR(balanceResp.Data.Balance), colorReset)
	fmt.Printf("  Total Topup: %s\n", formatIDR(balanceResp.Data.TotalTopup))
	fmt.Printf("  Total Spent: %s\n", formatIDR(balanceResp.Data.TotalSpent))
	if balanceResp.Data.TotalCommission > 0 {
		fmt.Printf("  Commission:  %s%s%s\n", colorCyan, formatIDR(balanceResp.Data.TotalCommission), colorReset)
	}
	fmt.Println()

	return nil
}

func creditHistory() error {
	client, err := api.NewAuthenticatedClient()
	if err != nil {
		return err
	}
	client.Verbose = VerboseOutput

	resp, err := client.Get("/credits/transactions?page=1&limit=25")
	if err != nil {
		return fmt.Errorf("failed to fetch transactions: %w", err)
	}

	var txResp api.TransactionsResponse
	if err := json.Unmarshal(resp, &txResp); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	if jsonOutput {
		data, _ := json.MarshalIndent(txResp.Data, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	if len(txResp.Data.Transactions) == 0 {
		printInfo("No transactions found")
		return nil
	}

	fmt.Printf("\n%sRecent Transactions%s (showing %d of %d)\n", colorBold, colorReset, len(txResp.Data.Transactions), txResp.Data.Total)
	fmt.Println(strings.Repeat("─", 70))
	fmt.Printf("%-12s %-10s %15s %15s  %s\n", "DATE", "TYPE", "AMOUNT", "BALANCE", "DESCRIPTION")
	fmt.Println(strings.Repeat("─", 70))

	for _, tx := range txResp.Data.Transactions {
		date := formatDate(tx.CreatedAt)
		typeColor := colorReset
		amountStr := formatIDR(tx.Amount)

		switch tx.Type {
		case "topup":
			typeColor = colorGreen
			amountStr = "+" + amountStr
		case "spend":
			typeColor = colorRed
			amountStr = "-" + amountStr
		case "commission":
			typeColor = colorCyan
			amountStr = "+" + amountStr
		case "refund":
			typeColor = colorYellow
			amountStr = "+" + amountStr
		}

		desc := tx.Description
		if len(desc) > 25 {
			desc = desc[:22] + "..."
		}

		fmt.Printf("%-12s %s%-10s%s %15s %15s  %s\n",
			date,
			typeColor, tx.Type, colorReset,
			amountStr,
			formatIDR(tx.BalanceAfter),
			desc,
		)
	}
	fmt.Println()

	return nil
}

func creditAdd(amountStr string) error {
	amount, err := strconv.Atoi(amountStr)
	if err != nil {
		return fmt.Errorf("invalid amount: %s", amountStr)
	}

	// Convert to actual amount (input is in thousands)
	actualAmount := amount * 1000

	if actualAmount < 50000 {
		return fmt.Errorf("minimum topup is 50K IDR (enter 50 or higher)")
	}

	client, err := api.NewAuthenticatedClient()
	if err != nil {
		return err
	}

	printInfo("Creating topup for %s...", formatIDR(int64(actualAmount)))

	resp, err := client.Post("/credits/topup", map[string]interface{}{
		"amount": actualAmount,
	})
	if err != nil {
		return fmt.Errorf("failed to create topup: %w", err)
	}

	var topupResp api.TopupResponse
	if err := json.Unmarshal(resp, &topupResp); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	if jsonOutput {
		data, _ := json.MarshalIndent(topupResp.Data, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	fmt.Println()
	printSuccess("Topup invoice created!")
	fmt.Printf("\n  %sAmount:%s %s\n", colorBold, colorReset, formatIDR(int64(topupResp.Data.Amount)))
	fmt.Printf("  %sPayment URL:%s\n", colorBold, colorReset)
	fmt.Printf("  %s%s%s\n\n", colorCyan, topupResp.Data.InvoiceURL, colorReset)
	printInfo("Open the URL above to complete payment")

	return nil
}

func formatIDR(amount int64) string {
	prefix := "Rp "
	if amount < 0 {
		prefix = "Rp -"
		amount = -amount
	}

	// Format with thousand separators
	str := fmt.Sprintf("%d", amount)
	n := len(str)
	if n <= 3 {
		return prefix + str
	}

	var result strings.Builder
	result.WriteString(prefix)

	remainder := n % 3
	if remainder > 0 {
		result.WriteString(str[:remainder])
		if n > remainder {
			result.WriteString(".")
		}
	}

	for i := remainder; i < n; i += 3 {
		result.WriteString(str[i : i+3])
		if i+3 < n {
			result.WriteString(".")
		}
	}

	return result.String()
}

func formatDate(isoDate string) string {
	if len(isoDate) >= 10 {
		return isoDate[:10]
	}
	return isoDate
}

func formatExpiryWithDays(isoDate string) string {
	if isoDate == "" {
		return "N/A"
	}

	// Parse the date
	t, err := time.Parse("2006-01-02T15:04:05Z", isoDate)
	if err != nil {
		// Try alternative format
		t, err = time.Parse("2006-01-02 15:04:05", isoDate)
		if err != nil {
			t, err = time.Parse("2006-01-02", isoDate)
			if err != nil {
				return formatDate(isoDate)
			}
		}
	}

	now := time.Now()
	days := int(t.Sub(now).Hours() / 24)

	dateStr := t.Format("2006-01-02")

	if days < 0 {
		return fmt.Sprintf("%s (%sexpired%s)", dateStr, colorRed, colorReset)
	} else if days == 0 {
		return fmt.Sprintf("%s (%stoday%s)", dateStr, colorYellow, colorReset)
	} else if days == 1 {
		return fmt.Sprintf("%s (%s1 day left%s)", dateStr, colorYellow, colorReset)
	} else if days <= 7 {
		return fmt.Sprintf("%s (%s%d days left%s)", dateStr, colorYellow, days, colorReset)
	} else {
		return fmt.Sprintf("%s (%d days left)", dateStr, days)
	}
}

func printCreditHelp() {
	fmt.Printf(`%sdalang credit%s - Manage credits/wallet

%sUSAGE:%s
    dalang credit            Show current balance
    dalang credit history    Show recent transactions
    dalang credit add <N>    Top up N thousand IDR (e.g., 50 = 50K)

%sDESCRIPTION:%s
    Manage your Dalang credits balance. Credits can be used to pay
    for VPS, containers, and app hosting services.

%sEXAMPLES:%s
    dalang credit            # Check balance
    dalang credit history    # View last 25 transactions
    dalang credit add 100    # Top up 100K IDR
    dalang credit add 500    # Top up 500K IDR

%sNOTE:%s
    Minimum topup is 50K IDR. Credits expire 12 months after topup.
`,
		colorCyan, colorReset,
		colorYellow, colorReset,
		colorYellow, colorReset,
		colorYellow, colorReset,
		colorYellow, colorReset,
	)
}
