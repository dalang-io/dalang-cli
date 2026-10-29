package cmd

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/dalang-io/dalang-cli/internal/api"
	"github.com/dalang-io/dalang-cli/internal/config"
	"github.com/dalang-io/dalang-cli/internal/terminal"
)

// A shell in a container or a GitHub app. A VPS keeps its own path (connectTerminal);
// this is only reached when the name is no VPS. The API side is /services/terminal
// (api.dalang.io specs/03): the same WebSocket-over-PTY session as /vps/terminal, owner
// or admin only, running services only, bearer token in the header.

type shellTarget struct {
	Kind   string // "container" | "app"
	ID     string
	Name   string // what to call it in messages
	Status string
}

func (t shellTarget) running() bool { return strings.EqualFold(t.Status, "running") }

// shellCandidate is a service together with every name a customer may call it by.
type shellCandidate struct {
	shellTarget
	names []string
}

func containerName(c api.Container) string {
	if c.DisplayName != "" {
		return c.DisplayName
	}
	return c.Name
}

// deploymentName: the name given in the dashboard, else the repository.
func deploymentName(d api.Deployment) string {
	switch {
	case d.DisplayName != "":
		return d.DisplayName
	case d.RepoFullName != "":
		return d.RepoFullName
	}
	return d.Name
}

func containerCandidate(c api.Container) shellCandidate {
	return shellCandidate{
		shellTarget: shellTarget{Kind: "container", ID: c.ID, Name: containerName(c), Status: c.Status},
		names:       []string{c.DisplayName, c.Name},
	}
}

func appCandidate(d api.Deployment) shellCandidate {
	names := []string{d.DisplayName, d.RepoFullName, d.Subdomain, d.Name}
	if i := strings.LastIndex(d.RepoFullName, "/"); i >= 0 {
		names = append(names, d.RepoFullName[i+1:]) // "example-app-go" for "dalang-io/example-app-go"
	}
	return shellCandidate{shellTarget: shellTarget{Kind: "app", ID: d.ID, Name: deploymentName(d), Status: d.Status}, names: names}
}

// matchShellCandidates returns what `name` refers to: an exact match on the id or any
// name wins; only without one is the case-insensitive match used.
func matchShellCandidates(cands []shellCandidate, name string) []shellTarget {
	for _, fold := range []bool{false, true} {
		var out []shellTarget
		for _, c := range cands {
			if c.matches(name, fold) {
				out = append(out, c.shellTarget)
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	return nil
}

func (c shellCandidate) matches(name string, fold bool) bool {
	eq := func(a string) bool {
		if a == "" {
			return false
		}
		if fold {
			return strings.EqualFold(a, name)
		}
		return a == name
	}
	if eq(c.ID) {
		return true
	}
	for _, n := range c.names {
		if eq(n) {
			return true
		}
	}
	return false
}

// findServiceShellTarget looks `name` up among the account's containers and apps. nil
// with no error means nothing by that name (the caller reports the VPS miss); more than
// one match is an error that says which ids to choose from.
func findServiceShellTarget(client *api.Client, name string) (*shellTarget, error) {
	var cands []shellCandidate

	if resp, err := client.Get("/containers/list"); err != nil {
		PrintDebug("Containers fetch error: %v", err)
	} else {
		var data api.ContainerListResponse
		if json.Unmarshal(resp, &data) == nil {
			for _, c := range data.Data.Containers {
				cands = append(cands, containerCandidate(c))
			}
		}
	}
	if resp, err := client.Get("/github/deployments"); err != nil {
		PrintDebug("Deployments fetch error: %v", err)
	} else {
		var data api.DeploymentListResponse
		if json.Unmarshal(resp, &data) == nil {
			for _, d := range data.Data {
				cands = append(cands, appCandidate(d))
			}
		}
	}

	matches := matchShellCandidates(cands, name)
	switch len(matches) {
	case 0:
		return nil, nil
	case 1:
		return &matches[0], nil
	}
	var lines []string
	for _, m := range matches {
		lines = append(lines, fmt.Sprintf("  %s  %s  (%s, %s)", m.ID, m.Name, m.Kind, m.Status))
	}
	return nil, fmt.Errorf("'%s' matches %d services; use the id of the one you mean:\n%s", name, len(matches), strings.Join(lines, "\n"))
}

// serviceTerminalURL is the WebSocket URL of a container or app shell.
func serviceTerminalURL(apiURL, kind, id string) string {
	ws := strings.Replace(apiURL, "https://", "wss://", 1)
	ws = strings.Replace(ws, "http://", "ws://", 1)
	return fmt.Sprintf("%s/services/terminal?kind=%s&id=%s", strings.TrimRight(ws, "/"), url.QueryEscape(kind), url.QueryEscape(id))
}

func connectServiceTerminal(t *shellTarget, mode string) error {
	if mode == "console" {
		return fmt.Errorf("'%s' is a %s: 'console' is only for a VPS. Use 'dalang shell %s'", t.Name, t.Kind, t.Name)
	}
	if !t.running() {
		return fmt.Errorf("%s '%s' is not running (status: %s)", t.Kind, t.Name, t.Status)
	}
	creds, err := config.LoadCredentials()
	if err != nil {
		return fmt.Errorf("failed to load credentials: %w", err)
	}

	wsURL := serviceTerminalURL(config.GetAPIURL(), t.Kind, t.ID)
	PrintDebug("WebSocket URL: %s", wsURL)
	printInfo("Connecting to %s %s...", t.Kind, t.Name)

	term, err := terminal.NewTerminal(wsURL, creds.AccessToken)
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}
	defer term.Close()
	printSuccess("Connected.\n")
	return runTerminalSession(term)
}
