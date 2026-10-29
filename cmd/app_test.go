package cmd

// dalang app, against a fake API that answers the endpoints it uses and keeps what it was
// sent. What is pinned: nothing is created or charged before the repository is known to be
// buildable, the amount paid is the quoted one, a failure after the app exists says how to
// finish, and --json leaves stdout as one parseable document.

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dalang-io/dalang-cli/internal/api"
	"github.com/dalang-io/dalang-cli/internal/config"
)

type fakeGitHubAPI struct {
	t          *testing.T
	dockerfile string // body of /github/repos/dockerfile data
	quote      string
	statuses   []string // /details answers, one per call (the last repeats)
	calls      []string
	created    map[string]interface{}
	paid       map[string]interface{}
	rebuilt    map[string]interface{}
	deleted    map[string]interface{}
	settings   map[string]interface{}
	connected  bool
}

func newFakeGitHubAPI(t *testing.T) *fakeGitHubAPI {
	t.Helper()
	f := &fakeGitHubAPI{
		t:          t,
		connected:  true,
		dockerfile: `{"exists":true,"exposed_port":3000}`,
		quote:      okQuote,
		statuses:   []string{"building", "running"},
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	read := func(r *http.Request) map[string]interface{} {
		m := map[string]interface{}{}
		b, _ := io.ReadAll(r.Body)
		json.Unmarshal(b, &m)
		return m
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.calls = append(f.calls, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/github/status":
			w.Write([]byte(`{"success":true,"data":{"connected":` + map[bool]string{true: "true", false: "false"}[f.connected] + `}}`))
		case "/github/repos":
			w.Write([]byte(`{"success":true,"data":[{"full_name":"acme/shop","private":true,"default_branch":"trunk","description":"the shop"},{"full_name":"acme/blog","default_branch":"main"}]}`))
		case "/github/repos/dockerfile":
			w.Write([]byte(`{"success":true,"data":` + f.dockerfile + `}`))
		case "/github/deployments/create":
			f.created = read(r)
			w.Write([]byte(`{"success":true,"data":{"deployment_id":"dep-new-1"}}`))
		case "/credits/quote":
			w.Write([]byte(f.quote))
		case "/credits/pay":
			f.paid = read(r)
			w.Write([]byte(`{"success":true,"data":{"external_id":"CRD-9","amount":25000,"new_balance":72250}}`))
		case "/github/deployments/details":
			st := f.statuses[0]
			if len(f.statuses) > 1 {
				f.statuses = f.statuses[1:]
			}
			w.Write([]byte(`{"success":true,"data":{"id":"dep-new-1","repo_full_name":"acme/shop","branch":"trunk","port":3000,"subdomain":"shop-dep-new.svc.dalang.io","status":"` + st + `","auto_deploy":true,"auto_renew":true,"expired_at":"2026-11-29T00:00:00Z"}}`))
		case "/github/deployments":
			w.Write([]byte(`{"success":true,"data":[{"id":"dep-new-1","repo_full_name":"acme/shop","branch":"trunk","subdomain":"shop-dep-new.svc.dalang.io","status":"running"},{"id":"dep-old-2","repo_full_name":"acme/blog","display_name":"Blog","status":"running"}]}`))
		case "/github/deployments/rebuild":
			f.rebuilt = read(r)
			w.Write([]byte(`{"success":true}`))
		case "/github/deployments/delete":
			f.deleted = read(r)
			w.Write([]byte(`{"success":true}`))
		case "/github/deployments/subscription-settings":
			f.settings = read(r)
			w.Write([]byte(`{"success":true}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	t.Setenv("DALANG_API_URL", srv.URL)
	if err := config.SaveCredentials(&config.Credentials{AccessToken: "tok", ExpiresAt: 4102444800}); err != nil {
		t.Fatal(err)
	}
	pollSleep = func(time.Duration) {}
	t.Cleanup(func() { pollSleep = time.Sleep })
	return f
}

func (f *fakeGitHubAPI) did(path string) bool {
	for _, c := range f.calls {
		if c == path {
			return true
		}
	}
	return false
}

func agentMode(t *testing.T) {
	t.Helper()
	resetGlobalFlags()
	t.Cleanup(resetGlobalFlags)
	jsonOutput, yesFlag = true, true
}

func TestAppCreate_DeploysPaysAndWaitsAndPrintsOneJSONDocument(t *testing.T) {
	agentMode(t)
	f := newFakeGitHubAPI(t)

	var err error
	out := captureStdout(t, func() {
		err = cmdApp([]string{"create", "--repo", "acme/shop", "--env", "PORT=3000", "--env", "MODE=prod", "--months", "1"})
	})
	if err != nil {
		t.Fatal(err)
	}

	var doc map[string]interface{}
	if jerr := json.Unmarshal([]byte(out), &doc); jerr != nil {
		t.Fatalf("stdout must be one JSON document, got %q: %v", out, jerr)
	}
	if doc["id"] != "dep-new-1" || doc["status"] != "running" || doc["url"] != "https://shop-dep-new.svc.dalang.io" || doc["branch"] != "trunk" {
		t.Errorf("result %v", doc)
	}
	// the default branch of the repository, the port from the Dockerfile's EXPOSE, push-to-rebuild on
	if f.created["branch"] != "trunk" || f.created["port"] != float64(3000) || f.created["auto_deploy"] != true {
		t.Errorf("created %v", f.created)
	}
	if env, _ := f.created["env_vars"].(map[string]interface{}); env["PORT"] != "3000" || env["MODE"] != "prod" {
		t.Errorf("env_vars %v", f.created["env_vars"])
	}
	// paid the quoted NET for this deployment's subscription
	meta, _ := f.paid["metadata"].(map[string]interface{})
	if f.paid["amount"] != float64(25000) || meta["deployment_id"] != "dep-new-1" || meta["type"] != "deployment_subscription" || meta["billing_period"] != float64(1) {
		t.Errorf("paid %v", f.paid)
	}
}

// A repository that cannot be built must not leave an app behind or cost anything.
func TestAppCreate_NoDockerfileCreatesAndChargesNothing(t *testing.T) {
	agentMode(t)
	f := newFakeGitHubAPI(t)
	f.dockerfile = `{"exists":false,"exposed_port":0}`

	err := cmdApp([]string{"create", "--repo", "acme/shop"})
	if err == nil || !strings.Contains(err.Error(), "no Dockerfile") || !strings.Contains(err.Error(), "Alpine") {
		t.Errorf("error: %v", err)
	}
	if f.did("/github/deployments/create") || f.did("/credits/pay") || f.did("/credits/quote") {
		t.Errorf("something was created or priced: %v", f.calls)
	}
}

func TestAppCreate_NeedsAPortWhenTheDockerfileExposesNone(t *testing.T) {
	agentMode(t)
	f := newFakeGitHubAPI(t)
	f.dockerfile = `{"exists":true,"exposed_port":0}`
	if err := cmdApp([]string{"create", "--repo", "acme/shop"}); err == nil || !strings.Contains(err.Error(), "--port") {
		t.Errorf("error: %v", err)
	}
	if f.did("/github/deployments/create") {
		t.Error("created an app without knowing its port")
	}
}

func TestAppCreate_RepositoryYouCannotSeeAndGitHubNotConnected(t *testing.T) {
	agentMode(t)
	f := newFakeGitHubAPI(t)
	if err := cmdApp([]string{"create", "--repo", "someone/else"}); err == nil || !strings.Contains(err.Error(), "not among the repositories") {
		t.Errorf("unknown repo: %v", err)
	}
	f.connected = false
	if err := cmdApp([]string{"create", "--repo", "acme/shop"}); err == nil || !strings.Contains(err.Error(), "GitHub is not connected") {
		t.Errorf("not connected: %v", err)
	}
	if f.did("/github/deployments/create") {
		t.Error("created an app without GitHub")
	}
}

// Payment is the step that can fail after the app exists: the error must say how to finish.
func TestAppCreate_FailedPaymentLeavesTheAppAndSaysHowToPay(t *testing.T) {
	agentMode(t)
	f := newFakeGitHubAPI(t)
	f.quote = `{"success":true,"data":{"net":25000,"ppn_rate":11,"ppn":2750,"gross":27750,"balance":1000,"sufficient":false,"shortfall":26750}}`

	err := cmdApp([]string{"create", "--repo", "acme/shop"})
	if err == nil || !strings.Contains(err.Error(), "not enough credits") || !strings.Contains(err.Error(), "dalang app extend dep-new-1") {
		t.Errorf("error: %v", err)
	}
	if f.paid != nil {
		t.Errorf("paid with too few credits: %v", f.paid)
	}
}

func TestAppCreate_MaxPriceStopsBeforePaying(t *testing.T) {
	agentMode(t)
	f := newFakeGitHubAPI(t)
	if err := cmdApp([]string{"create", "--repo", "acme/shop", "--max-price", "20000"}); err == nil || !strings.Contains(err.Error(), "above --max-price") {
		t.Errorf("error: %v", err)
	}
	if f.paid != nil {
		t.Errorf("paid above the limit: %v", f.paid)
	}
}

func TestAppCreate_FailedBuildIsAnErrorThatPointsAtTheBuildLog(t *testing.T) {
	agentMode(t)
	f := newFakeGitHubAPI(t)
	f.statuses = []string{"building", "failed"}

	var err error
	captureStdout(t, func() { err = cmdApp([]string{"create", "--repo", "acme/shop"}) })
	if err == nil || !strings.Contains(err.Error(), "dalang app logs dep-new-1 --build") {
		t.Errorf("error: %v", err)
	}
}

func TestAppCreate_RejectsBadInputBeforeTalkingToTheAPI(t *testing.T) {
	agentMode(t)
	f := newFakeGitHubAPI(t)
	for _, args := range [][]string{
		{"create"},
		{"create", "--repo", "noslash"},
		{"create", "--repo", "acme/shop", "--months", "6"},
		{"create", "--repo", "acme/shop", "--env", "BADENV"},
	} {
		if err := cmdApp(args); err == nil {
			t.Errorf("%v accepted", args)
		}
	}
	if len(f.calls) != 0 {
		t.Errorf("the API was called for invalid input: %v", f.calls)
	}
}

func TestMatchApp(t *testing.T) {
	apps := []api.Deployment{
		{ID: "dep-1", RepoFullName: "acme/shop", Status: "running"},
		{ID: "dep-2", RepoFullName: "acme/blog", DisplayName: "Blog", Status: "running"},
		{ID: "dep-3", RepoFullName: "acme/blog", Status: "stopped"},
	}
	if d, err := matchApp(apps, "shop"); err != nil || d.ID != "dep-1" {
		t.Errorf("by short repository name: %+v %v", d, err)
	}
	if d, err := matchApp(apps, "Blog"); err != nil || d.ID != "dep-2" {
		t.Errorf("display name wins: %+v %v", d, err)
	}
	if _, err := matchApp(apps, "acme/blog"); err == nil || !strings.Contains(err.Error(), "dep-2") || !strings.Contains(err.Error(), "dep-3") {
		t.Errorf("two apps of one repository must be refused with their ids: %v", err)
	}
	if _, err := matchApp(apps, "nothing"); err == nil {
		t.Error("an unknown name must be an error")
	}
}

func TestAppDelete_NeedsConfirmationAndThenDeletes(t *testing.T) {
	resetGlobalFlags()
	t.Cleanup(resetGlobalFlags)
	f := newFakeGitHubAPI(t)

	yesFlag = false // no terminal in a test: refused, nothing deleted
	if err := cmdApp([]string{"delete", "dep-new-1"}); err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Errorf("without --yes: %v", err)
	}
	if f.deleted != nil {
		t.Fatalf("deleted without confirmation: %v", f.deleted)
	}
	yesFlag = true
	captureStdout(t, func() {
		if err := cmdApp([]string{"delete", "dep-new-1"}); err != nil {
			t.Error(err)
		}
	})
	if f.deleted["deployment_id"] != "dep-new-1" {
		t.Errorf("deleted %v", f.deleted)
	}
}

func TestAppRebuildAndSettings(t *testing.T) {
	agentMode(t)
	f := newFakeGitHubAPI(t)

	captureStdout(t, func() {
		if err := cmdApp([]string{"rebuild", "shop"}); err != nil {
			t.Error(err)
		}
	})
	if f.rebuilt["deployment_id"] != "dep-new-1" {
		t.Errorf("rebuilt %v", f.rebuilt)
	}

	captureStdout(t, func() {
		if err := cmdApp([]string{"settings", "shop", "--auto-renew", "off", "--stop-at-period-end", "on"}); err != nil {
			t.Error(err)
		}
	})
	if f.settings["auto_renew"] != false || f.settings["cancel_at_period_end"] != true || f.settings["deployment_id"] != "dep-new-1" {
		t.Errorf("settings %v", f.settings)
	}
	if err := cmdApp([]string{"settings", "shop", "--auto-renew", "maybe"}); err == nil {
		t.Error("anything but on/off must be refused")
	}
}

func TestAppUnknownCommandAndHelp(t *testing.T) {
	resetGlobalFlags()
	t.Cleanup(resetGlobalFlags)
	if err := cmdApp([]string{"fly"}); err == nil || !strings.Contains(err.Error(), "unknown app command") {
		t.Errorf("unknown command: %v", err)
	}
	if err := cmdApp([]string{"--help"}); err != nil {
		t.Errorf("--help: %v", err)
	}
}
