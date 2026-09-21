package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestSaveAndGetTunnelReclaim(t *testing.T) {
	setTestHome(t)

	ticket := TunnelReclaim{
		Label:         "kucing-makan-ikan",
		URL:           "https://kucing-makan-ikan.try.dalang.io",
		ReclaimToken:  "dG9rZW4tMQ",
		UntilEstimate: time.Now().Add(6 * time.Hour).UTC().Format(time.RFC3339),
		WindowSeconds: 21600,
	}
	if err := SaveTunnelReclaim(ticket); err != nil {
		t.Fatalf("SaveTunnelReclaim returned error: %v", err)
	}

	got, ok := GetTunnelReclaim("kucing-makan-ikan")
	if !ok {
		t.Fatal("GetTunnelReclaim did not find the ticket that was just saved")
	}
	if got.ReclaimToken != ticket.ReclaimToken || got.URL != ticket.URL {
		t.Fatalf("round trip lost data: %+v", got)
	}
	if got.SavedAt == "" {
		t.Fatal("SavedAt should be stamped on save")
	}
}

// TestTunnelReclaimFilePermissions: the token is the only proof an address is
// yours, so the file is 0600 like `credentials`.
func TestTunnelReclaimFilePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix file modes do not apply on windows")
	}
	home := setTestHome(t)

	if err := SaveTunnelReclaim(TunnelReclaim{Label: "a-b-c", ReclaimToken: "tok"}); err != nil {
		t.Fatalf("SaveTunnelReclaim returned error: %v", err)
	}

	info, err := os.Stat(filepath.Join(home, ".dalang", "tunnels.json"))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Fatalf("tunnels.json mode = %o, want 600", perm)
	}
}

func TestGetTunnelReclaimIgnoresExpired(t *testing.T) {
	setTestHome(t)

	// Written straight into the store: SaveTunnelReclaim prunes on the way in,
	// and this is about a ticket that expired while it sat on disk.
	store, err := LoadTunnelStore()
	if err != nil {
		t.Fatalf("LoadTunnelStore: %v", err)
	}
	store.Reclaims["a-b-c"] = TunnelReclaim{
		Label:         "a-b-c",
		ReclaimToken:  "tok",
		UntilEstimate: time.Now().Add(-time.Minute).UTC().Format(time.RFC3339),
	}
	if _, ok := store.Get("a-b-c"); ok {
		t.Fatal("an expired reservation must not be offered for reclaim")
	}
}

func TestSaveTunnelStorePrunesExpired(t *testing.T) {
	setTestHome(t)

	if err := SaveTunnelReclaim(TunnelReclaim{
		Label:         "live-one-here",
		ReclaimToken:  "tok",
		UntilEstimate: time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("SaveTunnelReclaim: %v", err)
	}

	store, err := LoadTunnelStore()
	if err != nil {
		t.Fatalf("LoadTunnelStore: %v", err)
	}
	store.Reclaims["dead-one-here"] = TunnelReclaim{
		Label:         "dead-one-here",
		ReclaimToken:  "tok",
		UntilEstimate: time.Now().Add(-time.Hour).UTC().Format(time.RFC3339),
	}
	if err := SaveTunnelStore(store); err != nil {
		t.Fatalf("SaveTunnelStore: %v", err)
	}

	reloaded, err := LoadTunnelStore()
	if err != nil {
		t.Fatalf("LoadTunnelStore: %v", err)
	}
	if _, ok := reloaded.Reclaims["dead-one-here"]; ok {
		t.Fatal("expired entries should be pruned on save, not kept forever")
	}
	if _, ok := reloaded.Reclaims["live-one-here"]; !ok {
		t.Fatal("pruning removed a live entry")
	}
}

func TestDeleteTunnelReclaim(t *testing.T) {
	setTestHome(t)

	if err := SaveTunnelReclaim(TunnelReclaim{Label: "a-b-c", ReclaimToken: "tok"}); err != nil {
		t.Fatalf("SaveTunnelReclaim: %v", err)
	}
	if err := DeleteTunnelReclaim("a-b-c"); err != nil {
		t.Fatalf("DeleteTunnelReclaim: %v", err)
	}
	if _, ok := GetTunnelReclaim("a-b-c"); ok {
		t.Fatal("ticket still present after delete")
	}
	// Deleting something that is not there is not an error.
	if err := DeleteTunnelReclaim("a-b-c"); err != nil {
		t.Fatalf("second DeleteTunnelReclaim: %v", err)
	}
}

func TestLoadTunnelStoreOnFreshMachine(t *testing.T) {
	setTestHome(t)

	store, err := LoadTunnelStore()
	if err != nil {
		t.Fatalf("a missing store must not be an error, got %v", err)
	}
	if len(store.Reclaims) != 0 {
		t.Fatalf("expected an empty store, got %+v", store.Reclaims)
	}
	if _, ok := store.Get("a-b-c"); ok {
		t.Fatal("empty store returned a ticket")
	}
}

func TestTunnelReclaimExpired(t *testing.T) {
	now := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{name: "future", in: "2026-09-21T16:00:00Z", want: false},
		{name: "past", in: "2026-09-21T11:59:00Z", want: true},
		// Never throw a token away over a format surprise: the daemon decides.
		{name: "empty", in: "", want: false},
		{name: "unparseable", in: "soon", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := TunnelReclaim{UntilEstimate: tt.in}
			if got := r.Expired(now); got != tt.want {
				t.Fatalf("Expired(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

// TestRefreshTunnelReclaimDeadline: the window runs from the close, so the
// deadline written at assignment time is only a worst case until the socket
// actually goes away.
func TestRefreshTunnelReclaimDeadline(t *testing.T) {
	setTestHome(t)

	assignedAt := time.Now()
	if err := SaveTunnelReclaim(TunnelReclaim{
		Label:         "kucing-makan-ikan",
		ReclaimToken:  "tok",
		WindowSeconds: 21600,
		UntilEstimate: assignedAt.Add(6 * time.Hour).UTC().Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("SaveTunnelReclaim: %v", err)
	}

	closedAt := assignedAt.Add(2 * time.Hour)
	if err := RefreshTunnelReclaimDeadline("kucing-makan-ikan", closedAt); err != nil {
		t.Fatalf("RefreshTunnelReclaimDeadline: %v", err)
	}

	got, ok := GetTunnelReclaim("kucing-makan-ikan")
	if !ok {
		t.Fatal("ticket disappeared")
	}
	want := closedAt.Add(6 * time.Hour).UTC().Format(time.RFC3339)
	if got.UntilEstimate != want {
		t.Fatalf("estimate = %q, want %q (measured from the close, not the assignment)", got.UntilEstimate, want)
	}
}

func TestRefreshTunnelReclaimDeadlineIgnoresUnknownLabels(t *testing.T) {
	setTestHome(t)

	if err := RefreshTunnelReclaimDeadline("never-held-here", time.Now()); err != nil {
		t.Fatalf("refreshing an unknown label should be a no-op, got %v", err)
	}
	if _, ok := GetTunnelReclaim("never-held-here"); ok {
		t.Fatal("refresh invented a ticket")
	}
}
