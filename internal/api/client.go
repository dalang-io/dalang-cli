package api

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/dalang-io/dalang-cli/internal/config"
)

// Client is the API client
type Client struct {
	BaseURL    string
	Token      string
	HTTPClient *http.Client
	Verbose    bool
}

// APIError represents an error response from the API
type APIError struct {
	StatusCode int
	Message    string
	Body       string
}

func (e *APIError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return fmt.Sprintf("API error: %d", e.StatusCode)
}

// NewClient creates a new API client
func NewClient() (*Client, error) {
	baseURL := config.GetAPIURL()

	client := &Client{
		BaseURL: baseURL,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					MinVersion: tls.VersionTLS12,
				},
			},
		},
	}

	// Try to load token from credentials
	creds, err := config.LoadCredentials()
	if err == nil && creds.AccessToken != "" {
		client.Token = creds.AccessToken
	}

	return client, nil
}

// NewAuthenticatedClient creates a client and ensures authentication
func NewAuthenticatedClient() (*Client, error) {
	client, err := NewClient()
	if err != nil {
		return nil, err
	}

	if client.Token == "" {
		return nil, fmt.Errorf("not authenticated. Run 'dalang auth' to login")
	}

	return client, nil
}

// Request makes an HTTP request to the API
func (c *Client) Request(method, path string, body interface{}) ([]byte, error) {
	// Don't use url.JoinPath as it encodes query strings incorrectly
	u := c.BaseURL
	if !strings.HasSuffix(u, "/") && !strings.HasPrefix(path, "/") {
		u += "/"
	} else if strings.HasSuffix(u, "/") && strings.HasPrefix(path, "/") {
		path = path[1:]
	}
	u += path

	if c.Verbose {
		fmt.Fprintf(os.Stderr, "[DEBUG] %s %s\n", method, u)
	}

	var bodyReader io.Reader
	var jsonBody []byte
	if body != nil {
		var err error
		jsonBody, err = json.Marshal(body)
		if err != nil {
			return nil, err
		}
		bodyReader = bytes.NewReader(jsonBody)
		if c.Verbose {
			fmt.Fprintf(os.Stderr, "[DEBUG] Request body: %s\n", string(jsonBody))
		}
	}

	req, err := http.NewRequest(method, u, bodyReader)
	if err != nil {
		if c.Verbose {
			fmt.Fprintf(os.Stderr, "[DEBUG] Request create error: %v\n", err)
		}
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "dalang-cli/1.0")

	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
		if c.Verbose {
			tokenPreview := c.Token
			if len(tokenPreview) > 20 {
				tokenPreview = tokenPreview[:20] + "..."
			}
			fmt.Fprintf(os.Stderr, "[DEBUG] Auth: Bearer %s\n", tokenPreview)
		}
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		if c.Verbose {
			fmt.Fprintf(os.Stderr, "[DEBUG] HTTP error: %v\n", err)
		}
		return nil, err
	}
	defer resp.Body.Close()

	if c.Verbose {
		fmt.Fprintf(os.Stderr, "[DEBUG] Response status: %d %s\n", resp.StatusCode, resp.Status)
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		if c.Verbose {
			fmt.Fprintf(os.Stderr, "[DEBUG] Read body error: %v\n", err)
		}
		return nil, err
	}

	if c.Verbose {
		fmt.Fprintf(os.Stderr, "[DEBUG] Response body: %s\n", string(respBody))
	}

	if resp.StatusCode == 401 {
		return nil, &APIError{
			StatusCode: 401,
			Message:    "Authentication expired. Run 'dalang auth' to re-login",
		}
	}

	if resp.StatusCode == 429 {
		return nil, &APIError{
			StatusCode: 429,
			Message:    "Rate limited. Please wait and try again",
		}
	}

	if resp.StatusCode >= 400 {
		// Try to parse error message from response
		var errResp struct {
			Error   string `json:"error"`
			Message string `json:"message"`
		}
		if json.Unmarshal(respBody, &errResp) == nil {
			msg := errResp.Error
			if msg == "" {
				msg = errResp.Message
			}
			return nil, &APIError{
				StatusCode: resp.StatusCode,
				Message:    msg,
				Body:       string(respBody),
			}
		}
		return nil, &APIError{
			StatusCode: resp.StatusCode,
			Body:       string(respBody),
		}
	}

	return respBody, nil
}

// Get makes a GET request
func (c *Client) Get(path string) ([]byte, error) {
	return c.Request("GET", path, nil)
}

// Post makes a POST request
func (c *Client) Post(path string, body interface{}) ([]byte, error) {
	return c.Request("POST", path, body)
}

