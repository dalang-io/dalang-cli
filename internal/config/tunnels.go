package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// tunnelsFileName holds tunnel reclaim tokens. It lives beside `credentials`
// and `config.json` in ~/.dalang and is written 0600 for the same reason
// `credentials` is: a reclaim token is the only proof that a public address is
// yours, so anyone who can read it can take the address back from you.
const tunnelsFileName = "tunnels.json"

// TunnelReclaim is one label's claim ticket, from the `assigned` frame.
type TunnelReclaim struct {
	Label        string `json:"label"`
	URL          string `json:"url,omitempty"`
	ReclaimToken string `json:"reclaim_token"`
	// WindowSeconds is the reservation window the daemon announced, measured
	// from the moment the connection closes.
	WindowSeconds int64 `json:"reclaim_window_seconds,omitempty"`
	// UntilEstimate is RFC3339 UTC and is **ours, not the daemon's**: the wire
	// carries a duration because the deadline cannot be known at assignment
	// time. It is good enough to prune this file and to tell the user roughly
	// how long they have; the daemon decides on its own clock.
	UntilEstimate string `json:"reclaim_until_estimate,omitempty"`
	SavedAt       string `json:"saved_at,omitempty"`
}

// Expired reports whether our *estimate* of the reservation window has passed,
// which is only ever used to prune this file. An empty or unparseable timestamp
// counts as not expired: the daemon is the authority, and discarding a token
// over a format surprise — or over a clock we computed ourselves — would cost
// the user their address.
func (r TunnelReclaim) Expired(now time.Time) bool {
	if r.UntilEstimate == "" {
		return false
	}
	until, err := time.Parse(time.RFC3339, r.UntilEstimate)
	if err != nil {
		return false
	}
	return now.After(until)
}

// TunnelStore is the on-disk shape: reclaim tickets keyed by label.
type TunnelStore struct {
	Reclaims map[string]TunnelReclaim `json:"reclaims"`
}

// LoadTunnelStore reads the store. A missing file is an empty store, not an
// error — the common case is a machine that has never run `dalang tunnel`.
func LoadTunnelStore() (*TunnelStore, error) {
	dir, err := GetConfigDir()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(filepath.Join(dir, tunnelsFileName))
	if err != nil {
		if os.IsNotExist(err) {
			return &TunnelStore{Reclaims: map[string]TunnelReclaim{}}, nil
		}
		return nil, err
	}

	var store TunnelStore
	if err := json.Unmarshal(data, &store); err != nil {
		return nil, err
	}
	if store.Reclaims == nil {
		store.Reclaims = map[string]TunnelReclaim{}
	}
	return &store, nil
}

// Get returns the live reclaim ticket for a label, if there is one.
func (s *TunnelStore) Get(label string) (TunnelReclaim, bool) {
	if s == nil || s.Reclaims == nil {
		return TunnelReclaim{}, false
	}
	r, ok := s.Reclaims[label]
	if !ok || r.ReclaimToken == "" || r.Expired(time.Now()) {
		return TunnelReclaim{}, false
	}
	return r, true
}

// SaveTunnelStore writes the store, dropping entries whose window has passed so
// the file does not grow forever.
func SaveTunnelStore(store *TunnelStore) error {
	if err := EnsureConfigDir(); err != nil {
		return err
	}
	dir, err := GetConfigDir()
	if err != nil {
		return err
	}

	now := time.Now()
	pruned := make(map[string]TunnelReclaim, len(store.Reclaims))
	for label, r := range store.Reclaims {
		if r.ReclaimToken == "" || r.Expired(now) {
			continue
		}
		pruned[label] = r
	}

	data, err := json.MarshalIndent(&TunnelStore{Reclaims: pruned}, "", "  ")
	if err != nil {
		return err
	}

	// Written through a temp file and renamed: a torn write here would lose
	// every address the user could still reclaim, and tunnels are reassigned
	// often enough that an interrupted write is not hypothetical.
	path := filepath.Join(dir, tunnelsFileName)
	tmp, err := os.CreateTemp(dir, tunnelsFileName+".*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath) // no-op once the rename succeeded

	if err := tmp.Chmod(0600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

// SaveTunnelReclaim records (or replaces) one label's ticket.
func SaveTunnelReclaim(r TunnelReclaim) error {
	store, err := LoadTunnelStore()
	if err != nil {
		return err
	}
	if r.SavedAt == "" {
		r.SavedAt = time.Now().UTC().Format(time.RFC3339)
	}
	store.Reclaims[r.Label] = r
	return SaveTunnelStore(store)
}

// GetTunnelReclaim looks up a live ticket for a label.
func GetTunnelReclaim(label string) (TunnelReclaim, bool) {
	store, err := LoadTunnelStore()
	if err != nil {
		return TunnelReclaim{}, false
	}
	return store.Get(label)
}

// RefreshTunnelReclaimDeadline re-derives the estimated deadline for a label
// from a close that has actually happened. The ticket is written when the
// address is assigned (the process may die a moment later), so until the socket
// closes its deadline is only a worst case; this replaces it with the real one.
func RefreshTunnelReclaimDeadline(label string, closedAt time.Time) error {
	store, err := LoadTunnelStore()
	if err != nil {
		return err
	}
	ticket, ok := store.Reclaims[label]
	if !ok || ticket.WindowSeconds <= 0 {
		return nil
	}
	ticket.UntilEstimate = closedAt.Add(time.Duration(ticket.WindowSeconds) * time.Second).UTC().Format(time.RFC3339)
	store.Reclaims[label] = ticket
	return SaveTunnelStore(store)
}

// DeleteTunnelReclaim forgets a label, e.g. after the daemon says it is gone.
func DeleteTunnelReclaim(label string) error {
	store, err := LoadTunnelStore()
	if err != nil {
		return err
	}
	if _, ok := store.Reclaims[label]; !ok {
		return nil
	}
	delete(store.Reclaims, label)
	return SaveTunnelStore(store)
}
