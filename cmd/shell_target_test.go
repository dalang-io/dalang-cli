package cmd

// `dalang shell <name>` for something that is not a VPS: which container or app a name
// refers to, the URL of its shell, and that a VPS miss (not a failure) is the only
// thing that sends the lookup on to containers and apps.

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dalang-io/dalang-cli/internal/api"
	"github.com/dalang-io/dalang-cli/internal/config"
)

func sampleCandidates() []shellCandidate {
	return []shellCandidate{
		containerCandidate(api.Container{ID: "ctr-1111", Name: "svc-aaaa1111", DisplayName: "orders-db", ContainerType: "postgresql", Status: "RUNNING"}),
		containerCandidate(api.Container{ID: "ctr-2222", Name: "svc-bbbb2222", Status: "STOPPED"}),
		appCandidate(api.Deployment{ID: "dep-3333", RepoFullName: "dalang-io/example-app-go", Subdomain: "example-app-go-3333", Status: "running"}),
		appCandidate(api.Deployment{ID: "dep-4444", DisplayName: "My Shop", RepoFullName: "acme/shop", Status: "running"}),
	}
}

func ids(ts []shellTarget) string {
	var out []string
	for _, t := range ts {
		out = append(out, t.ID)
	}
	return strings.Join(out, ",")
}

func TestMatchShellCandidates(t *testing.T) {
	c := sampleCandidates()
	for name, want := range map[string]string{
		"orders-db":                "ctr-1111", // display name
		"svc-aaaa1111":             "ctr-1111", // instance name
		"ctr-2222":                 "ctr-2222", // id
		"ORDERS-DB":                "ctr-1111", // case-insensitive only without an exact match
		"example-app-go":           "dep-3333", // the repository's short name
		"dalang-io/example-app-go": "dep-3333",
		"example-app-go-3333":      "dep-3333", // subdomain
		"My Shop":                  "dep-4444", // display name wins over the repository
		"acme/shop":                "dep-4444",
		"nothing-like-it":          "",
	} {
		if got := ids(matchShellCandidates(c, name)); got != want {
			t.Errorf("%q -> %q, want %q", name, got, want)
		}
	}
}

func TestMatchShellCandidates_ExactBeatsCaseInsensitive(t *testing.T) {
	c := []shellCandidate{
		containerCandidate(api.Container{ID: "a", DisplayName: "Web", Status: "RUNNING"}),
		containerCandidate(api.Container{ID: "b", DisplayName: "web", Status: "RUNNING"}),
	}
	if got := ids(matchShellCandidates(c, "web")); got != "b" {
		t.Errorf("exact match should win, got %q", got)
	}
	if got := ids(matchShellCandidates(c, "WEB")); got != "a,b" {
		t.Errorf("without an exact match both fold-match, got %q", got)
	}
}

func TestServiceTerminalURL(t *testing.T) {
	for in, want := range map[string]string{
		"https://api.dalang.io":  "wss://api.dalang.io/services/terminal?kind=app&id=dep-1",
		"http://localhost:8080":  "ws://localhost:8080/services/terminal?kind=app&id=dep-1",
		"https://api.dalang.io/": "wss://api.dalang.io/services/terminal?kind=app&id=dep-1",
	} {
		if got := serviceTerminalURL(in, "app", "dep-1"); got != want {
			t.Errorf("serviceTerminalURL(%q) = %q, want %q", in, got, want)
		}
	}
	if got := serviceTerminalURL("https://x", "container", "a b&c"); !strings.HasSuffix(got, "id=a+b%26c") {
		t.Errorf("the id must be escaped: %q", got)
	}
}

func TestConnectServiceTerminal_RefusesConsoleAndNotRunning(t *testing.T) {
	resetGlobalFlags()
	t.Cleanup(resetGlobalFlags)

	err := connectServiceTerminal(&shellTarget{Kind: "app", ID: "dep-1", Name: "shop", Status: "running"}, "console")
	if err == nil || !strings.Contains(err.Error(), "only for a VPS") {
		t.Errorf("console on an app: %v", err)
	}
	err = connectServiceTerminal(&shellTarget{Kind: "container", ID: "c", Name: "db", Status: "STOPPED"}, "shell")
	if err == nil || !strings.Contains(err.Error(), "not running") {
		t.Errorf("a stopped container: %v", err)
	}
}

