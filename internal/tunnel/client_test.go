package tunnel

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// The protocol is the thing most likely to be wrong, so these tests drive a
// real WebSocket server speaking the frames from PROTOCOL.md rather than
// mocking the transport away.

type fakeTunnel struct {
	wsURL string
	conns chan *serverConn
	srv   *httptest.Server
}

type serverConn struct {
	conn *websocket.Conn
	req  *http.Request
	// hello is the first frame the client sent, already decoded.
	hello Hello
	done  chan struct{}
}

func newFakeTunnel(t *testing.T) *fakeTunnel {
	t.Helper()

	ft := &fakeTunnel{conns: make(chan *serverConn, 8)}
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}

	ft.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		_, data, err := c.ReadMessage()
		if err != nil {
			c.Close()
			return
		}
		var h Hello
		if err := json.Unmarshal(data, &h); err != nil {
			c.Close()
			return
		}
		sc := &serverConn{conn: c, req: r, hello: h, done: make(chan struct{})}
		ft.conns <- sc
		<-sc.done // the test owns this connection until it says otherwise
		c.Close()
	}))
	t.Cleanup(func() {
		ft.srv.Close()
	})

	ft.wsURL = "ws" + strings.TrimPrefix(ft.srv.URL, "http") + "/_tunnel/connect"
	return ft
}

// accept waits for the next client connection (the hello frame is already read).
func (ft *fakeTunnel) accept(t *testing.T) *serverConn {
	t.Helper()
	select {
	case sc := <-ft.conns:
		t.Cleanup(sc.finish)
		return sc
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the CLI to connect")
		return nil
	}
}

func (sc *serverConn) finish() {
	select {
	case <-sc.done:
	default:
		close(sc.done)
	}
}

func (sc *serverConn) send(t *testing.T, frame any) {
	t.Helper()
	data, err := json.Marshal(frame)
	if err != nil {
		t.Fatalf("marshal frame: %v", err)
	}
	if err := sc.conn.WriteMessage(websocket.TextMessage, data); err != nil {
		t.Fatalf("write frame: %v", err)
	}
}

// readFrameOfType reads until a frame of the given type arrives, so an
// interleaved keepalive never breaks an assertion.
func (sc *serverConn) readFrameOfType(t *testing.T, want string, into any) {
	t.Helper()
	_ = sc.conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	for {
		_, data, err := sc.conn.ReadMessage()
		if err != nil {
			t.Fatalf("waiting for a %q frame: %v", want, err)
		}
		var env envelope
		if err := json.Unmarshal(data, &env); err != nil {
			t.Fatalf("unparseable frame from client: %v", err)
		}
		if env.Type != want {
			continue
		}
		if err := json.Unmarshal(data, into); err != nil {
			t.Fatalf("decoding %q frame: %v", want, err)
		}
		return
	}
}

func newTestClient(t *testing.T, ft *fakeTunnel, localURL string, opts Options) (*Client, Events) {
	t.Helper()
	opts.ServerURL = ft.wsURL
	opts.LocalURL = localURL
	if opts.ClientName == "" {
		opts.ClientName = "dalang-cli/test"
	}
	c, err := New(opts)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c, opts.Events
}