// Put makes a PUT request
func (c *Client) Put(path string, body interface{}) ([]byte, error) {
	return c.Request("PUT", path, body)
}

// Delete makes a DELETE request
func (c *Client) Delete(path string) ([]byte, error) {
	return c.Request("DELETE", path, nil)
}

// Response types

// BalanceResponse represents the credits balance response
type BalanceResponse struct {
	Success bool `json:"success"`
	Data    struct {
		Balance         int64 `json:"balance"`
		TotalTopup      int64 `json:"total_topup"`
		TotalSpent      int64 `json:"total_spent"`
		TotalCommission int64 `json:"total_commission"`
	} `json:"data"`
}

// TransactionsResponse represents credit transactions
type TransactionsResponse struct {
	Success bool `json:"success"`
	Data    struct {
		Transactions []Transaction `json:"transactions"`
		Total        int           `json:"total"`
		Page         int           `json:"page"`
		Limit        int           `json:"limit"`
	} `json:"data"`
}

// Transaction represents a single transaction
type Transaction struct {
	ID           int    `json:"id"`
	Type         string `json:"type"`
	Amount       int64  `json:"amount"`
	BalanceAfter int64  `json:"balance_after"`
	Description  string `json:"description"`
	Reference    string `json:"reference"`
	CreatedAt    string `json:"created_at"`
}

// VPSListResponse represents the VPS list response
type VPSListResponse struct {
	Success bool  `json:"success"`
	Data    []VPS `json:"data"`
}

// VPS represents a VPS instance
type VPS struct {
	ID                  string `json:"id"`
	Name                string `json:"name"`
	DisplayName         string `json:"display_name"`
	Status              string `json:"status"`
	VCPU                int    `json:"vcpu"`
	RAM                 int    `json:"ram"`
	Storage             int    `json:"storage"`
	StorageType         string `json:"storage_type"`
	Bandwidth           int    `json:"bandwidth"`
	Price               int    `json:"price"`
	ExpiresAt           string `json:"expired_at"`
	Region              string `json:"region"`
	Image               string `json:"image"`
	Node                string `json:"node"`
	PublicIP            string `json:"public_ip"`
	LocalIP             string `json:"ipv4_local"`
	Domain              string `json:"domain"`
	CustomDomainEnabled int    `json:"custom_domain_enabled"`
	CustomDomainPrice   int    `json:"custom_domain_price"`
}

// ContainerListResponse represents container list
type ContainerListResponse struct {
	Success bool `json:"success"`
	Data    struct {
		Containers []Container `json:"containers"`
	} `json:"data"`
}

// Container represents a container service
type Container struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	ContainerType string `json:"container_type"`
	Status        string `json:"status"`
	Plan          string `json:"plan"`
	ExpiresAt     string `json:"expires_at"`
}

// DeploymentListResponse represents app deployments
type DeploymentListResponse struct {
	Success bool         `json:"success"`
	Data    []Deployment `json:"data"`
}

// Deployment represents an app deployment
type Deployment struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Status    string `json:"status"`
	RepoURL   string `json:"repo_url"`
	Branch    string `json:"branch"`
	URL       string `json:"url"`
	ExpiresAt string `json:"expires_at"`
}

// TopupResponse represents topup creation response
type TopupResponse struct {
	Success bool `json:"success"`
	Data    struct {
		InvoiceURL string `json:"invoice_url"`
		InvoiceID  string `json:"invoice_id"`
		Amount     int    `json:"amount"`
	} `json:"data"`
	Message string `json:"message"`
}

// CLIAuthInitResponse represents CLI auth init response
type CLIAuthInitResponse struct {
	Success bool `json:"success"`
	Data    struct {
		DeviceCode      string `json:"device_code"`
		UserCode        string `json:"user_code"`
		VerificationURL string `json:"verification_url"`
		ExpiresIn       int    `json:"expires_in"`
		Interval        int    `json:"interval"`
	} `json:"data"`
}

// CLIAuthPollResponse represents CLI auth poll response
type CLIAuthPollResponse struct {
	Success bool `json:"success"`
	Data    struct {
		AccessToken  string `json:"access_token,omitempty"`
		RefreshToken string `json:"refresh_token,omitempty"`
		Email        string `json:"email,omitempty"`
		ExpiresIn    int    `json:"expires_in,omitempty"`
	} `json:"data"`
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
}

// MeResponse represents /auth/me response
type MeResponse struct {
	Success bool `json:"success"`
	Data    struct {
		ID    int    `json:"id"`
		Email string `json:"email"`
		Role  string `json:"role"`
	} `json:"data"`
}
