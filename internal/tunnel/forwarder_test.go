package tunnel

import (
	"context"
	"encoding/base64"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func mustForwarder(t *testing.T, base string) *Forwarder {
	t.Helper()
	f, err := NewForwarder(base, 5*time.Second)
	if err != nil {
		t.Fatalf("NewForwarder(%q): %v", base, err)
	}
	return f
}

func decodeBody(t *testing.T, res Result) string {
	t.Helper()
	b, err := base64.StdEncoding.DecodeString(res.Response.BodyB64)
	if err != nil {
		t.Fatalf("response body is not valid base64: %v", err)
	}
	return string(b)
}

func TestForwarderReplaysRequestFaithfully(t *testing.T) {
	var (
		gotMethod string
		gotURL    string
		gotHost   string
		gotHeader string
		gotBody   string
	)
	local := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotURL = r.URL.String()
		gotHost = r.Host
		gotHeader = r.Header.Get("X-Custom")
		body := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(body)
		gotBody = string(body)

		w.Header().Set("Content-Type", "application/json")
		w.Header().Add("Set-Cookie", "a=1")
		w.Header().Add("Set-Cookie", "b=2")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer local.Close()

	f := mustForwarder(t, local.URL)
	res := f.Do(context.Background(), Request{
		Type:    TypeRequest,
		ID:      "01J",
		Method:  http.MethodPost,
		Path:    "/api/items?page=2",
		Headers: map[string][]string{"X-Custom": {"yes"}, "Connection": {"keep-alive"}},
		BodyB64: base64.StdEncoding.EncodeToString([]byte("hello")),
	})

	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
	if gotMethod != http.MethodPost {
		t.Fatalf("method = %q, want POST", gotMethod)
	}
	if gotURL != "/api/items?page=2" {
		t.Fatalf("url = %q, want /api/items?page=2 (query must survive)", gotURL)
	}
	if gotHeader != "yes" {
		t.Fatalf("X-Custom = %q, want yes", gotHeader)
	}
	if gotBody != "hello" {
		t.Fatalf("body = %q, want hello", gotBody)
	}
	// The daemon rewrites Host to local_url's host; with no Host header supplied
	// the forwarder must still address the local server by its own name.
	if gotHost != strings.TrimPrefix(local.URL, "http://") {
		t.Fatalf("Host = %q, want %q", gotHost, strings.TrimPrefix(local.URL, "http://"))
	}
	if res.Response.Type != TypeResponse || res.Response.ID != "01J" {
		t.Fatalf("response envelope = %+v, want type=response id=01J", res.Response)
	}
	if res.Response.Status != http.StatusCreated {
		t.Fatalf("status = %d, want 201", res.Response.Status)
	}
	if got := decodeBody(t, res); got != `{"ok":true}` {
		t.Fatalf("body = %q", got)
	}
	if cookies := res.Response.Headers["Set-Cookie"]; len(cookies) != 2 {
		t.Fatalf("Set-Cookie = %v, want both values preserved", cookies)
	}
	if got := res.Response.Headers["Content-Length"]; len(got) != 1 || got[0] != "11" {
		t.Fatalf("Content-Length = %v, want [11] (recomputed from the buffered body)", got)
	}
}

func TestForwarderHonoursHostHeaderFromDaemon(t *testing.T) {
	var gotHost string
	local := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHost = r.Host
	}))
	defer local.Close()

	f := mustForwarder(t, local.URL)
	f.Do(context.Background(), Request{
		ID:      "1",
		Method:  http.MethodGet,
		Path:    "/",
		Headers: map[string][]string{"Host": {"app.internal"}},
	})

	if gotHost != "app.internal" {
		t.Fatalf("Host = %q, want app.internal", gotHost)
	}
}

// TestForwarderDoesNotFollowRedirects: the browser at the far end must receive
// the 302 the local server actually sent.
func TestForwarderDoesNotFollowRedirects(t *testing.T) {
	local := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/old" {
			http.Redirect(w, r, "/new", http.StatusFound)
			return
		}
		_, _ = w.Write([]byte("followed"))
	}))
	defer local.Close()

	res := mustForwarder(t, local.URL).Do(context.Background(), Request{ID: "1", Method: http.MethodGet, Path: "/old"})
	if res.Response.Status != http.StatusFound {
		t.Fatalf("status = %d, want 302 (redirect must be passed through, not followed)", res.Response.Status)
	}
	if loc := res.Response.Headers["Location"]; len(loc) != 1 || loc[0] != "/new" {
		t.Fatalf("Location = %v, want [/new]", loc)
	}
}

