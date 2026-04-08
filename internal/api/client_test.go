package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func testClient(t *testing.T, fn roundTripFunc) *Client {
	t.Helper()
	return &Client{
		BaseURL: "https://api.dalang.io",
		HTTPClient: &http.Client{
			Transport: fn,
		},
	}
}

func TestAPIErrorError(t *testing.T) {
	if got := (&APIError{Message: "boom"}).Error(); got != "boom" {
		t.Fatalf("unexpected error string: %q", got)
	}

	if got := (&APIError{StatusCode: 503}).Error(); got != "API error: 503" {
		t.Fatalf("unexpected fallback error string: %q", got)
	}
}

func TestBuildURL(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		path    string
		want    string
	}{
		{name: "adds slash", baseURL: "https://api.dalang.io", path: "vps/list", want: "https://api.dalang.io/vps/list"},
		{name: "keeps single slash", baseURL: "https://api.dalang.io/", path: "/vps/list", want: "https://api.dalang.io/vps/list"},
		{name: "query string", baseURL: "https://api.dalang.io", path: "/vps/list?status=running", want: "https://api.dalang.io/vps/list?status=running"},
		{name: "both have slash", baseURL: "https://api.dalang.io/", path: "/test", want: "https://api.dalang.io/test"},
		{name: "neither has slash", baseURL: "https://api.dalang.io", path: "test", want: "https://api.dalang.io/test"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Client{BaseURL: tt.baseURL}
			if got := c.buildURL(tt.path); got != tt.want {
				t.Fatalf("buildURL(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestRequestSuccessIncludesAuthorizationAndJSONBody(t *testing.T) {
	type payload struct {
		Name string `json:"name"`
	}

	sawAuth := ""
	sawContentType := ""
	sawURL := ""
	sawBody := payload{}

	c := testClient(t, func(r *http.Request) (*http.Response, error) {
		sawAuth = r.Header.Get("Authorization")
		sawContentType = r.Header.Get("Content-Type")
		sawURL = r.URL.String()
		if err := json.NewDecoder(r.Body).Decode(&sawBody); err != nil {
			t.Fatalf("decode request body failed: %v", err)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"ok":true}`)),
		}, nil
	})
	c.Token = "secret-token"

	resp, err := c.Request(http.MethodPost, "/vps/order", payload{Name: "vm-1"})
	if err != nil {
		t.Fatalf("Request returned error: %v", err)
	}

	if string(resp) != `{"ok":true}` {
		t.Fatalf("unexpected response body: %s", string(resp))
	}
	if sawAuth != "Bearer secret-token" {
		t.Fatalf("unexpected auth header: %q", sawAuth)
	}
	if sawContentType != "application/json" {
		t.Fatalf("unexpected content type: %q", sawContentType)
	}
	if sawURL != "https://api.dalang.io/vps/order" {
		t.Fatalf("unexpected request url: %q", sawURL)
	}
	if sawBody.Name != "vm-1" {
		t.Fatalf("unexpected body payload: %+v", sawBody)
	}
}

func TestRequestReturnsSpecialErrors(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string
		wantErr    string
	}{
		{name: "auth expired", statusCode: 401, body: `{"error":"nope"}`, wantErr: "Authentication expired. Run 'dalang auth' to re-login"},
		{name: "rate limited", statusCode: 429, body: `{"error":"slow down"}`, wantErr: "Rate limited. Please wait and try again"},
		{name: "json message", statusCode: 400, body: `{"message":"bad request"}`, wantErr: "bad request"},
		{name: "json error", statusCode: 500, body: `{"error":"server exploded"}`, wantErr: "server exploded"},
		{name: "plain body", statusCode: 502, body: `upstream failed`, wantErr: "API error: 502"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := testClient(t, func(r *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: tt.statusCode,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(tt.body)),
				}, nil
			})

			_, err := c.Request(http.MethodGet, "/test", nil)
			if err == nil {
				t.Fatal("expected error")
			}

			var apiErr *APIError
			if !errors.As(err, &apiErr) {
				t.Fatalf("expected APIError, got %T", err)
			}
			if apiErr.StatusCode != tt.statusCode {
				t.Fatalf("unexpected status code: got %d want %d", apiErr.StatusCode, tt.statusCode)
			}
			if apiErr.Error() != tt.wantErr {
				t.Fatalf("unexpected error message: got %q want %q", apiErr.Error(), tt.wantErr)
			}
			if tt.statusCode >= 400 && tt.statusCode != 401 && tt.statusCode != 429 && !strings.Contains(apiErr.Body, tt.body) && tt.body != "" {
				t.Fatalf("expected body to be preserved, got %q", apiErr.Body)
			}
		})
	}
}

func TestGetPostPutDeleteDelegateToRequest(t *testing.T) {
	methods := map[string]func(*Client, string) ([]byte, error){
		"GET": func(c *Client, path string) ([]byte, error) {
			return c.Get(path)
		},
		"POST": func(c *Client, path string) ([]byte, error) {
			return c.Post(path, nil)
		},
		"PUT": func(c *Client, path string) ([]byte, error) {
			return c.Put(path, nil)
		},
		"DELETE": func(c *Client, path string) ([]byte, error) {
			return c.Delete(path)
		},
	}

	for method, fn := range methods {
		t.Run(method, func(t *testing.T) {
			sawMethod := ""
			c := testClient(t, func(r *http.Request) (*http.Response, error) {
				sawMethod = r.Method
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(`{}`)),
				}, nil
			})

			_, err := fn(c, "/test")
			if err != nil {
				t.Fatalf("%s returned error: %v", method, err)
			}
			if sawMethod != method {
				t.Fatalf("expected method %s, got %s", method, sawMethod)
			}
		})
	}
}

func TestRequestWithNilBody(t *testing.T) {
	c := testClient(t, func(r *http.Request) (*http.Response, error) {
		// Body should be nil/empty for nil input
		if r.Body != nil {
			body, _ := io.ReadAll(r.Body)
			if len(body) > 0 {
				t.Fatalf("expected empty body, got %s", string(body))
			}
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"ok":true}`)),
		}, nil
	})

	_, err := c.Request(http.MethodGet, "/test", nil)
	if err != nil {
		t.Fatalf("Request returned error: %v", err)
	}
}