func TestNamesShownByServiceList(t *testing.T) {
	if got := deploymentName(api.Deployment{DisplayName: "Shop", RepoFullName: "acme/shop"}); got != "Shop" {
		t.Errorf("display name first, got %q", got)
	}
	// The API sends no "name" for a deployment: this used to be blank.
	if got := deploymentName(api.Deployment{RepoFullName: "acme/shop"}); got != "acme/shop" {
		t.Errorf("the repository when there is no display name, got %q", got)
	}
	if got := containerName(api.Container{Name: "svc-1", DisplayName: "db"}); got != "db" {
		t.Errorf("container display name, got %q", got)
	}
}

// fakeAPI answers the three list endpoints and records which were asked for.
func fakeAPI(t *testing.T, vps, containers, deployments string) (asked *[]string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/vps/list":
			w.Write([]byte(vps))
		case "/containers/list":
			w.Write([]byte(containers))
		case "/github/deployments":
			w.Write([]byte(deployments))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	t.Setenv("DALANG_API_URL", srv.URL)
	if err := config.SaveCredentials(&config.Credentials{AccessToken: "tok", ExpiresAt: 4102444800}); err != nil {
		t.Fatalf("save credentials: %v", err)
	}
	return &seen
}

func TestFindServiceShellTarget_FindsAnAppByRepositoryName(t *testing.T) {
	resetGlobalFlags()
	t.Cleanup(resetGlobalFlags)
	fakeAPI(t, `{"success":true,"data":[]}`,
		`{"success":true,"data":{"containers":[{"id":"ctr-1","name":"svc-1","display_name":"db","status":"RUNNING"}]}}`,
		`{"success":true,"data":[{"id":"dep-9","repo_full_name":"dalang-io/example-app-go","status":"running"}]}`)
	client, err := api.NewAuthenticatedClient()
	if err != nil {
		t.Fatal(err)
	}

	got, err := findServiceShellTarget(client, "example-app-go")
	if err != nil || got == nil || got.Kind != "app" || got.ID != "dep-9" || got.Name != "dalang-io/example-app-go" {
		t.Fatalf("got %+v, %v", got, err)
	}
	if got, err := findServiceShellTarget(client, "unknown"); got != nil || err != nil {
		t.Errorf("no match must be (nil, nil), got %+v, %v", got, err)
	}
}

func TestFindServiceShellTarget_AmbiguousNameListsTheIds(t *testing.T) {
	resetGlobalFlags()
	t.Cleanup(resetGlobalFlags)
	fakeAPI(t, `{"success":true,"data":[]}`, `{"success":true,"data":{"containers":[]}}`,
		`{"success":true,"data":[{"id":"dep-1","repo_full_name":"acme/web","status":"running"},{"id":"dep-2","repo_full_name":"acme/web","status":"stopped"}]}`)
	client, _ := api.NewAuthenticatedClient()

	_, err := findServiceShellTarget(client, "web")
	if err == nil || !strings.Contains(err.Error(), "dep-1") || !strings.Contains(err.Error(), "dep-2") || !strings.Contains(err.Error(), "use the id") {
		t.Errorf("ambiguous name: %v", err)
	}
}

func TestVPSMissIsAMissNotAFailure(t *testing.T) {
	resetGlobalFlags()
	t.Cleanup(resetGlobalFlags)
	fakeAPI(t, `{"success":true,"data":[{"id":"v1","name":"binus","status":"RUNNING"}]}`, `{}`, `{}`)
	client, _ := api.NewAuthenticatedClient()

	_, err := findVPSByName(client, "binuss")
	var missing *notFoundError
	if !errors.As(err, &missing) || !strings.Contains(err.Error(), "Did you mean: binus") {
		t.Errorf("a VPS miss must be a notFoundError that still suggests names: %v", err)
	}
}
