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
	envPosted  map[string]interface{}
	portPosted map[string]interface{}
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
			w.Write([]byte(`{"success":true,"data":{"id":"dep-new-1","repo_full_name":"acme/shop","branch":"trunk","port":3000,"subdomain":"shop-dep-new.svc.dalang.io","status":"` + st + `","auto_deploy":true,"auto_renew":true,"expired_at":"2026-11-29T00:00:00Z","env_vars":{"KEEP":"1","DROP":"x","API_KEY":"old"}}}`))
		case "/github/deployments":
			w.Write([]byte(`{"success":true,"data":[{"id":"dep-new-1","repo_full_name":"acme/shop","branch":"trunk","subdomain":"shop-dep-new.svc.dalang.io","status":"running"},{"id":"dep-old-2","repo_full_name":"acme/blog","display_name":"Blog","status":"running"}]}`))
		case "/github/deployments/rebuild":
			f.rebuilt = read(r)
			w.Write([]byte(`{"success":true}`))
		case "/github/deployments/delete":
			f.deleted = read(r)
			w.Write([]byte(`{"success":true}`))
		case "/github/deployments/update-port":
			f.portPosted = read(r)
			w.Write([]byte(`{"success":true,"message":"Port saved"}`))
		case "/github/deployments/update-env":
			f.envPosted = read(r)
			w.Write([]byte(`{"success":true,"message":"Environment saved"}`))
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

func indexOf(calls []string, path string) int {
	for i, c := range calls {
		if c == path {
			return i
		}
	}
	return -1
}

// The API replaces the whole map, so a change must be merged into the stored one.
func TestAppEnv_SetAndUnsetAreMergedIntoTheStoredMap(t *testing.T) {
	agentMode(t)
	f := newFakeGitHubAPI(t)

	captureStdout(t, func() {
		if err := cmdApp([]string{"env", "shop", "--set", "API_KEY=new", "--set", "ADDED=1", "--unset", "DROP"}); err != nil {
			t.Error(err)
		}
	})
	env, _ := f.envPosted["env_vars"].(map[string]interface{})
	if f.envPosted["deployment_id"] != "dep-new-1" || env["KEEP"] != "1" || env["API_KEY"] != "new" || env["ADDED"] != "1" {
		t.Errorf("posted %v", f.envPosted)
	}
	if _, still := env["DROP"]; still {
		t.Errorf("an unset variable was sent again: %v", env)
	}
	if f.did("/github/deployments/rebuild") {
		t.Error("rebuilt without --rebuild: saved variables apply from the NEXT build, and the customer decides when")
	}
}

func TestAppEnv_RebuildSavesFirstThenBuildsThenWaits(t *testing.T) {
	agentMode(t)
	f := newFakeGitHubAPI(t)
	var err error
	out := captureStdout(t, func() { err = cmdApp([]string{"env", "shop", "--set", "FEATURE=on", "--rebuild"}) })
	if err != nil {
		t.Fatal(err)
	}
	if i, j := indexOf(f.calls, "/github/deployments/update-env"), indexOf(f.calls, "/github/deployments/rebuild"); i < 0 || j < 0 || i > j {
		t.Errorf("order: save at %d, rebuild at %d (%v): the build must run with what was just saved", i, j, f.calls)
	}
	var doc map[string]interface{}
	if jerr := json.Unmarshal([]byte(out), &doc); jerr != nil || doc["status"] != "running" {
		t.Errorf("one JSON document with the final status, got %q (%v)", out, jerr)
	}
}

func TestAppEnv_RebuildAloneBuildsWithoutSaving(t *testing.T) {
	agentMode(t)
	f := newFakeGitHubAPI(t)
	captureStdout(t, func() {
		if err := cmdApp([]string{"env", "shop", "--rebuild"}); err != nil {
			t.Error(err)
		}
	})
	if f.envPosted != nil || f.rebuilt == nil {
		t.Errorf("saved=%v rebuilt=%v", f.envPosted, f.rebuilt)
	}
}