func TestRequestWithoutToken(t *testing.T) {
	sawAuth := ""
	c := testClient(t, func(r *http.Request) (*http.Response, error) {
		sawAuth = r.Header.Get("Authorization")
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{}`)),
		}, nil
	})
	// Token is empty by default

	_, err := c.Request(http.MethodGet, "/test", nil)
	if err != nil {
		t.Fatalf("Request returned error: %v", err)
	}
	if sawAuth != "" {
		t.Fatalf("expected no auth header, got %q", sawAuth)
	}
}

func TestRequestNetworkError(t *testing.T) {
	c := testClient(t, func(r *http.Request) (*http.Response, error) {
		return nil, errors.New("connection refused")
	})

	_, err := c.Request(http.MethodGet, "/test", nil)
	if err == nil {
		t.Fatal("expected error for network failure")
	}
	if !strings.Contains(err.Error(), "connection refused") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRequestUserAgentHeader(t *testing.T) {
	sawUA := ""
	c := testClient(t, func(r *http.Request) (*http.Response, error) {
		sawUA = r.Header.Get("User-Agent")
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{}`)),
		}, nil
	})

	_, err := c.Request(http.MethodGet, "/test", nil)
	if err != nil {
		t.Fatalf("Request returned error: %v", err)
	}
	if sawUA != "dalang-cli/1.0" {
		t.Fatalf("unexpected User-Agent: %q", sawUA)
	}
}

func TestStreamGetSuccess(t *testing.T) {
	c := testClient(t, func(r *http.Request) (*http.Response, error) {
		if r.Method != "GET" {
			t.Fatalf("expected GET, got %s", r.Method)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("file content")),
		}, nil
	})
	c.Token = "test-token"

	resp, err := c.StreamGet("/vps/file/download?vps_id=123")
	if err != nil {
		t.Fatalf("StreamGet returned error: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if string(body) != "file content" {
		t.Fatalf("unexpected body: %s", string(body))
	}
}

