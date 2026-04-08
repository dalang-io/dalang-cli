package cmd

import (
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	"github.com/dalang-io/dalang-cli/internal/api"
	"github.com/dalang-io/dalang-cli/internal/config"
)

func cmdAuth(args []string) error {
	if len(args) == 0 {
		return doAuth()
	}

	switch args[0] {
	case "status":
		return authStatus()
	case "logout":
		return authLogout()
	case "--help", "-h":
		printAuthHelp()
		return nil
	default:
		printError("Unknown auth subcommand: %s", args[0])
		return fmt.Errorf("unknown subcommand: %s", args[0])
	}
}

func doAuth() error {
	client, err := api.NewClient()
	if err != nil {
		return err
	}
	client.Verbose = VerboseOutput

	printInfo("Initiating authentication...")

	// Call CLI auth init endpoint
	resp, err := client.Post("/cli/auth/init", nil)
	if err != nil {
		return fmt.Errorf("failed to initiate auth: %w", err)
	}

	var initResp api.CLIAuthInitResponse
	if err := json.Unmarshal(resp, &initResp); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	if !initResp.Success {
		return fmt.Errorf("auth init failed")
	}

	// Display instructions
	fmt.Println()
	fmt.Printf("  %sVisit:%s %s%s%s\n", colorBold, colorReset, colorCyan, initResp.Data.VerificationURL, colorReset)
	fmt.Printf("  %sEnter code:%s %s%s%s\n", colorBold, colorReset, colorYellow+colorBold, initResp.Data.UserCode, colorReset)
	fmt.Println()

	deviceCode := initResp.Data.DeviceCode
	pollIntervalSeconds := initResp.Data.Interval
	if pollIntervalSeconds <= 0 {
		pollIntervalSeconds = 15
	}
	pollInterval := time.Duration(pollIntervalSeconds) * time.Second

	maxRetries := initResp.Data.ExpiresIn / pollIntervalSeconds
	if initResp.Data.ExpiresIn%pollIntervalSeconds != 0 {
		maxRetries++
	}
	if maxRetries < 1 {
		maxRetries = 1
	}

	for retry := 1; retry <= maxRetries; retry++ {
		fmt.Printf("%s→%s Attempt %d/%d: Waiting for authorization", colorBlue, colorReset, retry, maxRetries)

		// Countdown timer
		for remaining := int(pollInterval.Seconds()); remaining > 0; remaining-- {
			fmt.Printf("\r%s→%s Attempt %d/%d: Checking in %2ds...  ", colorBlue, colorReset, retry, maxRetries, remaining)
			time.Sleep(1 * time.Second)
		}

		fmt.Printf("\r%s→%s Attempt %d/%d: Polling...            ", colorBlue, colorReset, retry, maxRetries)

		pollResp, err := client.Get(fmt.Sprintf("/cli/auth/poll?device_code=%s", url.QueryEscape(deviceCode)))
		if err != nil {
			fmt.Printf("\r%s✗%s Attempt %d/%d: Network error        \n", colorRed, colorReset, retry, maxRetries)
			continue
		}

		var authResp api.CLIAuthPollResponse
		if err := json.Unmarshal(pollResp, &authResp); err != nil {
			fmt.Printf("\r%s✗%s Attempt %d/%d: Parse error          \n", colorRed, colorReset, retry, maxRetries)
			continue
		}

		// Check both Error and Message fields (backend uses Message)
		errMsg := authResp.Error
		if errMsg == "" {
			errMsg = authResp.Message
		}

		if errMsg == "authorization_pending" {
			fmt.Printf("\r%s…%s Attempt %d/%d: Not yet authorized   \n", colorYellow, colorReset, retry, maxRetries)
			continue
		}

		if errMsg == "expired_token" {
			fmt.Printf("\r%s✗%s Attempt %d/%d: Code expired         \n", colorRed, colorReset, retry, maxRetries)
			return fmt.Errorf("authorization expired. Please try again")
		}

		if errMsg == "access_denied" {
			fmt.Printf("\r%s✗%s Attempt %d/%d: Access denied        \n", colorRed, colorReset, retry, maxRetries)
			return fmt.Errorf("authorization denied")
		}

		if authResp.Success && authResp.Data.AccessToken != "" {
			fmt.Printf("\r%s✓%s Attempt %d/%d: Authorized!          \n", colorGreen, colorReset, retry, maxRetries)

			// Save credentials
			creds := &config.Credentials{
				AccessToken:  authResp.Data.AccessToken,
				RefreshToken: authResp.Data.RefreshToken,
				Email:        authResp.Data.Email,
				ExpiresAt:    time.Now().Add(time.Duration(authResp.Data.ExpiresIn) * time.Second).Unix(),
			}

			if err := config.SaveCredentials(creds); err != nil {
				return fmt.Errorf("failed to save credentials: %w", err)
			}

			fmt.Println()
			printSuccess("Successfully authenticated as %s%s%s", colorCyan, authResp.Data.Email, colorReset)
			return nil
		}

		// Unknown response
		fmt.Printf("\r%s?%s Attempt %d/%d: Unexpected response  \n", colorYellow, colorReset, retry, maxRetries)
	}

	return fmt.Errorf("authorization failed after %d attempts. Please try again", maxRetries)
}