func TestForwarder502WhenLocalServerIsDown(t *testing.T) {
	// Bind and release a port so we have an address nothing is listening on.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()
	ln.Close()

	base, err := NormalizeLocalURL(addr)
	if err != nil {
		t.Fatalf("NormalizeLocalURL: %v", err)
	}
	f := mustForwarder(t, base)

	res := f.Do(context.Background(), Request{ID: "01J", Method: http.MethodGet, Path: "/"})

	if res.Err == nil {
		t.Fatal("expected an error for an unreachable local server")
	}
	if res.Response.ID != "01J" {
		t.Fatalf("id = %q, want 01J — an unanswered id hangs the browser", res.Response.ID)
	}
	if res.Response.Status != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", res.Response.Status)
	}
	body := decodeBody(t, res)
	if !strings.Contains(body, base) {
		t.Fatalf("502 body must name the local URL that failed, got: %s", body)
	}
}

func TestForwarder413WhenLocalResponseExceedsLimit(t *testing.T) {
	local := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(make([]byte, 4096))
	}))
	defer local.Close()

	f := mustForwarder(t, local.URL)
	f.SetMaxBodyBytes(1024)

	res := f.Do(context.Background(), Request{ID: "01J", Method: http.MethodGet, Path: "/"})
	if res.Response.Status != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413 for a body over the tunnel limit", res.Response.Status)
	}
	if res.Response.ID != "01J" {
		t.Fatalf("id = %q, want 01J", res.Response.ID)
	}
}

func TestForwarderStripsHopByHopResponseHeaders(t *testing.T) {
	local := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Keep", "me")
		_, _ = w.Write([]byte("ok"))
	}))
	defer local.Close()

	res := mustForwarder(t, local.URL).Do(context.Background(), Request{ID: "1", Method: http.MethodGet, Path: "/"})
	if _, ok := res.Response.Headers["Connection"]; ok {
		t.Fatal("hop-by-hop header Connection must not be forwarded")
	}
	if got := res.Response.Headers["X-Keep"]; len(got) != 1 || got[0] != "me" {
		t.Fatalf("X-Keep = %v, want [me]", got)
	}
}

func TestForwarderAppendsPathToBasePath(t *testing.T) {
	var gotPath string
	local := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
	}))
	defer local.Close()

	f := mustForwarder(t, local.URL+"/base")
	f.Do(context.Background(), Request{ID: "1", Method: http.MethodGet, Path: "/thing"})
	if gotPath != "/base/thing" {
		t.Fatalf("path = %q, want /base/thing", gotPath)
	}
}

// TestForwarder504WhenLocalServerHangs: the protocol pins the local timeout at
// 60s and says the CLI answers that id with a 504 — distinct from the 502 that
// means "nothing is listening", because the two send the user to different
// places.
func TestForwarder504WhenLocalServerHangs(t *testing.T) {
	release := make(chan struct{})
	local := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
	}))
	defer func() {
		close(release)
		local.Close()
	}()

	f, err := NewForwarder(local.URL, 50*time.Millisecond)
	if err != nil {
		t.Fatalf("NewForwarder: %v", err)
	}

	res := f.Do(context.Background(), Request{ID: "01JSLOW", Method: http.MethodGet, Path: "/"})
	if res.Response.Status != http.StatusGatewayTimeout {
		t.Fatalf("status = %d, want 504 for a local server that never answers", res.Response.Status)
	}
	if res.Response.ID != "01JSLOW" {
		t.Fatalf("id = %q, want 01JSLOW", res.Response.ID)
	}
	if body := decodeBody(t, res); !strings.Contains(body, local.URL) {
		t.Fatalf("504 body should name the local URL, got: %s", body)
	}
}

func TestDefaultLocalTimeoutMatchesTheProtocol(t *testing.T) {
	if DefaultLocalTimeout != 60*time.Second {
		t.Fatalf("DefaultLocalTimeout = %s, want 60s (pinned by PROTOCOL.md's timing table)", DefaultLocalTimeout)
	}
}