// TestClientEndToEnd walks the whole happy path: hello → assigned → request →
// response → shutdown.
func TestClientEndToEnd(t *testing.T) {
	local := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("hi " + r.URL.String()))
	}))
	defer local.Close()

	ft := newFakeTunnel(t)
	assigned := make(chan Assigned, 1)
	logged := make(chan Request, 4)

	client, _ := newTestClient(t, ft, local.URL, Options{
		Token:          "secret-token",
		RequestedLabel: "kucing-makan-ikan",
		ReclaimToken:   "cHJldmlvdXMtdG9rZW4",
		Events: Events{
			OnAssigned: func(a Assigned) { assigned <- a },
			OnRequest:  func(req Request, _ Result) { logged <- req },
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runErr := make(chan error, 1)
	go func() { runErr <- client.Run(ctx) }()

	sc := ft.accept(t)

	// --- hello ---
	if sc.hello.Type != TypeHello || sc.hello.Version != ProtocolVersion {
		t.Fatalf("hello = %+v, want type=hello version=%d", sc.hello, ProtocolVersion)
	}
	if sc.hello.LocalURL != local.URL {
		t.Fatalf("hello.local_url = %q, want %q", sc.hello.LocalURL, local.URL)
	}
	if sc.hello.RequestedLabel != "kucing-makan-ikan" {
		t.Fatalf("hello.requested_label = %q", sc.hello.RequestedLabel)
	}
	if sc.hello.Token != "secret-token" {
		t.Fatalf("hello.token = %q, want the bearer token", sc.hello.Token)
	}
	// A label without its reclaim token is always refused by the daemon, so the
	// two must travel together.
	if sc.hello.ReclaimToken != "cHJldmlvdXMtdG9rZW4" {
		t.Fatalf("hello.reclaim_token = %q, want the stored token alongside the label", sc.hello.ReclaimToken)
	}
	if !strings.HasPrefix(sc.hello.Client, "dalang-cli/") {
		t.Fatalf("hello.client = %q, want a dalang-cli/<version> string", sc.hello.Client)
	}

	// --- assigned ---
	sc.send(t, Assigned{
		Type:                 TypeAssigned,
		Label:                "kucing-makan-ikan",
		URL:                  "https://kucing-makan-ikan." + Domain,
		ExpiresAt:            time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339),
		MaxBodyBytes:         10 << 20,
		ReclaimToken:         "cmVjbGFpbS0x",
		ReclaimWindowSeconds: 21600,
	})
	select {
	case a := <-assigned:
		if a.URL != "https://kucing-makan-ikan."+Domain {
			t.Fatalf("assigned url = %q", a.URL)
		}
		// A duration, not a deadline: the window runs from the close, which has
		// not happened when this frame is sent.
		if a.ReclaimToken != "cmVjbGFpbS0x" || a.ReclaimWindowSeconds != 21600 {
			t.Fatalf("assigned did not carry the reclaim ticket: %+v", a)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("OnAssigned was never called — the public URL would never be printed")
	}

	// --- request / response ---
	sc.send(t, Request{
		Type:     TypeRequest,
		ID:       "01JABCDEF",
		Method:   http.MethodGet,
		Path:     "/items?page=2",
		Headers:  map[string][]string{"Accept": {"text/plain"}},
		BodyB64:  "",
		RemoteIP: "203.0.113.9",
	})

	var resp Response
	sc.readFrameOfType(t, TypeResponse, &resp)
	if resp.ID != "01JABCDEF" {
		t.Fatalf("response id = %q, want the request id echoed back", resp.ID)
	}
	if resp.Status != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.Status)
	}
	body, err := base64.StdEncoding.DecodeString(resp.BodyB64)
	if err != nil {
		t.Fatalf("body_b64 is not base64: %v", err)
	}
	if string(body) != "hi /items?page=2" {
		t.Fatalf("body = %q, want the local server's answer including the query", body)
	}
	select {
	case req := <-logged:
		if req.Method != http.MethodGet || req.Path != "/items?page=2" {
			t.Fatalf("OnRequest got %+v", req)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("OnRequest was never called — no request log line would be printed")
	}

	// --- shutdown ---
	sc.send(t, Shutdown{Type: TypeShutdown, Reason: ReasonExpired, Message: "2 hour limit reached"})

	select {
	case err := <-runErr:
		var fatal *FatalError
		if !errors.As(err, &fatal) {
			t.Fatalf("Run returned %v, want a FatalError", err)
		}
		if fatal.Code != ReasonExpired {
			t.Fatalf("fatal code = %q, want %q", fatal.Code, ReasonExpired)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after a shutdown frame")
	}
}

// TestClientAnswersWith502WhenLocalServerIsDown is the "never drop a frame"
// guarantee, exercised through the real frame loop.
func TestClientAnswersWith502WhenLocalServerIsDown(t *testing.T) {
	local := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	localURL := local.URL
	local.Close() // nothing is listening there any more

	ft := newFakeTunnel(t)
	client, _ := newTestClient(t, ft, localURL, Options{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runErr := make(chan error, 1)
	go func() { runErr <- client.Run(ctx) }()

	sc := ft.accept(t)
	sc.send(t, Assigned{Type: TypeAssigned, Label: "a-b-c", URL: "https://a-b-c." + Domain})
	sc.send(t, Request{Type: TypeRequest, ID: "deadbeef", Method: http.MethodGet, Path: "/"})

	var resp Response
	sc.readFrameOfType(t, TypeResponse, &resp)
	if resp.ID != "deadbeef" {
		t.Fatalf("response id = %q, want deadbeef", resp.ID)
	}
	if resp.Status != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", resp.Status)
	}
	body, _ := base64.StdEncoding.DecodeString(resp.BodyB64)
	if !strings.Contains(string(body), localURL) {
		t.Fatalf("502 body should name the failing local URL, got %q", body)
	}

	cancel()
	<-runErr
}

func TestClientRepliesToServerPing(t *testing.T) {
	local := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer local.Close()

	ft := newFakeTunnel(t)
	client, _ := newTestClient(t, ft, local.URL, Options{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runErr := make(chan error, 1)
	go func() { runErr <- client.Run(ctx) }()

	sc := ft.accept(t)
	sc.send(t, Assigned{Type: TypeAssigned, Label: "a-b-c", URL: "https://a-b-c." + Domain})
	sc.send(t, PingFrame{Type: TypePing, T: 1758441600})

	var pong PingFrame
	sc.readFrameOfType(t, TypePong, &pong)
	if pong.T != 1758441600 {
		t.Fatalf("pong.t = %d, want the ping's t echoed back", pong.T)
	}

	cancel()
	<-runErr
}

// TestClientSendsKeepalivePings pins the keepalive that stops Cloudflare from
// dropping an idle tunnel at ~100s.
func TestClientSendsKeepalivePings(t *testing.T) {
	local := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer local.Close()

	ft := newFakeTunnel(t)
	client, _ := newTestClient(t, ft, local.URL, Options{PingInterval: 20 * time.Millisecond})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runErr := make(chan error, 1)
	go func() { runErr <- client.Run(ctx) }()

	sc := ft.accept(t)
	sc.send(t, Assigned{Type: TypeAssigned, Label: "a-b-c", URL: "https://a-b-c." + Domain})

	var ping PingFrame
	sc.readFrameOfType(t, TypePing, &ping)
	if ping.T == 0 {
		t.Fatal("ping.t should carry a unix timestamp")
	}

	cancel()
	<-runErr
}

// TestClientReconnectsWithSameLabelAfterMissedPongs is the protocol's
// "three missed pongs and the CLI reconnects with the same requested_label":
// the user's public URL has to survive a dead connection.
func TestClientReconnectsWithSameLabelAfterMissedPongs(t *testing.T) {
	local := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer local.Close()

	ft := newFakeTunnel(t)
	client, _ := newTestClient(t, ft, local.URL, Options{
		// No label requested up front: the label must be remembered from the
		// first `assigned`, not merely echoed from the options.
		PingInterval:   20 * time.Millisecond,
		MaxMissedPongs: 3,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runErr := make(chan error, 1)
	go func() { runErr <- client.Run(ctx) }()

	first := ft.accept(t)
	if first.hello.RequestedLabel != "" {
		t.Fatalf("first hello.requested_label = %q, want empty", first.hello.RequestedLabel)
	}
	first.send(t, Assigned{
		Type: TypeAssigned, Label: "kucing-makan-ikan",
		URL:          "https://kucing-makan-ikan." + Domain,
		ReclaimToken: "dG9rZW4tZnJvbS1hc3NpZ25lZA",
	})
	// Deliberately never pong.

	second := ft.accept(t)
	if second.hello.RequestedLabel != "kucing-makan-ikan" {
		t.Fatalf("reconnect hello.requested_label = %q, want the assigned label so the URL survives", second.hello.RequestedLabel)
	}
	// The label went to `reserved` when the socket dropped; only the token from
	// the most recent `assigned` gets it back.
	if second.hello.ReclaimToken != "dG9rZW4tZnJvbS1hc3NpZ25lZA" {
		t.Fatalf("reconnect hello.reclaim_token = %q, want the token from the last assigned frame", second.hello.ReclaimToken)
	}

	second.send(t, ErrorFrame{Type: TypeError, Code: CodeLabelTaken, Message: "nope"})
	select {
	case err := <-runErr:
		var fatal *FatalError
		if !errors.As(err, &fatal) || fatal.Code != CodeLabelTaken {
			t.Fatalf("Run returned %v, want a label_taken FatalError", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after an error frame")
	}
}

// TestClientReconnectsOnServerRestart: `server_restart` is the one shutdown
// reason that is not fatal.
func TestClientReconnectsOnServerRestart(t *testing.T) {
	local := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer local.Close()

	ft := newFakeTunnel(t)
	shutdowns := make(chan Shutdown, 2)
	client, _ := newTestClient(t, ft, local.URL, Options{
		Events: Events{OnShutdown: func(sd Shutdown) { shutdowns <- sd }},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runErr := make(chan error, 1)
	go func() { runErr <- client.Run(ctx) }()

	first := ft.accept(t)
	first.send(t, Assigned{
		Type: TypeAssigned, Label: "kucing-makan-ikan",
		URL:          "https://kucing-makan-ikan." + Domain,
		ReclaimToken: "cmVzdGFydC10b2tlbg",
	})
	first.send(t, Shutdown{Type: TypeShutdown, Reason: ReasonServerRestart, Message: "back in a moment"})

	select {
	case sd := <-shutdowns:
		if sd.Reason != ReasonServerRestart {
			t.Fatalf("shutdown reason = %q", sd.Reason)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("OnShutdown was never called")
	}

	second := ft.accept(t)
	if second.hello.RequestedLabel != "kucing-makan-ikan" {
		t.Fatalf("hello.requested_label after restart = %q, want the same label", second.hello.RequestedLabel)
	}
	if second.hello.ReclaimToken != "cmVzdGFydC10b2tlbg" {
		t.Fatalf("hello.reclaim_token after restart = %q, want the reissued token", second.hello.ReclaimToken)
	}

	cancel()
	select {
	case err := <-runErr:
		if err != nil {
			t.Fatalf("Run after Ctrl-C returned %v, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after cancellation")
	}
}

// TestCredentialsNeverLandInTheURL guards the reason the terminal client uses a
// header: this connection is proxied by Cloudflare, and a ?token= would be
// written to its access logs.
func TestCredentialsNeverLandInTheURL(t *testing.T) {
	local := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer local.Close()

	ft := newFakeTunnel(t)
	client, _ := newTestClient(t, ft, local.URL, Options{Token: "secret-token"})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runErr := make(chan error, 1)
	go func() { runErr <- client.Run(ctx) }()

	sc := ft.accept(t)
	if q := sc.req.URL.RawQuery; strings.Contains(q, "secret-token") {
		t.Fatalf("token leaked into the handshake query string: %q", q)
	}
	if got := sc.req.Header.Get("Authorization"); got != "Bearer secret-token" {
		t.Fatalf("Authorization header = %q, want a bearer header on the handshake", got)
	}

	cancel()
	<-runErr
}

// TestClientShutsDownCleanly: Ctrl-C must tell the server, not just vanish.
func TestClientShutsDownCleanly(t *testing.T) {
	local := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer local.Close()

	ft := newFakeTunnel(t)
	client, _ := newTestClient(t, ft, local.URL, Options{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runErr := make(chan error, 1)
	go func() { runErr <- client.Run(ctx) }()

	sc := ft.accept(t)
	sc.send(t, Assigned{Type: TypeAssigned, Label: "a-b-c", URL: "https://a-b-c." + Domain})

	closed := make(chan int, 1)
	sc.conn.SetCloseHandler(func(code int, text string) error {
		closed <- code
		return nil
	})

	cancel()

	go func() {
		_ = sc.conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		for {
			if _, _, err := sc.conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	select {
	case code := <-closed:
		if code != websocket.CloseNormalClosure {
			t.Fatalf("close code = %d, want %d (normal closure)", code, websocket.CloseNormalClosure)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no WebSocket close frame was sent — the server would hold the label until it times out")
	}

	select {
	case err := <-runErr:
		if err != nil {
			t.Fatalf("Run returned %v after Ctrl-C, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after cancellation")
	}
}

func TestNewRejectsBadOptions(t *testing.T) {
	if _, err := New(Options{}); err == nil {
		t.Fatal("expected an error when LocalURL is missing")
	}
	if _, err := New(Options{LocalURL: "http://localhost:8000", RequestedLabel: "not_a_label"}); err == nil {
		t.Fatal("expected an error for a label that does not match the protocol regex")
	}
}

func TestBackoffFor(t *testing.T) {
	tests := []struct {
		attempt int
		want    time.Duration
	}{
		{attempt: 1, want: time.Second},
		{attempt: 2, want: 2 * time.Second},
		{attempt: 5, want: 15 * time.Second},
		{attempt: 9, want: 30 * time.Second},
	}
	for _, tt := range tests {
		if got := backoffFor(tt.attempt); got != tt.want {
			t.Fatalf("backoffFor(%d) = %s, want %s", tt.attempt, got, tt.want)
		}
	}
}

// TestClientHandlesNoticeWithoutEndingTheSession: a notice is advisory. If it
// were treated like `error` the tunnel would die every time one request was
// too big.
func TestClientHandlesNoticeWithoutEndingTheSession(t *testing.T) {
	local := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("still here"))
	}))
	defer local.Close()

	ft := newFakeTunnel(t)
	notices := make(chan Notice, 4)
	client, _ := newTestClient(t, ft, local.URL, Options{
		Events: Events{OnNotice: func(n Notice) { notices <- n }},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runErr := make(chan error, 1)
	go func() { runErr <- client.Run(ctx) }()

	sc := ft.accept(t)
	sc.send(t, Assigned{Type: TypeAssigned, Label: "a-b-c", URL: "https://a-b-c." + Domain})
	sc.send(t, Notice{
		Type:      TypeNotice,
		Code:      NoticeRequestTooLarge,
		Message:   "a 12 MB body was rejected with 413",
		RequestID: "01JTOOBIG",
	})

	select {
	case n := <-notices:
		if n.Code != NoticeRequestTooLarge || n.RequestID != "01JTOOBIG" {
			t.Fatalf("notice = %+v", n)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("OnNotice was never called")
	}

	// The session must still be serving after the notice.
	sc.send(t, Request{Type: TypeRequest, ID: "after-notice", Method: http.MethodGet, Path: "/"})
	var resp Response
	sc.readFrameOfType(t, TypeResponse, &resp)
	if resp.ID != "after-notice" || resp.Status != http.StatusOK {
		t.Fatalf("session did not survive the notice: %+v", resp)
	}

	cancel()
	select {
	case err := <-runErr:
		if err != nil {
			t.Fatalf("Run returned %v, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after cancellation")
	}
}

// TestClientSurfacesLabelExpired covers the reclaim refusal the user will
// actually hit: a label whose six-hour window has passed.
func TestClientSurfacesLabelExpired(t *testing.T) {
	local := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer local.Close()

	ft := newFakeTunnel(t)
	client, _ := newTestClient(t, ft, local.URL, Options{
		RequestedLabel: "kucing-makan-ikan",
		ReclaimToken:   "c3RhbGUtdG9rZW4",
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runErr := make(chan error, 1)
	go func() { runErr <- client.Run(ctx) }()

	sc := ft.accept(t)
	sc.send(t, ErrorFrame{Type: TypeError, Code: CodeLabelExpired, Message: "reservation window passed"})

	select {
	case err := <-runErr:
		var fatal *FatalError
		if !errors.As(err, &fatal) || fatal.Code != CodeLabelExpired {
			t.Fatalf("Run returned %v, want a label_expired FatalError", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after a label_expired error frame")
	}
}

// TestFreshSessionAsksForNoLabel: every new session gets a new address, so a
// hello with no --subdomain must not smuggle a label or a token.
func TestFreshSessionAsksForNoLabel(t *testing.T) {
	local := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer local.Close()

	ft := newFakeTunnel(t)
	client, _ := newTestClient(t, ft, local.URL, Options{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runErr := make(chan error, 1)
	go func() { runErr <- client.Run(ctx) }()

	sc := ft.accept(t)
	if sc.hello.RequestedLabel != "" || sc.hello.ReclaimToken != "" {
		t.Fatalf("fresh hello = %+v, want no label and no reclaim token", sc.hello)
	}

	cancel()
	<-runErr
}

// TestTimingConstantsMatchTheProtocol pins the numbers PROTOCOL.md's timing
// table fixes, so a well-meaning tweak here shows up as a failing test rather
// than as a tunnel Cloudflare quietly drops.
func TestTimingConstantsMatchTheProtocol(t *testing.T) {
	if DefaultPingInterval != 30*time.Second {
		t.Fatalf("DefaultPingInterval = %s, want 30s", DefaultPingInterval)
	}
	if DefaultMaxMissedPongs != 3 {
		t.Fatalf("DefaultMaxMissedPongs = %d, want 3", DefaultMaxMissedPongs)
	}
	want := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 15 * time.Second, 30 * time.Second}
	for i, w := range want {
		if got := backoffFor(i + 1); got != w {
			t.Fatalf("backoffFor(%d) = %s, want %s", i+1, got, w)
		}
	}
	if got := backoffFor(len(want) + 1); got != 30*time.Second {
		t.Fatalf("backoff should hold at 30s, got %s", got)
	}
}

// TestFirstConnectionGivesUp: a connection that has never succeeded stops after
// MaxInitialAttempts. At that point the likely causes are a typo, a firewall or
// an outage, and retrying silently helps nobody.
func TestFirstConnectionGivesUp(t *testing.T) {
	local := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer local.Close()

	// Bind and release a port so nothing is listening on it.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	dead := ln.Addr().String()
	ln.Close()

	var attempts atomic.Int32
	client, err := New(Options{
		ServerURL: "ws://" + dead + "/_tunnel/connect",
		LocalURL:  local.URL,
		backoff:   func(int) time.Duration { return time.Millisecond },
		Events: Events{
			OnReconnect: func(int, time.Duration, error) { attempts.Add(1) },
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- client.Run(context.Background()) }()

	select {
	case err := <-done:
		var unreachable *UnreachableError
		if !errors.As(err, &unreachable) {
			t.Fatalf("Run returned %v, want an UnreachableError", err)
		}
		if unreachable.Attempts != MaxInitialAttempts {
			t.Fatalf("gave up after %d attempts, want %d", unreachable.Attempts, MaxInitialAttempts)
		}
		if unreachable.ServerURL == "" || unreachable.Err == nil {
			t.Fatalf("UnreachableError is missing context: %+v", unreachable)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Run never gave up on a server that was never reachable")
	}

	// Four announced retries, then the fifth failure ends it.
	if got := attempts.Load(); got != MaxInitialAttempts-1 {
		t.Fatalf("announced %d retries, want %d", got, MaxInitialAttempts-1)
	}
}

// TestAssignedSessionRetriesForever is the other half of the rule: once a
// tunnel has existed, a daemon restart under it must not kill the CLI, however
// many attempts it takes.
func TestAssignedSessionRetriesForever(t *testing.T) {
	local := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer local.Close()

	ft := newFakeTunnel(t)
	client, err := New(Options{
		ServerURL: ft.wsURL,
		LocalURL:  local.URL,
		backoff:   func(int) time.Duration { return time.Millisecond },
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- client.Run(ctx) }()

	// First session gets an address, then the connection dies.
	first := ft.accept(t)
	first.send(t, Assigned{
		Type: TypeAssigned, Label: "kucing-makan-ikan",
		URL:                  "https://kucing-makan-ikan." + Domain,
		ReclaimToken:         "dG9rZW4",
		ReclaimWindowSeconds: 21600,
	})
	// Let the assigned frame land before the socket goes away.
	time.Sleep(50 * time.Millisecond)
	first.finish()

	// Every later connection dies immediately. Well past the give-up bound.
	for i := 0; i < MaxInitialAttempts+3; i++ {
		sc := ft.accept(t)
		if sc.hello.RequestedLabel != "kucing-makan-ikan" || sc.hello.ReclaimToken != "dG9rZW4" {
			t.Fatalf("retry %d sent %+v, want the label and its token", i, sc.hello)
		}
		sc.finish()
		select {
		case err := <-done:
			t.Fatalf("Run gave up after an assigned session (retry %d): %v", i, err)
		default:
		}
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned %v after cancellation, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after cancellation")
	}
}

// TestReclaimTokenIsOverwrittenOnEveryAssigned: the previous token stops working
// the instant a new `assigned` is sent.
func TestReclaimTokenIsOverwrittenOnEveryAssigned(t *testing.T) {
	local := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer local.Close()

	ft := newFakeTunnel(t)
	seen := make(chan Assigned, 4)
	client, _ := newTestClient(t, ft, local.URL, Options{
		Events: Events{OnAssigned: func(a Assigned) { seen <- a }},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runErr := make(chan error, 1)
	go func() { runErr <- client.Run(ctx) }()

	sc := ft.accept(t)
	sc.send(t, Assigned{Type: TypeAssigned, Label: "a-b-c", URL: "https://a-b-c." + Domain, ReclaimToken: "first"})
	<-seen
	sc.send(t, Assigned{Type: TypeAssigned, Label: "a-b-c", URL: "https://a-b-c." + Domain, ReclaimToken: "second"})
	<-seen

	if got := client.ReclaimToken(); got != "second" {
		t.Fatalf("held token = %q, want the newest one", got)
	}

	cancel()
	<-runErr
}

// TestHeaderCasingSurvivesTheWholeLoop drives the interop case end to end: a
// `request` frame whose header names arrived lowercased (as Pingora hands them
// to the daemon) must reach the local server intact, and the `response` frame
// must go back in canonical Title-Case.
func TestHeaderCasingSurvivesTheWholeLoop(t *testing.T) {
	seen := make(chan http.Header, 1)
	local := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen <- r.Header.Clone()
		w.Header().Set("ETag", `"abc"`)
		w.Header().Set("content-type", "text/plain")
		_, _ = w.Write([]byte("ok"))
	}))
	defer local.Close()

	ft := newFakeTunnel(t)
	client, _ := newTestClient(t, ft, local.URL, Options{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runErr := make(chan error, 1)
	go func() { runErr <- client.Run(ctx) }()

	sc := ft.accept(t)
	sc.send(t, Assigned{Type: TypeAssigned, Label: "a-b-c", URL: "https://a-b-c." + Domain})
	sc.send(t, Request{
		Type:   TypeRequest,
		ID:     "01JCASE",
		Method: http.MethodGet,
		Path:   "/",
		Headers: map[string][]string{
			"accept":     {"text/plain"},
			"user-agent": {"curl/8"},
		},
	})

	select {
	case h := <-seen:
		if h.Get("Accept") != "text/plain" || h.Get("User-Agent") != "curl/8" {
			t.Fatalf("local server received %v, want the lowercase names normalised", h)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("the local server was never called")
	}

	var resp Response
	sc.readFrameOfType(t, TypeResponse, &resp)
	if v := resp.Headers["Content-Type"]; len(v) != 1 || v[0] != "text/plain" {
		t.Fatalf("response headers = %v, want a canonical Content-Type key", resp.Headers)
	}
	if v := resp.Headers["Etag"]; len(v) != 1 || v[0] != `"abc"` {
		t.Fatalf("response headers = %v, want Etag exactly as the daemon spells it", resp.Headers)
	}

	cancel()
	<-runErr
}

// TestClientAcceptsNoticeWithoutRequestID: request_too_large is decided while
// the body is still being read, before the request has an id, so the field is
// absent and the message carries the method and path instead.
func TestClientAcceptsNoticeWithoutRequestID(t *testing.T) {
	local := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer local.Close()

	ft := newFakeTunnel(t)
	notices := make(chan Notice, 2)
	client, _ := newTestClient(t, ft, local.URL, Options{
		Events: Events{OnNotice: func(n Notice) { notices <- n }},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runErr := make(chan error, 1)
	go func() { runErr <- client.Run(ctx) }()

	sc := ft.accept(t)
	sc.send(t, Assigned{Type: TypeAssigned, Label: "a-b-c", URL: "https://a-b-c." + Domain})
	// Sent as raw JSON with no request_id field at all, not merely an empty one.
	if err := sc.conn.WriteMessage(websocket.TextMessage, []byte(
		`{"type":"notice","code":"request_too_large","message":"POST /upload was over the 10 MB limit"}`)); err != nil {
		t.Fatalf("write notice: %v", err)
	}

	select {
	case n := <-notices:
		if n.RequestID != "" {
			t.Fatalf("request_id = %q, want it absent", n.RequestID)
		}
		if !strings.Contains(n.Message, "/upload") {
			t.Fatalf("message = %q, want the method and path", n.Message)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("a notice without request_id was not delivered")
	}

	cancel()
	<-runErr
}