func authStatus() error {
	creds, err := config.LoadCredentials()
	if err != nil || creds.AccessToken == "" {
		if jsonOutput {
			output := map[string]interface{}{
				"authenticated": false,
			}
			data, _ := json.MarshalIndent(output, "", "  ")
			fmt.Println(string(data))
			return nil
		}
		printWarn("Not authenticated. Run 'dalang auth' to login.")
		return nil
	}

	// Verify token by calling /auth/me
	client, err := api.NewAuthenticatedClient()
	if err != nil {
		if jsonOutput {
			output := map[string]interface{}{
				"authenticated": false,
				"error":         err.Error(),
			}
			data, _ := json.MarshalIndent(output, "", "  ")
			fmt.Println(string(data))
			return nil
		}
		printWarn("Credentials found but may be invalid: %v", err)
		return nil
	}

	resp, err := client.Get("/auth/me")
	if err != nil {
		if jsonOutput {
			output := map[string]interface{}{
				"authenticated": false,
				"error":         "Token expired or invalid",
			}
			data, _ := json.MarshalIndent(output, "", "  ")
			fmt.Println(string(data))
			return nil
		}
		printWarn("Token expired or invalid. Run 'dalang auth' to re-login.")
		return nil
	}

	var meResp api.MeResponse
	if err := json.Unmarshal(resp, &meResp); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	if jsonOutput {
		output := map[string]interface{}{
			"authenticated": true,
			"email":         meResp.Data.Email,
			"role":          meResp.Data.Role,
		}
		data, _ := json.MarshalIndent(output, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	printSuccess("Authenticated as %s%s%s (role: %s)", colorCyan, meResp.Data.Email, colorReset, meResp.Data.Role)
	return nil
}

func authLogout() error {
	if err := config.DeleteCredentials(); err != nil {
		return fmt.Errorf("failed to remove credentials: %w", err)
	}

	printSuccess("Logged out successfully")
	return nil
}

func printAuthHelp() {
	fmt.Printf(`%sdalang auth%s - Authenticate with Dalang

%sUSAGE:%s
    dalang auth              Start authentication flow
    dalang auth status       Check current auth status
    dalang auth logout       Clear stored credentials

%sDESCRIPTION:%s
    Authenticates the CLI with your Dalang account using Device
    Authorization Grant flow. You'll be given a URL and code to
    enter in your browser.

    Credentials are stored in ~/.dalang/credentials

%sEXAMPLES:%s
    dalang auth              # Start authentication
    dalang auth status       # Check if logged in
    dalang auth logout       # Log out
`,
		colorCyan, colorReset,
		colorYellow, colorReset,
		colorYellow, colorReset,
		colorYellow, colorReset,
	)
}
