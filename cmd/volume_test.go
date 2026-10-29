package cmd

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dalang-io/dalang-cli/internal/config"
)

type fakeVolumesAPI struct {
	posted map[string]map[string]interface{} // path -> body
}

func newFakeVolumesAPI(t *testing.T) *fakeVolumesAPI {
	t.Helper()
	f := &fakeVolumesAPI{posted: map[string]map[string]interface{}{}}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/volumes/list":
			w.Write([]byte(`{"success":true,"data":[{"id":"vol-aaa","name":"data","size_gb":10,"status":"available"},{"id":"vol-bbb","name":"logs","size_gb":5,"status":"attached","future_field":"kept"}]}`))
		case r.URL.Path == "/vps/list":
			w.Write([]byte(`{"success":true,"data":[{"id":"vps-1","name":"binus","status":"RUNNING"}]}`))
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/volumes/"):
			m := map[string]interface{}{}
			b, _ := io.ReadAll(r.Body)
			json.Unmarshal(b, &m)
			f.posted[r.URL.Path] = m
			w.Write([]byte(`{"success":true,"data":{"volume_id":"vol-new","status":"creating"}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	t.Setenv("DALANG_API_URL", srv.URL)
	if err := config.SaveCredentials(&config.Credentials{AccessToken: "tok", ExpiresAt: 4102444800}); err != nil {
		t.Fatal(err)
	}
	return f
}

func TestVolumeCreate_ChargesOnlyWithConfirmationAndSendsNoPrice(t *testing.T) {
	resetGlobalFlags()
	t.Cleanup(resetGlobalFlags)
	f := newFakeVolumesAPI(t)

	yesFlag = false // an agent that forgot --yes: refused, nothing sent
	if err := cmdVolume([]string{"create", "--name", "data2", "--size", "10"}); err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Errorf("without --yes: %v", err)
	}
	if len(f.posted) != 0 {
		t.Fatalf("sent without confirmation: %v", f.posted)
	}

	jsonOutput, yesFlag = true, true
	out := captureStdout(t, func() {
		if err := cmdVolume([]string{"create", "--name", "data2", "--size", "10"}); err != nil {
			t.Error(err)
		}
	})
	body := f.posted["/volumes/create"]
	if body["name"] != "data2" || body["size_gb"] != float64(10) || body["pay_method"] != "credits" {
		t.Errorf("sent %v", body)
	}
	if _, hasPrice := body["amount"]; hasPrice {
		t.Error("the CLI sent an amount: the API prices a volume")
	}
	if !strings.Contains(out, `"volume_id": "vol-new"`) && !strings.Contains(out, `"volume_id":"vol-new"`) {
		t.Errorf("--json must pass the API's document through: %q", out)
	}
}

func TestVolumeCreate_RejectsBadInput(t *testing.T) {
	resetGlobalFlags()
	t.Cleanup(resetGlobalFlags)
	yesFlag = true
	f := newFakeVolumesAPI(t)
	for _, args := range [][]string{{"create"}, {"create", "--name", "x"}, {"create", "--size", "5"}, {"create", "--name", "x", "--size", "-3"}} {
		if err := cmdVolume(args); err == nil {
			t.Errorf("%v accepted", args)
		}
	}
	if len(f.posted) != 0 {
		t.Errorf("sent for invalid input: %v", f.posted)
	}
}

func TestVolumeAttachDetachResizeDelete(t *testing.T) {
	resetGlobalFlags()
	t.Cleanup(resetGlobalFlags)
	jsonOutput, yesFlag = true, true
	f := newFakeVolumesAPI(t)

	captureStdout(t, func() {
		for _, args := range [][]string{
			{"attach", "data", "--vps", "binus"},
			{"detach", "logs"},
			{"resize", "vol-aaa", "--size", "20"},
			{"delete", "DATA"}, // case-insensitive when there is no exact match
		} {
			if err := cmdVolume(args); err != nil {
				t.Errorf("%v: %v", args, err)
			}
		}
	})
	if b := f.posted["/volumes/attach"]; b["volume_id"] != "vol-aaa" || b["vps_id"] != "vps-1" {
		t.Errorf("attach %v", b)
	}
	if b := f.posted["/volumes/detach"]; b["volume_id"] != "vol-bbb" {
		t.Errorf("detach %v", b)
	}
	if b := f.posted["/volumes/resize"]; b["volume_id"] != "vol-aaa" || b["new_size_gb"] != float64(20) {
		t.Errorf("resize %v", b)
	}
	if b := f.posted["/volumes/delete"]; b["volume_id"] != "vol-aaa" {
		t.Errorf("delete %v", b)
	}
}

func TestVolumeDeleteNeedsConfirmation(t *testing.T) {
	resetGlobalFlags()
	t.Cleanup(resetGlobalFlags)
	yesFlag = false
	f := newFakeVolumesAPI(t)
	if err := cmdVolume([]string{"delete", "data"}); err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Errorf("without --yes: %v", err)
	}
	if len(f.posted) != 0 {
		t.Errorf("deleted without confirmation: %v", f.posted)
	}
}

func TestVolumeListKeepsFieldsTheCLIDoesNotKnow(t *testing.T) {
	resetGlobalFlags()
	t.Cleanup(resetGlobalFlags)
	jsonOutput = true
	newFakeVolumesAPI(t)
	out := captureStdout(t, func() { cmdVolume([]string{"list"}) })
	if !strings.Contains(out, "future_field") {
		t.Errorf("an agent loses what the API adds: %q", out)
	}
}

func TestMatchVolume(t *testing.T) {
	vols := []volumeRec{{"id": "v1", "name": "data"}, {"id": "v2", "name": "data"}, {"id": "v3", "name": "logs"}}
	if v, err := matchVolume(vols, "v3"); err != nil || v.str("name") != "logs" {
		t.Errorf("by id: %v %v", v, err)
	}
	if _, err := matchVolume(vols, "data"); err == nil || !strings.Contains(err.Error(), "v1") || !strings.Contains(err.Error(), "v2") {
		t.Errorf("a shared name: %v", err)
	}
	if _, err := matchVolume(vols, "zzz"); err == nil {
		t.Error("unknown volume accepted")
	}
}
