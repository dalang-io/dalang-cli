package cmd

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/dalang-io/dalang-cli/internal/api"
)

// findVPSByName resolves a VPS by its name or display name. Matching is exact
// (case-sensitive) first, then case-insensitive. When nothing matches it returns
// an error that suggests the closest known names ("did you mean ...?").
//
// This is the single lookup path shared by shell, exec, vm actions, service
// upgrade/extend, and domain commands.
func findVPSByName(client *api.Client, name string) (*api.VPS, error) {
	resp, err := client.Get("/vps/list")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch services: %w", err)
	}

	var vpsData api.VPSListResponse
	if err := json.Unmarshal(resp, &vpsData); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Exact match wins.
	for i := range vpsData.Data {
		if vpsData.Data[i].Name == name || vpsData.Data[i].DisplayName == name {
			return &vpsData.Data[i], nil
		}
	}

	// Fall back to case-insensitive match (e.g. "Binus" vs "binus").
	for i := range vpsData.Data {
		if strings.EqualFold(vpsData.Data[i].Name, name) ||
			strings.EqualFold(vpsData.Data[i].DisplayName, name) {
			return &vpsData.Data[i], nil
		}
	}

	return nil, vpsNotFoundError(name, vpsData.Data)
}

// vpsNotFoundError builds a "not found" error, appending up to three of the
// closest-looking names as suggestions when any are reasonably similar.
func vpsNotFoundError(name string, list []api.VPS) error {
	suggestions := suggestNames(name, list, 3)
	if len(suggestions) == 0 {
		return fmt.Errorf("VPS '%s' not found", name)
	}
	return fmt.Errorf("VPS '%s' not found. Did you mean: %s?", name, strings.Join(suggestions, ", "))
}

// suggestNames returns up to max VPS names similar to the query, ranked by edit
// distance. A name qualifies when it is within a small edit distance or is a
// substring relationship with the query.
func suggestNames(name string, list []api.VPS, max int) []string {
	q := strings.ToLower(name)

	type scored struct {
		name string
		dist int
	}
	var cands []scored
	seen := map[string]bool{}
	for _, v := range list {
		n := v.DisplayName
		if n == "" {
			n = v.Name
		}
		if n == "" || seen[n] {
			continue
		}
		seen[n] = true

		ln := strings.ToLower(n)
		d := levenshtein(q, ln)
		if d <= 3 || strings.Contains(ln, q) || strings.Contains(q, ln) {
			cands = append(cands, scored{n, d})
		}
	}

	sort.Slice(cands, func(i, j int) bool { return cands[i].dist < cands[j].dist })

	var out []string
	for i := 0; i < len(cands) && i < max; i++ {
		out = append(out, cands[i].name)
	}
	return out
}

// levenshtein returns the edit distance between two strings.
func levenshtein(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	la, lb := len(ra), len(rb)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}

	prev := make([]int, lb+1)
	curr := make([]int, lb+1)
	for j := 0; j <= lb; j++ {
		prev[j] = j
	}
	for i := 1; i <= la; i++ {
		curr[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			curr[j] = min3(prev[j]+1, curr[j-1]+1, prev[j-1]+cost)
		}
		prev, curr = curr, prev
	}
	return prev[lb]
}

func min3(a, b, c int) int {
	m := a
	if b < m {
		m = b
	}
	if c < m {
		m = c
	}
	return m
}
