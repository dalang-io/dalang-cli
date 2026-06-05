package cmd

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/dalang-io/dalang-cli/internal/api"
)

type rtFunc func(*http.Request) (*http.Response, error)

func (f rtFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// vpsListClient returns a client whose /vps/list responds with the given VPSes.
func vpsListClient(t *testing.T, vpses []api.VPS) *api.Client {
	t.Helper()
	body, err := json.Marshal(api.VPSListResponse{Success: true, Data: vpses})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return &api.Client{
		BaseURL: "https://api.dalang.io",
		HTTPClient: &http.Client{
			Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(bytes.NewReader(body)),
					Header:     make(http.Header),
				}, nil
			}),
		},
	}
}

func TestFindVPSByName_ExactAndCaseInsensitive(t *testing.T) {
	client := vpsListClient(t, []api.VPS{
		{ID: "1", Name: "binus", DisplayName: "Binus"},
		{ID: "2", Name: "web-server", DisplayName: ""},
	})

	cases := []struct {
		query  string
		wantID string
	}{
		{"binus", "1"},      // exact name
		{"Binus", "1"},      // exact display name
		{"BINUS", "1"},      // case-insensitive
		{"web-server", "2"}, // name, empty display name
		{"WEB-SERVER", "2"}, // case-insensitive name
	}
	for _, tc := range cases {
		t.Run(tc.query, func(t *testing.T) {
			vps, err := findVPSByName(client, tc.query)
			if err != nil {
				t.Fatalf("findVPSByName(%q): %v", tc.query, err)
			}
			if vps.ID != tc.wantID {
				t.Errorf("findVPSByName(%q) = ID %q, want %q", tc.query, vps.ID, tc.wantID)
			}
		})
	}
}

func TestFindVPSByName_NotFoundWithSuggestion(t *testing.T) {
	client := vpsListClient(t, []api.VPS{
		{ID: "1", Name: "binus", DisplayName: "Binus"},
		{ID: "2", Name: "dalangbot", DisplayName: "DalangBot"},
	})

	_, err := findVPSByName(client, "binut") // one typo away from "binus"/"Binus"
	if err == nil {
		t.Fatal("expected error for unknown VPS")
	}
	if !strings.Contains(err.Error(), "Did you mean") || !strings.Contains(err.Error(), "Binus") {
		t.Errorf("expected a 'Did you mean ... Binus' suggestion, got: %v", err)
	}
}

func TestFindVPSByName_NotFoundNoSuggestion(t *testing.T) {
	client := vpsListClient(t, []api.VPS{
		{ID: "1", Name: "binus", DisplayName: "Binus"},
	})

	_, err := findVPSByName(client, "zzzzzzzzz")
	if err == nil {
		t.Fatal("expected error for unknown VPS")
	}
	if strings.Contains(err.Error(), "Did you mean") {
		t.Errorf("did not expect a suggestion for a wildly different name, got: %v", err)
	}
}

func TestLevenshtein(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"", "", 0},
		{"abc", "", 3},
		{"", "abc", 3},
		{"binus", "binus", 0},
		{"binus", "binut", 1},
		{"kitten", "sitting", 3},
	}
	for _, tc := range cases {
		if got := levenshtein(tc.a, tc.b); got != tc.want {
			t.Errorf("levenshtein(%q,%q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestSuggestNames_RanksByDistanceAndDedupes(t *testing.T) {
	list := []api.VPS{
		{Name: "binus", DisplayName: "Binus"},
		{Name: "binus", DisplayName: "Binus"}, // duplicate display name
		{Name: "binary", DisplayName: ""},
		{Name: "totally-different", DisplayName: ""},
	}
	got := suggestNames("binut", list, 3)
	if len(got) == 0 || got[0] != "Binus" {
		t.Fatalf("expected closest suggestion 'Binus' first, got %v", got)
	}
	// "totally-different" should not be suggested for "binut".
	for _, s := range got {
		if s == "totally-different" {
			t.Errorf("unexpected far suggestion in %v", got)
		}
	}
}
