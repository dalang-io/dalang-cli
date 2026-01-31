package cmd

import (
	"encoding/json"
	"fmt"
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
	printInfo("Waiting for authorization...")

	// Poll for authorization
	interval := time.Duration(initResp.Data.Interval) * time.Second
	if interval < 5*time.Second {
		interval = 5 * time.Second
	}

	expiresAt := time.Now().Add(time.Duration(initResp.Data.ExpiresIn) * time.Second)
	deviceCode := initResp.Data.DeviceCode

	for time.Now().Before(expiresAt) {
		time.Sleep(interval)

		pollResp, err := client.Get(fmt.Sprintf("/cli/auth/poll?device_code=%s", deviceCode))
		if err != nil {
			continue // Keep polling on errors
		}

		var authResp api.CLIAuthPollResponse
		if err := json.Unmarshal(pollResp, &authResp); err != nil {
			continue
		}

		if authResp.Error == "authorization_pending" {
			continue
		}

		if authResp.Error == "expired_token" {
			return fmt.Errorf("authorization expired. Please try again")
		}

		if authResp.Error == "access_denied" {
			return fmt.Errorf("authorization denied")
		}

		if authResp.Success && authResp.Data.AccessToken != "" {
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
	}

	return fmt.Errorf("authorization timed out. Please try again")
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
