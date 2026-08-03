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
	"github.com/dalang-io/dalang-cli/internal/netdial"
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
				// Falls back to public DNS when the system has no resolver config
				// (Android/Termux has no /etc/resolv.conf) — see internal/netdial.
				DialContext: netdial.DialContext,
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

// tokenRefreshThreshold is how close to expiry an access token must be before
// NewAuthenticatedClient proactively renews it.
const tokenRefreshThreshold = 24 * time.Hour

// NewAuthenticatedClient creates a client and ensures authentication
func NewAuthenticatedClient() (*Client, error) {
	client, err := NewClient()
	if err != nil {
		return nil, err
	}

	if client.Token == "" {
		return nil, fmt.Errorf("not authenticated. Run 'dalang auth' to login")
	}

	client.maybeRefreshToken()

	return client, nil
}

// maybeRefreshToken proactively renews the access token when it is still valid
// but close to expiry, so active users rarely get bounced to a full re-login.
//
// The /cli/auth/refresh endpoint requires a still-valid token (it just re-issues
// a fresh one for the same user), so this only acts before expiry — an already
// expired token is left to the normal 401 path. Entirely best-effort: any
// failure is ignored and the existing token continues to be used.
func (c *Client) maybeRefreshToken() {
	creds, err := config.LoadCredentials()
	if err != nil || creds.ExpiresAt == 0 {
		return // unknown expiry (older credentials) — nothing to do
	}

	expiry := time.Unix(creds.ExpiresAt, 0)
	now := time.Now()
	if !now.Before(expiry) {
		return // already expired; a refresh would be rejected
	}
	if expiry.Sub(now) > tokenRefreshThreshold {
		return // not near expiry yet
	}

	resp, err := c.Post("/cli/auth/refresh", nil)
	if err != nil {
		return
	}

	var r CLIAuthPollResponse
	if err := json.Unmarshal(resp, &r); err != nil || !r.Success || r.Data.AccessToken == "" {
		return
	}

	email := r.Data.Email
	if email == "" {
		email = creds.Email
	}
	newCreds := &config.Credentials{
		AccessToken:  r.Data.AccessToken,
		RefreshToken: creds.RefreshToken,
		Email:        email,
		ExpiresAt:    time.Now().Add(time.Duration(r.Data.ExpiresIn) * time.Second).Unix(),
	}
	if err := config.SaveCredentials(newCreds); err == nil {
		c.Token = newCreds.AccessToken
		if c.Verbose {
			fmt.Fprintln(os.Stderr, "[DEBUG] Access token proactively refreshed")
		}
	}
}

// buildURL constructs a full URL from a path
func (c *Client) buildURL(path string) string {
	u := c.BaseURL
	if !strings.HasSuffix(u, "/") && !strings.HasPrefix(path, "/") {
		u += "/"
	} else if strings.HasSuffix(u, "/") && strings.HasPrefix(path, "/") {
		path = path[1:]
	}
	return u + path
}

// Request makes an HTTP request to the API using the client's default HTTP client.
func (c *Client) Request(method, path string, body interface{}) ([]byte, error) {
	return c.doRequest(c.HTTPClient, method, path, body)
}

// PostWithTimeout makes a POST request using a one-off HTTP client with the
// given timeout, for endpoints whose work may exceed the default 30s (e.g.
// remote command execution). It shares the default client's transport.
func (c *Client) PostWithTimeout(path string, body interface{}, timeout time.Duration) ([]byte, error) {
	hc := &http.Client{Timeout: timeout, Transport: c.HTTPClient.Transport}
	return c.doRequest(hc, "POST", path, body)
}

// doRequest performs an HTTP request against the API using the supplied client.
func (c *Client) doRequest(httpClient *http.Client, method, path string, body interface{}) ([]byte, error) {
	// Don't use url.JoinPath as it encodes query strings incorrectly
	u := c.buildURL(path)

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
			masked := c.Token
			if len(masked) > 4 {
				masked = "***" + masked[len(masked)-4:]
			}
			fmt.Fprintf(os.Stderr, "[DEBUG] Auth: Bearer %s\n", masked)
		}
	}

	resp, err := httpClient.Do(req)
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

// UploadMultipart makes a multipart POST request with streaming body.
// The caller provides a pre-built multipart body and content type.
// Uses a 15-minute timeout for large file transfers.
func (c *Client) UploadMultipart(path, contentType string, body io.Reader) (*http.Response, error) {
	u := c.buildURL(path)

	if c.Verbose {
		fmt.Fprintf(os.Stderr, "[DEBUG] POST (multipart) %s\n", u)
	}

	req, err := http.NewRequest("POST", u, body)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", contentType)
	req.Header.Set("User-Agent", "dalang-cli/1.0")
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}

	// Use a long-timeout client sharing the same transport
	longClient := &http.Client{
		Timeout:   15 * time.Minute,
		Transport: c.HTTPClient.Transport,
	}

	return longClient.Do(req)
}

// StreamGet makes a GET request and returns the raw response for streaming.
// The caller is responsible for closing resp.Body.
// Uses a 15-minute timeout for large file transfers.
func (c *Client) StreamGet(path string) (*http.Response, error) {
	u := c.buildURL(path)

	if c.Verbose {
		fmt.Fprintf(os.Stderr, "[DEBUG] GET (stream) %s\n", u)
	}

	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", "dalang-cli/1.0")
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}

	longClient := &http.Client{
		Timeout:   15 * time.Minute,
		Transport: c.HTTPClient.Transport,
	}

	resp, err := longClient.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode == 401 {
		resp.Body.Close()
		return nil, &APIError{
			StatusCode: 401,
			Message:    "Authentication expired. Run 'dalang auth' to re-login",
		}
	}

	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		respBody, _ := io.ReadAll(resp.Body)
		var errResp struct {
			Error   string `json:"error"`
			Message string `json:"message"`
		}
		if json.Unmarshal(respBody, &errResp) == nil {
			msg := errResp.Error
			if msg == "" {
				msg = errResp.Message
			}
			if msg == "" {
				msg = string(respBody)
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

	return resp, nil
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
	Node                string `json:"node"`
	PublicIP            string `json:"ipv4_public"`
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
