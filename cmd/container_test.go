package cmd

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

type fakeContainersAPI struct {
	containers []string // JSON objects
	listCalls  int
	appearsAt  int // the new container shows up from this /list call on
	quote      string
	paid       map[string]interface{}
	action     map[string]interface{}
	envPosted  map[string]interface{}
	recreated  map[string]interface{}
	calls      []string
}

func newFakeContainersAPI(t *testing.T) *fakeContainersAPI {
	t.Helper()
	f := &fakeContainersAPI{
		quote:     `{"success":true,"data":{"net":50000,"ppn_rate":11,"ppn":5500,"gross":55500,"balance":100000,"sufficient":true,"shortfall":0}}`,
		appearsAt: 3,
		containers: []string{
			`{"id":"ctr-aaa11111","name":"svc-aaa11111","display_name":"orders-db","container_type":"postgresql","plan":"starter","status":"RUNNING"}`,
		},
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
		case "/containers/list":
			f.listCalls++
			list := append([]string{}, f.containers...)
			if f.paid != nil && f.listCalls >= f.appearsAt {
				name := f.paid["metadata"].(map[string]interface{})["service_name"].(string)
				list = append(list, `{"id":"ctr-new22222","name":"svc-new22222","display_name":"`+name+`","container_type":"redis","plan":"starter","status":"RUNNING"}`)
			}
			w.Write([]byte(`{"success":true,"data":{"containers":[` + strings.Join(list, ",") + `]}}`))
		case "/credits/quote":
			w.Write([]byte(f.quote))
		case "/credits/pay":
			f.paid = read(r)
			w.Write([]byte(`{"success":true,"data":{"external_id":"CRD-5","amount":50000,"new_balance":44500}}`))
		case "/containers/detail":
			// the API sends env_vars as a JSON string
			w.Write([]byte(`{"success":true,"data":{"container":{"id":"ctr-aaa11111","container_type":"postgresql","container_version":"18.6-alpine","plan":"starter","status":"RUNNING"},"detail":{"port":5432,"connection_string":"postgres://h:5432","env_vars":"{\"POSTGRES_PASSWORD\":\"old\",\"KEEP\":\"1\",\"DROP\":\"x\"}"}}}`))
		case "/containers/action":
			f.action = read(r)
			w.Write([]byte(`{"success":true}`))
		case "/containers/update-env":
			f.envPosted = read(r)
			w.Write([]byte(`{"success":true}`))
		case "/containers/recreate":
			f.recreated = read(r)
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

func TestContainerCreate_PaysTheQuotedNetAndReturnsTheNewContainer(t *testing.T) {
	agentMode(t)
	f := newFakeContainersAPI(t)

	var err error
	out := captureStdout(t, func() {
		err = cmdContainer([]string{"create", "--type", "redis", "--name", "cache-1", "--plan", "starter"})
	})
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]interface{}
	if jerr := json.Unmarshal([]byte(out), &doc); jerr != nil {
		t.Fatalf("stdout must be one JSON document, got %q", out)
	}
	if doc["id"] != "ctr-new22222" || doc["status"] != "RUNNING" || doc["paid_gross"] != float64(55500) {
		t.Errorf("result %v", doc)
	}
	meta, _ := f.paid["metadata"].(map[string]interface{})
	if f.paid["amount"] != float64(50000) || f.paid["product_type"] != "container" ||
		meta["container_type"] != "redis" || meta["container_version"] != "8.8.4-alpine" || meta["container_plan"] != "starter" || meta["service_name"] != "cache-1" || meta["product_type"] != "container" {
		t.Errorf("paid %v", f.paid)
	}
}

func TestContainerCreate_WhatEachTemplateNeedsIsAskedForBeforeAnythingIsPaid(t *testing.T) {
	agentMode(t)
	f := newFakeContainersAPI(t)
	for name, args := range map[string][]string{
		"postgres without its password": {"create", "--type", "postgresql"},
		"unknown type":                  {"create", "--type", "cobol"},
		"a plan nobody sells":           {"create", "--type", "redis", "--plan", "micro"},
		"wordpress without a database":  {"create", "--type", "wordpress"},
		"odoo without a postgres":       {"create", "--type", "odoo"},
		"create-mysql without password": {"create", "--type", "wordpress", "--create-mysql"},
		"no type at all":                {"create"},
		"a malformed --env":             {"create", "--type", "redis", "--env", "NOEQUALS"},
	} {
		if err := cmdContainer(args); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if f.paid != nil || len(f.calls) != 0 {
		t.Errorf("the API was used for invalid input: paid=%v calls=%v", f.paid, f.calls)
	}
}

func TestBuildContainerRequest_LinkedDatabases(t *testing.T) {
	r, err := buildContainerRequest("wordpress", "", "starter", "blog", nil, "", false, "", "", true, "pw", "mysql")
	if err != nil || r["create_linked_mysql"] != true || r["mysql_password"] != "pw" || r["mysql_type"] != "mysql" {
		t.Errorf("wordpress: %v %v", r, err)
	}
	r, err = buildContainerRequest("odoo", "", "basic", "erp", nil, "pg-123", false, "", "", false, "", "")
	if err != nil || r["linked_postgres_id"] != "pg-123" {
		t.Errorf("odoo with an existing postgres: %v %v", r, err)
	}
	r, err = buildContainerRequest("n8n", "", "starter", "flows", kvFlag{"N8N_BASIC_AUTH_PASSWORD": "x"}, "", false, "", "", false, "", "")
	if err != nil || r["linked_postgres_id"] != nil || r["create_linked_postgres"] != nil {
		t.Errorf("n8n's postgres is optional: %v %v", r, err)
	}
}

func TestContainerEnv_AChangeIsMergedBecauseTheAPIReplacesTheWholeMap(t *testing.T) {
	agentMode(t)
	f := newFakeContainersAPI(t)

	captureStdout(t, func() {
		if err := cmdContainer([]string{"env", "orders-db", "--set", "NEW=1", "--set", "POSTGRES_PASSWORD=fresh", "--unset", "DROP"}); err != nil {
			t.Error(err)
		}
	})
	env, _ := f.envPosted["env_vars"].(map[string]interface{})
	if f.envPosted["container_id"] != "ctr-aaa11111" || env["KEEP"] != "1" || env["NEW"] != "1" || env["POSTGRES_PASSWORD"] != "fresh" {
		t.Errorf("posted %v", f.envPosted)
	}
	if _, still := env["DROP"]; still {
		t.Errorf("a key that was unset was sent again: %v", env)
	}
}

func TestContainerEnv_WithoutChangesOnlyShows(t *testing.T) {
	agentMode(t)
	f := newFakeContainersAPI(t)
	out := captureStdout(t, func() {
		if err := cmdContainer([]string{"env", "orders-db"}); err != nil {
			t.Error(err)
		}
	})
	if f.envPosted != nil || !strings.Contains(out, `"KEEP": "1"`) {
		t.Errorf("posted=%v out=%q", f.envPosted, out)
	}
}

func TestContainerActionsAndRecreate(t *testing.T) {
	agentMode(t)
	f := newFakeContainersAPI(t)
	captureStdout(t, func() {
		if err := cmdContainer([]string{"restart", "orders-db"}); err != nil {
			t.Error(err)
		}
	})
	if f.action["container_id"] != "ctr-aaa11111" || f.action["action"] != "restart" {
		t.Errorf("action %v", f.action)
	}

	yesFlag = false
	if err := cmdContainer([]string{"recreate", "orders-db"}); err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Errorf("recreate without --yes: %v", err)
	}
	if f.recreated != nil {
		t.Fatal("recreated without confirmation")
	}
	yesFlag = true
	captureStdout(t, func() { cmdContainer([]string{"recreate", "orders-db"}) })
	if f.recreated["container_id"] != "ctr-aaa11111" {
		t.Errorf("recreated %v", f.recreated)
	}
}

func TestMatchContainer(t *testing.T) {
	list := []api.Container{
		{ID: "c1", Name: "svc-1", DisplayName: "db", Status: "RUNNING"},
		{ID: "c2", Name: "svc-2", DisplayName: "db", Status: "STOPPED"},
		{ID: "c3", Name: "svc-3", Status: "RUNNING"},
	}
	if c, err := matchContainer(list, "svc-3"); err != nil || c.ID != "c3" {
		t.Errorf("by instance name: %+v %v", c, err)
	}
	if _, err := matchContainer(list, "db"); err == nil || !strings.Contains(err.Error(), "c1") || !strings.Contains(err.Error(), "c2") {
		t.Errorf("a name shared by two containers: %v", err)
	}
	if _, err := matchContainer(list, "nope"); err == nil {
		t.Error("unknown name accepted")
	}
}

func TestContainerTemplatesAreTheDocumentedOnes(t *testing.T) {
	seen := map[string]bool{}
	for _, tpl := range containerTemplates {
		if tpl.Version == "" || seen[tpl.Type] {
			t.Errorf("template %q: empty version or duplicate", tpl.Type)
		}
		seen[tpl.Type] = true
	}
	if len(containerTemplates) != 20 {
		t.Errorf("%d templates, the wizard has 20 (update this copy when a template is added)", len(containerTemplates))
	}
	agentMode(t)
	out := captureStdout(t, func() { cmdContainer([]string{"templates"}) })
	if !strings.Contains(out, `"plans"`) || !strings.Contains(out, `"postgresql"`) {
		t.Errorf("templates --json: %q", out)
	}
}
