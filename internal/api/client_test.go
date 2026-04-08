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