func TestAppEnv_ListHidesValuesUnlessAsked(t *testing.T) {
	resetGlobalFlags()
	t.Cleanup(resetGlobalFlags)
	newFakeGitHubAPI(t)

	hidden := captureStdout(t, func() { cmdApp([]string{"env", "shop"}) })
	if !strings.Contains(hidden, "API_KEY=********") || strings.Contains(hidden, "old") {
		t.Errorf("values must be hidden by default: %q", hidden)
	}
	shown := captureStdout(t, func() { cmdApp([]string{"env", "shop", "--reveal"}) })
	if !strings.Contains(shown, "API_KEY=old") {
		t.Errorf("--reveal: %q", shown)
	}
	jsonOutput = true
	asJSON := captureStdout(t, func() { cmdApp([]string{"env", "shop"}) })
	if !strings.Contains(asJSON, `"API_KEY": "old"`) {
		t.Errorf("--json is for the owner's own tooling and carries the values: %q", asJSON)
	}
}

func TestAppEnv_BadVariableNameIsRefusedBeforeTheAPIIsUsed(t *testing.T) {
	agentMode(t)
	f := newFakeGitHubAPI(t)
	for _, args := range [][]string{{"env", "shop", "--set", "A.B=1"}, {"env", "shop", "--set", "NOEQUALS"}, {"env"}} {
		if err := cmdApp(args); err == nil {
			t.Errorf("%v accepted", args)
		}
	}
	if len(f.calls) != 0 || f.envPosted != nil {
		t.Errorf("the API was used for invalid input: %v", f.calls)
	}
}

func TestAppPort_ChangesItAndAppliesFromTheNextBuild(t *testing.T) {
	agentMode(t)
	f := newFakeGitHubAPI(t)
	captureStdout(t, func() {
		if err := cmdApp([]string{"port", "shop", "8000"}); err != nil {
			t.Error(err)
		}
	})
	if f.portPosted["deployment_id"] != "dep-new-1" || f.portPosted["port"] != float64(8000) {
		t.Errorf("posted %v", f.portPosted)
	}
	if f.did("/github/deployments/rebuild") {
		t.Error("rebuilt without --rebuild: a port applies from the next build, and the customer decides when")
	}
}

func TestAppPort_RebuildSavesThenBuilds(t *testing.T) {
	agentMode(t)
	f := newFakeGitHubAPI(t)
	captureStdout(t, func() {
		if err := cmdApp([]string{"port", "shop", "8000", "--rebuild"}); err != nil {
			t.Error(err)
		}
	})
	if i, j := indexOf(f.calls, "/github/deployments/update-port"), indexOf(f.calls, "/github/deployments/rebuild"); i < 0 || j < 0 || i > j {
		t.Errorf("order: save at %d, rebuild at %d: the build must use the port that was just saved", i, j)
	}
}

func TestAppPort_WithoutANumberOnlyShowsIt(t *testing.T) {
	agentMode(t)
	f := newFakeGitHubAPI(t)
	out := captureStdout(t, func() { cmdApp([]string{"port", "shop"}) })
	if f.portPosted != nil || !strings.Contains(out, `"port": 3000`) {
		t.Errorf("posted=%v out=%q", f.portPosted, out)
	}
}

func TestAppPort_RefusesWhatIsNotAPortBeforeTheAPIIsUsed(t *testing.T) {
	agentMode(t)
	f := newFakeGitHubAPI(t)
	for _, p := range []string{"0", "65536", "abc", "-5", "80.5"} {
		if err := cmdApp([]string{"port", "shop", p}); err == nil {
			t.Errorf("port %q accepted", p)
		}
	}
	if len(f.calls) != 0 {
		t.Errorf("the API was used for an invalid port: %v", f.calls)
	}
}