func TestStreamGetAuthError(t *testing.T) {
	c := testClient(t, func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: 401,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"error":"unauthorized"}`)),
		}, nil
	})

	_, err := c.StreamGet("/test")
	if err == nil {
		t.Fatal("expected error for 401")
	}

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected APIError, got %T", err)
	}
	if apiErr.StatusCode != 401 {
		t.Fatalf("expected status 401, got %d", apiErr.StatusCode)
	}
}

func TestStreamGetServerError(t *testing.T) {
	c := testClient(t, func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: 500,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"error":"internal server error"}`)),
		}, nil
	})

	_, err := c.StreamGet("/test")
	if err == nil {
		t.Fatal("expected error for 500")
	}

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected APIError, got %T", err)
	}
	if apiErr.StatusCode != 500 {
		t.Fatalf("expected status 500, got %d", apiErr.StatusCode)
	}
}

func TestStreamGetNetworkError(t *testing.T) {
	c := testClient(t, func(r *http.Request) (*http.Response, error) {
		return nil, errors.New("network timeout")
	})

	_, err := c.StreamGet("/test")
	if err == nil {
		t.Fatal("expected error for network failure")
	}
}

func TestUploadMultipartSuccess(t *testing.T) {
	sawContentType := ""
	sawAuth := ""

	c := testClient(t, func(r *http.Request) (*http.Response, error) {
		sawContentType = r.Header.Get("Content-Type")
		sawAuth = r.Header.Get("Authorization")
		if r.Method != "POST" {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"success":true}`)),
		}, nil
	})
	c.Token = "upload-token"

	body := strings.NewReader("file data")
	resp, err := c.UploadMultipart("/vps/file/upload", "multipart/form-data; boundary=xxx", body)
	if err != nil {
		t.Fatalf("UploadMultipart returned error: %v", err)
	}
	defer resp.Body.Close()

	if sawContentType != "multipart/form-data; boundary=xxx" {
		t.Fatalf("unexpected content type: %q", sawContentType)
	}
	if sawAuth != "Bearer upload-token" {
		t.Fatalf("unexpected auth header: %q", sawAuth)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}
}

func TestResponseTypesSerialization(t *testing.T) {
	// Test VPS JSON round-trip
	vps := VPS{
		ID:          "test-123",
		Name:        "my-vm",
		DisplayName: "MyVM",
		Status:      "RUNNING",
		VCPU:        2,
		RAM:         2048,
		Storage:     20,
		Bandwidth:   40,
		Price:       70000,
		Region:      "ID-BANTEN-02",
	}

	data, err := json.Marshal(vps)
	if err != nil {
		t.Fatalf("Marshal VPS failed: %v", err)
	}

	var decoded VPS
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal VPS failed: %v", err)
	}

	if decoded.ID != vps.ID || decoded.Name != vps.Name || decoded.VCPU != vps.VCPU {
		t.Fatalf("VPS round-trip mismatch: got %+v", decoded)
	}
}

func TestVPSListResponseDeserialization(t *testing.T) {
	raw := `{
		"success": true,
		"data": [
			{
				"id": "vps-1",
				"name": "test-vm",
				"display_name": "TestVM",
				"status": "RUNNING",
				"vcpu": 2,
				"ram": 2048,
				"storage": 20,
				"bandwidth": 40,
				"price": 70000,
				"expired_at": "2025-12-31T00:00:00Z",
				"region": "ID-BANTEN-02"
			}
		]
	}`

	var resp VPSListResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("Unmarshal VPSListResponse failed: %v", err)
	}

	if !resp.Success {
		t.Fatal("expected success=true")
	}
	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 VPS, got %d", len(resp.Data))
	}
	if resp.Data[0].DisplayName != "TestVM" {
		t.Fatalf("unexpected display name: %s", resp.Data[0].DisplayName)
	}
}

func TestBalanceResponseDeserialization(t *testing.T) {
	raw := `{
		"success": true,
		"data": {
			"balance": 500000,
			"total_topup": 1000000,
			"total_spent": 500000,
			"total_commission": 0
		}
	}`

	var resp BalanceResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("Unmarshal BalanceResponse failed: %v", err)
	}

	if resp.Data.Balance != 500000 {
		t.Fatalf("expected balance 500000, got %d", resp.Data.Balance)
	}
}

func TestTransactionsResponseDeserialization(t *testing.T) {
	raw := `{
		"success": true,
		"data": {
			"transactions": [
				{
					"id": 1,
					"type": "topup",
					"amount": 100000,
					"balance_after": 100000,
					"description": "Credit topup",
					"created_at": "2025-01-01T00:00:00Z"
				}
			],
			"total": 1,
			"page": 1,
			"limit": 25
		}
	}`

	var resp TransactionsResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("Unmarshal TransactionsResponse failed: %v", err)
	}

	if len(resp.Data.Transactions) != 1 {
		t.Fatalf("expected 1 transaction, got %d", len(resp.Data.Transactions))
	}
	if resp.Data.Transactions[0].Type != "topup" {
		t.Fatalf("expected type topup, got %s", resp.Data.Transactions[0].Type)
	}
}

func TestContainerListResponseDeserialization(t *testing.T) {
	raw := `{
		"success": true,
		"data": {
			"containers": [
				{
					"id": "cont-1",
					"name": "redis",
					"container_type": "redis",
					"status": "running",
					"plan": "basic",
					"expires_at": "2025-12-31"
				}
			]
		}
	}`

	var resp ContainerListResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("Unmarshal ContainerListResponse failed: %v", err)
	}

	if len(resp.Data.Containers) != 1 {
		t.Fatalf("expected 1 container, got %d", len(resp.Data.Containers))
	}
}

func TestDeploymentListResponseDeserialization(t *testing.T) {
	raw := `{
		"success": true,
		"data": [
			{
				"id": "dep-1",
				"name": "my-app",
				"status": "active",
				"repo_url": "https://github.com/user/repo",
				"branch": "main",
				"url": "https://my-app.dalang.io",
				"expires_at": "2025-12-31"
			}
		]
	}`

	var resp DeploymentListResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("Unmarshal DeploymentListResponse failed: %v", err)
	}

	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 deployment, got %d", len(resp.Data))
	}
	if resp.Data[0].Branch != "main" {
		t.Fatalf("expected branch main, got %s", resp.Data[0].Branch)
	}
}

func TestCLIAuthInitResponseDeserialization(t *testing.T) {
	raw := `{
		"success": true,
		"data": {
			"device_code": "abc123",
			"user_code": "USER-1234",
			"verification_url": "https://dalang.io/auth/device",
			"expires_in": 900,
			"interval": 5
		}
	}`

	var resp CLIAuthInitResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("Unmarshal CLIAuthInitResponse failed: %v", err)
	}

	if resp.Data.UserCode != "USER-1234" {
		t.Fatalf("expected user code USER-1234, got %s", resp.Data.UserCode)
	}
	if resp.Data.ExpiresIn != 900 {
		t.Fatalf("expected expires_in 900, got %d", resp.Data.ExpiresIn)
	}
}

func TestCLIAuthPollResponseDeserialization(t *testing.T) {
	raw := `{
		"success": true,
		"data": {
			"access_token": "at_123",
			"refresh_token": "rt_456",
			"email": "user@example.com",
			"expires_in": 86400
		}
	}`

	var resp CLIAuthPollResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("Unmarshal CLIAuthPollResponse failed: %v", err)
	}

	if resp.Data.Email != "user@example.com" {
		t.Fatalf("expected email user@example.com, got %s", resp.Data.Email)
	}
}

func TestTopupResponseDeserialization(t *testing.T) {
	raw := `{
		"success": true,
		"data": {
			"invoice_url": "https://pay.dalang.io/inv-123",
			"invoice_id": "inv-123",
			"amount": 100000
		}
	}`

	var resp TopupResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("Unmarshal TopupResponse failed: %v", err)
	}

	if resp.Data.Amount != 100000 {
		t.Fatalf("expected amount 100000, got %d", resp.Data.Amount)
	}
}
