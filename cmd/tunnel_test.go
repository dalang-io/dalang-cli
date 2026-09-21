package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/dalang-io/dalang-cli/internal/config"
	"github.com/dalang-io/dalang-cli/internal/tunnel"
	"github.com/gorilla/websocket"
)

// newLabelExpiredServer stands up a tunnel endpoint that refuses every reclaim,
// which is what the daemon does once the six-hour window has passed.
func newLabelExpiredServer(t *testing.T) string {
	t.Helper()

	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer c.Close()
		if _, _, err := c.ReadMessage(); err != nil {
			return
		}
		_ = c.WriteJSON(tunnel.ErrorFrame{
			Type:    tunnel.TypeError,
			Code:    tunnel.CodeLabelExpired,
			Message: "reservation window passed",
		})
	}))
	t.Cleanup(srv.Close)

	return "ws" + strings.TrimPrefix(srv.URL, "http") + "/_tunnel/connect"
}

// captureStdout runs fn with stdout redirected and returns what it printed.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe setup failed: %v", err)
	}
	os.Stdout = w
	defer func() { os.Stdout = old }()

	fn()

	if err := w.Close(); err != nil {
		t.Fatalf("closing writer failed: %v", err)
	}
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("reading output failed: %v", err)
	}
	return buf.String()
}

func TestParseTunnelArgs(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		wantURL   string
		wantSub   string
		wantSrv   string
		wantHelp  bool
		wantError bool
	}{
		{name: "url flag", args: []string{"--url", "http://localhost:8000"}, wantURL: "http://localhost:8000"},
		{name: "url equals form", args: []string{"--url=:8000"}, wantURL: ":8000"},
		{name: "positional", args: []string{"8000"}, wantURL: "8000"},
		{name: "subdomain", args: []string{"--url", "8000", "--subdomain", "kucing-makan-ikan"},
			wantURL: "8000", wantSub: "kucing-makan-ikan"},
		{name: "subdomain equals form", args: []string{"--url=8000", "--subdomain=a-b-c"},
			wantURL: "8000", wantSub: "a-b-c"},
		{name: "server override", args: []string{"--url", "8000", "--server", "ws://127.0.0.1:9999/_tunnel/connect"},
			wantURL: "8000", wantSrv: "ws://127.0.0.1:9999/_tunnel/connect"},
		{name: "help", args: []string{"--help"}, wantHelp: true},
		{name: "help short", args: []string{"-h"}, wantHelp: true},
		{name: "no args", args: nil},
		{name: "url without value", args: []string{"--url"}, wantError: true},
		{name: "subdomain without value", args: []string{"--url", "8000", "--subdomain"}, wantError: true},
		{name: "unknown flag", args: []string{"--port", "8000"}, wantError: true},
		{name: "two positionals", args: []string{"8000", "9000"}, wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseTunnelArgs(tt.args)
			if tt.wantError {
				if err == nil {
					t.Fatalf("parseTunnelArgs(%v) = %+v, want an error", tt.args, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseTunnelArgs(%v) returned error: %v", tt.args, err)
			}
			if got.localURL != tt.wantURL || got.subdomain != tt.wantSub ||
				got.server != tt.wantSrv || got.help != tt.wantHelp {
				t.Fatalf("parseTunnelArgs(%v) = %+v, want url=%q sub=%q server=%q help=%v",
					tt.args, got, tt.wantURL, tt.wantSub, tt.wantSrv, tt.wantHelp)
			}
		})
	}
}

// TestCmdTunnelFailsFastBeforeConnecting: both of these must be rejected
// locally, without a round trip to the tunnel server.
func TestCmdTunnelFailsFastBeforeConnecting(t *testing.T) {
	resetGlobalFlags()
	t.Cleanup(resetGlobalFlags)

	// Point at an address nothing serves, so a bug that dials anyway fails
	// loudly rather than silently reaching the real tunnel server.
	t.Setenv("DALANG_TUNNEL_URL", "ws://127.0.0.1:1/_tunnel/connect")

	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "missing url", args: nil, want: "--url is required"},
		{name: "bad url", args: []string{"--url", "ftp://localhost:21"}, want: "only http and https"},
		{name: "bad subdomain", args: []string{"--url", "8000", "--subdomain", "kucing_makan_ikan"}, want: "three lowercase"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			done := make(chan error, 1)
			go func() { done <- cmdTunnel(tt.args) }()
			select {
			case err := <-done:
				if err == nil {
					t.Fatalf("cmdTunnel(%v) = nil, want an error", tt.args)
				}
				if !strings.Contains(err.Error(), tt.want) {
					t.Fatalf("error = %q, want it to mention %q", err, tt.want)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("cmdTunnel did not fail locally — it is validating after connecting")
			}
		})
	}
}

func TestCmdTunnelHelp(t *testing.T) {
	resetGlobalFlags()
	t.Cleanup(resetGlobalFlags)

	out := captureStdout(t, func() {
		if err := cmdTunnel([]string{"--help"}); err != nil {
			t.Fatalf("cmdTunnel --help returned %v", err)
		}
	})

	for _, needle := range []string{"dalang tunnel", "--url", "--subdomain", "10 MB"} {
		if !strings.Contains(out, needle) {
			t.Fatalf("tunnel help is missing %q", needle)
		}
	}
}

func TestTunnelHelpIsReachableFromTheDispatcher(t *testing.T) {
	resetGlobalFlags()
	t.Cleanup(resetGlobalFlags)

	out := captureStdout(t, func() {
		if err := cmdHelpFor("tunnel"); err != nil {
			t.Fatalf("cmdHelpFor(tunnel) returned %v", err)
		}
	})
	if !strings.Contains(out, "dalang tunnel") {
		t.Fatal("cmdHelpFor(\"tunnel\") did not print the tunnel help")
	}

	out = captureStdout(t, printHelp)
	if !strings.Contains(out, "tunnel --url <addr>") {
		t.Fatal("the top-level help does not list the tunnel command")
	}
}

// TestTunnelReporterPrintsURLProminently: printing the public URL is the whole
// point of the command.
func TestTunnelReporterPrintsURLProminently(t *testing.T) {
	resetGlobalFlags()
	t.Cleanup(resetGlobalFlags)

	r := &tunnelReporter{localURL: "http://localhost:8000"}
	out := captureStdout(t, func() {
		r.assigned(tunnel.Assigned{
			Type:         tunnel.TypeAssigned,
			Label:        "kucing-makan-ikan",
			URL:          "https://kucing-makan-ikan.try.dalang.io",
			ExpiresAt:    time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339),
			MaxBodyBytes: 10 << 20,
		})
	})

	if !strings.Contains(out, "https://kucing-makan-ikan.try.dalang.io") {
		t.Fatalf("assigned output does not contain the public URL:\n%s", out)
	}
	if !strings.Contains(out, "http://localhost:8000") {
		t.Fatalf("assigned output does not show what it forwards to:\n%s", out)
	}
	if !strings.Contains(out, "10 MB") {
		t.Fatalf("assigned output does not state the body limit:\n%s", out)
	}
}

func TestTunnelReporterQuietPrintsOnlyTheURL(t *testing.T) {
	resetGlobalFlags()
	t.Cleanup(resetGlobalFlags)
	quietOutput = true

	r := &tunnelReporter{localURL: "http://localhost:8000"}
	out := captureStdout(t, func() {
		r.assigned(tunnel.Assigned{Type: tunnel.TypeAssigned, Label: "a-b-c", URL: "https://a-b-c.try.dalang.io"})
		r.request(
			tunnel.Request{Method: "GET", Path: "/"},
			tunnel.Result{Status: 200, Duration: 3 * time.Millisecond},
		)
	})

	if out != "https://a-b-c.try.dalang.io\n" {
		t.Fatalf("quiet output = %q, want just the URL line", out)
	}
}

func TestTunnelReporterLogsOneLinePerRequest(t *testing.T) {
	resetGlobalFlags()
	t.Cleanup(resetGlobalFlags)
	noColorFlag = true
	applyColorSettings()

	r := &tunnelReporter{localURL: "http://localhost:8000"}
	out := captureStdout(t, func() {
		r.request(
			tunnel.Request{Method: "GET", Path: "/api/items?page=2"},
			tunnel.Result{Status: 200, Duration: 12 * time.Millisecond},
		)
	})

	for _, needle := range []string{"200", "GET", "/api/items?page=2", "12ms"} {
		if !strings.Contains(out, needle) {
			t.Fatalf("request log %q is missing %q", strings.TrimSpace(out), needle)
		}
	}
	if lines := strings.Count(strings.TrimSpace(out), "\n"); lines != 0 {
		t.Fatalf("expected a single log line, got:\n%s", out)
	}
}

// TestTunnelReporterWarnsWhenLocalServerIsDown: a 502 is the user's own app
// being down, and saying so beats them blaming the tunnel.
func TestTunnelReporterWarnsWhenLocalServerIsDown(t *testing.T) {
	resetGlobalFlags()
	t.Cleanup(resetGlobalFlags)
	noColorFlag = true
	applyColorSettings()

	r := &tunnelReporter{localURL: "http://localhost:8000"}
	out := captureStdout(t, func() {
		r.request(
			tunnel.Request{Method: "GET", Path: "/"},
			tunnel.Result{Status: 502, Duration: time.Millisecond, Err: errConnRefused{}},
		)
	})

	if !strings.Contains(out, "502") {
		t.Fatalf("expected the 502 in the log line, got:\n%s", out)
	}
	if !strings.Contains(out, "http://localhost:8000") {
		t.Fatalf("expected a warning naming the unreachable local server, got:\n%s", out)
	}
}

type errConnRefused struct{}

func (errConnRefused) Error() string { return "dial tcp 127.0.0.1:8000: connect: connection refused" }

func TestTunnelReporterJSONOutput(t *testing.T) {
	resetGlobalFlags()
	t.Cleanup(resetGlobalFlags)
	jsonOutput = true

	r := &tunnelReporter{localURL: "http://localhost:8000"}
	out := captureStdout(t, func() {
		r.assigned(tunnel.Assigned{Type: tunnel.TypeAssigned, Label: "a-b-c", URL: "https://a-b-c.try.dalang.io"})
		r.request(
			tunnel.Request{ID: "01J", Method: "GET", Path: "/"},
			tunnel.Result{Status: 200, Duration: 5 * time.Millisecond},
		)
	})

	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected one JSON object per line, got:\n%s", out)
	}
	var assigned map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &assigned); err != nil {
		t.Fatalf("assigned line is not JSON: %v (%s)", err, lines[0])
	}
	if assigned["url"] != "https://a-b-c.try.dalang.io" || assigned["type"] != "assigned" {
		t.Fatalf("assigned JSON = %v", assigned)
	}
	var req map[string]any
	if err := json.Unmarshal([]byte(lines[1]), &req); err != nil {
		t.Fatalf("request line is not JSON: %v (%s)", err, lines[1])
	}
	if req["status"] != float64(200) || req["path"] != "/" {
		t.Fatalf("request JSON = %v", req)
	}
}

func TestFormatTunnelDuration(t *testing.T) {
	tests := []struct {
		in   time.Duration
		want string
	}{
		{in: 250 * time.Microsecond, want: "250µs"},
		{in: 12 * time.Millisecond, want: "12ms"},
		{in: 1500 * time.Millisecond, want: "1.50s"},
	}
	for _, tt := range tests {
		if got := formatTunnelDuration(tt.in); got != tt.want {
			t.Fatalf("formatTunnelDuration(%s) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestTunnelFatalMessage(t *testing.T) {
	tests := []struct {
		name  string
		fatal *tunnel.FatalError
		label string
		want  string
	}{
		{name: "label taken", fatal: &tunnel.FatalError{Code: tunnel.CodeLabelTaken}, label: "kucing-makan-ikan", want: "kucing-makan-ikan"},
		{name: "rate limited", fatal: &tunnel.FatalError{Code: tunnel.CodeRateLimited}, want: "rate limited"},
		{name: "unsupported version", fatal: &tunnel.FatalError{Code: tunnel.CodeUnsupportedVersion}, want: "dalang update"},
		{name: "expired", fatal: &tunnel.FatalError{Code: tunnel.ReasonExpired}, want: "expired"},
		{name: "unknown code", fatal: &tunnel.FatalError{Code: "weird", Message: "hm"}, want: "weird"},
		{name: "empty code still errors", fatal: &tunnel.FatalError{}, want: "unrecognised"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tunnelFatalMessage(tt.fatal, tt.label)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("tunnelFatalMessage = %v, want it to mention %q", err, tt.want)
			}
		})
	}
}

func TestTunnelStopSignalsIncludesInterrupt(t *testing.T) {
	sigs := tunnelStopSignals()
	if len(sigs) == 0 {
		t.Fatal("tunnelStopSignals returned nothing — Ctrl+C would not be handled")
	}
	found := false
	for _, s := range sigs {
		if s == os.Interrupt {
			found = true
		}
	}
	if !found {
		t.Fatalf("tunnelStopSignals = %v, want it to include os.Interrupt", sigs)
	}
}

// setTunnelTestHome points ~/.dalang at a temp dir so reclaim-token tests never
// touch the developer's own credentials.
func setTunnelTestHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home) // windows
	return home
}

func TestPersistReclaimRoundTrip(t *testing.T) {
	resetGlobalFlags()
	t.Cleanup(resetGlobalFlags)
	setTunnelTestHome(t)

	persistReclaim(tunnel.Assigned{
		Type:                 tunnel.TypeAssigned,
		Label:                "kucing-makan-ikan",
		URL:                  "https://kucing-makan-ikan.try.dalang.io",
		ReclaimToken:         "dG9rZW4",
		ReclaimWindowSeconds: 21600,
	})

	got, ok := config.GetTunnelReclaim("kucing-makan-ikan")
	if !ok {
		t.Fatal("the reclaim token was not stored — the address could never be taken back")
	}
	if got.ReclaimToken != "dG9rZW4" {
		t.Fatalf("stored token = %q", got.ReclaimToken)
	}
}

func TestPersistReclaimIgnoresAnAssignedWithoutAToken(t *testing.T) {
	resetGlobalFlags()
	t.Cleanup(resetGlobalFlags)
	setTunnelTestHome(t)

	persistReclaim(tunnel.Assigned{Type: tunnel.TypeAssigned, Label: "a-b-c", URL: "https://a-b-c.try.dalang.io"})
	if _, ok := config.GetTunnelReclaim("a-b-c"); ok {
		t.Fatal("stored an empty reclaim token")
	}
}

// TestCmdTunnelWarnsWhenReclaimingWithoutAToken: the request still goes to the
// daemon (it is the authority), but the user is told why it will probably fail.
func TestCmdTunnelWarnsWhenReclaimingWithoutAToken(t *testing.T) {
	resetGlobalFlags()
	t.Cleanup(resetGlobalFlags)
	setTunnelTestHome(t)
	noColorFlag = true
	applyColorSettings()

	// The server refuses the reclaim, so the command returns right after the
	// local warning this test is about.
	t.Setenv("DALANG_TUNNEL_URL", newLabelExpiredServer(t))

	out := captureStdout(t, func() {
		done := make(chan error, 1)
		go func() { done <- cmdTunnel([]string{"--url", "8000", "--subdomain", "kucing-makan-ikan"}) }()
		select {
		case err := <-done:
			if err == nil {
				t.Error("expected the reclaim to be refused")
			}
		case <-time.After(10 * time.Second):
			t.Error("cmdTunnel did not return")
		}
	})

	if !strings.Contains(out, "No reclaim token") {
		t.Fatalf("expected a warning about the missing reclaim token, got:\n%s", out)
	}
	if !strings.Contains(out, "6 hours") {
		t.Fatalf("expected the warning to explain the 6-hour window, got:\n%s", out)
	}
}

func TestTunnelReporterPrintsNotices(t *testing.T) {
	resetGlobalFlags()
	t.Cleanup(resetGlobalFlags)
	noColorFlag = true
	applyColorSettings()

	r := &tunnelReporter{localURL: "http://localhost:8000"}
	out := captureStdout(t, func() {
		r.notice(tunnel.Notice{
			Type:      tunnel.TypeNotice,
			Code:      tunnel.NoticeRequestTooLarge,
			Message:   "a 12 MB body was rejected with 413",
			RequestID: "01JTOOBIG",
		})
	})

	for _, needle := range []string{"12 MB", "request_too_large", "01JTOOBIG"} {
		if !strings.Contains(out, needle) {
			t.Fatalf("notice output %q is missing %q", strings.TrimSpace(out), needle)
		}
	}
}

func TestTunnelReporterNoticeFallsBackToTheCode(t *testing.T) {
	resetGlobalFlags()
	t.Cleanup(resetGlobalFlags)
	noColorFlag = true
	applyColorSettings()

	r := &tunnelReporter{localURL: "http://localhost:8000"}
	out := captureStdout(t, func() {
		r.notice(tunnel.Notice{Type: tunnel.TypeNotice, Code: tunnel.NoticeNearingExpiry})
	})
	if !strings.Contains(out, "expiry") {
		t.Fatalf("a bare notice code should still read as something, got:\n%s", out)
	}
}

func TestTunnelReporterNoticeJSON(t *testing.T) {
	resetGlobalFlags()
	t.Cleanup(resetGlobalFlags)
	jsonOutput = true

	r := &tunnelReporter{localURL: "http://localhost:8000"}
	out := captureStdout(t, func() {
		r.notice(tunnel.Notice{Type: tunnel.TypeNotice, Code: tunnel.NoticeResponseDropped, Message: "gone", RequestID: "01J"})
	})

	var entry map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &entry); err != nil {
		t.Fatalf("notice line is not JSON: %v (%s)", err, out)
	}
	if entry["type"] != "notice" || entry["code"] != "response_dropped" || entry["request_id"] != "01J" {
		t.Fatalf("notice JSON = %v", entry)
	}
}

// TestPrintReclaimHintShowsTheExactCommand: the window is six hours and the
// token only exists on this machine, so the command is worth printing.
func TestPrintReclaimHintShowsTheExactCommand(t *testing.T) {
	resetGlobalFlags()
	t.Cleanup(resetGlobalFlags)
	noColorFlag = true
	applyColorSettings()

	r := &tunnelReporter{localURL: "http://localhost:8000"}
	captureStdout(t, func() {
		r.assigned(tunnel.Assigned{
			Type:                 tunnel.TypeAssigned,
			Label:                "kucing-makan-ikan",
			URL:                  "https://kucing-makan-ikan.try.dalang.io",
			ReclaimToken:         "tok",
			ReclaimWindowSeconds: 21600,
		})
	})

	out := captureStdout(t, r.printReclaimHint)
	if !strings.Contains(out, "dalang tunnel --url http://localhost:8000 --subdomain kucing-makan-ikan") {
		t.Fatalf("reclaim hint does not show the command to run:\n%s", out)
	}
}

func TestPrintReclaimHintSilentWithoutALabel(t *testing.T) {
	resetGlobalFlags()
	t.Cleanup(resetGlobalFlags)

	r := &tunnelReporter{localURL: "http://localhost:8000"}
	if out := captureStdout(t, r.printReclaimHint); out != "" {
		t.Fatalf("expected no hint before any address was assigned, got:\n%s", out)
	}
}

func TestStaleCredentialsDoNotBlockAnAnonymousTunnel(t *testing.T) {
	// Reported by a user running `dalang tunnel --url http://localhost:80` with
	// a credentials file six weeks old. There is no --token flag: the token is
	// read from disk without them asking, so refusing the whole tunnel over an
	// expired one broke the single promise this feature makes — that it works
	// without an account. Anyone who signed in once and let it lapse was locked
	// out of the free tier.
	refused := &tunnel.FatalError{Code: tunnel.CodeBadRequest, Message: "refused"}

	cases := []struct {
		name   string
		err    error
		stored bool
		want   bool
	}{
		{"stored token refused -> retry anonymously", refused, true, true},
		{"no token to blame -> do not retry", refused, false, false},
		{"a different refusal is not about the token", &tunnel.FatalError{Code: tunnel.CodeLabelTaken}, true, false},
		{"rate limited is a real refusal, not a bad token", &tunnel.FatalError{Code: tunnel.CodeRateLimited}, true, false},
		{"a transport error is not a token problem", errors.New("connection reset"), true, false},
		{"a clean exit retries nothing", nil, true, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldRetryAnonymously(tc.err, tc.stored); got != tc.want {
				t.Fatalf("shouldRetryAnonymously(%v, %v) = %v, want %v", tc.err, tc.stored, got, tc.want)
			}
		})
	}
}

func TestTunnelFatalMessageBadRequestIsNotTreatedAsUnknown(t *testing.T) {
	// bad_request is in the protocol, and the daemon uses it for a token
	// api.dalang.io refused. Falling through to the default arm told the user
	// "this CLI may be older than the server; try 'dalang update'" when the
	// thing that was out of date was their token — advice that sends them to
	// fix the wrong thing.
	err := tunnelFatalMessage(&tunnel.FatalError{
		Code:    tunnel.CodeBadRequest,
		Message: "that token was refused by api.dalang.io — run 'dalang auth' to sign in again",
	}, "")
	got := err.Error()

	if strings.Contains(got, "unrecognised") || strings.Contains(got, "dalang update") {
		t.Fatalf("bad_request was handled by the default arm: %s", got)
	}
	if !strings.Contains(got, "dalang auth") {
		t.Fatalf("the server's advice was dropped: %s", got)
	}
}

func TestTunnelFatalMessageReclaimCodes(t *testing.T) {
	tests := []struct {
		name  string
		fatal *tunnel.FatalError
		label string
		want  []string
	}{
		{
			name:  "label expired names the window",
			fatal: &tunnel.FatalError{Code: tunnel.CodeLabelExpired},
			label: "kucing-makan-ikan",
			want:  []string{"kucing-makan-ikan.try.dalang.io", "not yours to reclaim", "6 hours"},
		},
		{
			name:  "label taken explains the token",
			fatal: &tunnel.FatalError{Code: tunnel.CodeLabelTaken},
			label: "kucing-makan-ikan",
			want:  []string{"in use", "--subdomain"},
		},
		{
			name:  "pool exhausted",
			fatal: &tunnel.FatalError{Code: tunnel.CodePoolExhausted},
			want:  []string{"no free addresses"},
		},
		{
			name:  "handshake rejected names the layer that refused",
			fatal: &tunnel.FatalError{Code: tunnel.CodeHandshakeRejected, Message: "status 403 from wss://tunnel.try.dalang.io/_tunnel/connect"},
			want:  []string{"handshake", "403"},
		},
		{
			// Codes leave the protocol (`evicted`, `unauthorized` both did);
			// a CLI that meets one it does not know must still stop, and say
			// something a user can act on.
			name:  "unrecognised code fails safe",
			fatal: &tunnel.FatalError{Code: "evicted", Message: "gone"},
			want:  []string{"unrecognised code", "evicted", "dalang update"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tunnelFatalMessage(tt.fatal, tt.label)
			if err == nil {
				t.Fatal("expected an error")
			}
			for _, needle := range tt.want {
				if !strings.Contains(err.Error(), needle) {
					t.Fatalf("message %q is missing %q", err, needle)
				}
			}
		})
	}
}

// TestCmdTunnelForgetsAnExpiredReservation: keeping a dead token would make the
// next attempt fail the same way.
func TestCmdTunnelForgetsAnExpiredReservation(t *testing.T) {
	resetGlobalFlags()
	t.Cleanup(resetGlobalFlags)
	setTunnelTestHome(t)
	quietOutput = true

	if err := config.SaveTunnelReclaim(config.TunnelReclaim{
		Label:         "kucing-makan-ikan",
		ReclaimToken:  "stale",
		WindowSeconds: 21600,
		UntilEstimate: time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("seeding the store failed: %v", err)
	}

	srv := newLabelExpiredServer(t)
	t.Setenv("DALANG_TUNNEL_URL", srv)

	err := cmdTunnel([]string{"--url", "8000", "--subdomain", "kucing-makan-ikan"})
	if err == nil || !strings.Contains(err.Error(), "not yours to reclaim") {
		t.Fatalf("cmdTunnel returned %v, want a label_expired message", err)
	}
	if _, ok := config.GetTunnelReclaim("kucing-makan-ikan"); ok {
		t.Fatal("the stale reclaim token should have been dropped")
	}
}

// TestPersistReclaimDerivesTheDeadline: the wire carries a duration because the
// window runs from the close, so the CLI computes the deadline itself.
func TestPersistReclaimDerivesTheDeadline(t *testing.T) {
	resetGlobalFlags()
	t.Cleanup(resetGlobalFlags)
	setTunnelTestHome(t)

	before := time.Now()
	persistReclaim(tunnel.Assigned{
		Type:                 tunnel.TypeAssigned,
		Label:                "kucing-makan-ikan",
		URL:                  "https://kucing-makan-ikan.try.dalang.io",
		ReclaimToken:         "dG9rZW4",
		ReclaimWindowSeconds: 21600,
	})

	got, ok := config.GetTunnelReclaim("kucing-makan-ikan")
	if !ok {
		t.Fatal("nothing stored")
	}
	if got.WindowSeconds != 21600 {
		t.Fatalf("WindowSeconds = %d, want 21600", got.WindowSeconds)
	}
	until, err := time.Parse(time.RFC3339, got.UntilEstimate)
	if err != nil {
		t.Fatalf("estimate %q is not RFC3339: %v", got.UntilEstimate, err)
	}
	// Written at assignment time it is the worst case: if the process died now,
	// this is exactly the deadline.
	lo, hi := before.Add(6*time.Hour), time.Now().Add(6*time.Hour).Add(time.Second)
	if until.Before(lo.Add(-time.Second)) || until.After(hi) {
		t.Fatalf("estimate %s is not ~6h from now (%s..%s)", until, lo, hi)
	}
}

// TestPersistReclaimOverwritesTheOlderToken: the previous token stops working
// the instant a new `assigned` is sent.
func TestPersistReclaimOverwritesTheOlderToken(t *testing.T) {
	resetGlobalFlags()
	t.Cleanup(resetGlobalFlags)
	setTunnelTestHome(t)

	base := tunnel.Assigned{
		Type:                 tunnel.TypeAssigned,
		Label:                "kucing-makan-ikan",
		URL:                  "https://kucing-makan-ikan.try.dalang.io",
		ReclaimWindowSeconds: 21600,
	}
	base.ReclaimToken = "first"
	persistReclaim(base)
	base.ReclaimToken = "second"
	persistReclaim(base)

	got, ok := config.GetTunnelReclaim("kucing-makan-ikan")
	if !ok {
		t.Fatal("nothing stored")
	}
	if got.ReclaimToken != "second" {
		t.Fatalf("stored token = %q, want the newest one", got.ReclaimToken)
	}
}

// TestReclaimHintCallsTheDeadlineAnEstimate: the daemon decides on its own
// clock, so the CLI must not present its own arithmetic as a promise.
func TestReclaimHintCallsTheDeadlineAnEstimate(t *testing.T) {
	resetGlobalFlags()
	t.Cleanup(resetGlobalFlags)
	noColorFlag = true
	applyColorSettings()

	r := &tunnelReporter{localURL: "http://localhost:8000"}
	captureStdout(t, func() {
		r.assigned(tunnel.Assigned{
			Type:                 tunnel.TypeAssigned,
			Label:                "kucing-makan-ikan",
			URL:                  "https://kucing-makan-ikan.try.dalang.io",
			ReclaimToken:         "tok",
			ReclaimWindowSeconds: 21600,
		})
	})

	out := captureStdout(t, r.printReclaimHint)
	if !strings.Contains(out, "about 6h0m0s") {
		t.Fatalf("hint should state the window it was given, got:\n%s", out)
	}
	if !strings.Contains(out, "roughly") {
		t.Fatalf("hint must not present a computed deadline as a promise, got:\n%s", out)
	}
}

func TestTunnelUnreachableMessage(t *testing.T) {
	err := tunnelUnreachableMessage(&tunnel.UnreachableError{
		ServerURL: "wss://tunnel.try.dalang.io/_tunnel/connect",
		Attempts:  5,
		Err:       errConnRefused{},
	})
	if err == nil {
		t.Fatal("expected an error")
	}
	for _, needle := range []string{"wss://tunnel.try.dalang.io", "5 attempts", "connection refused", "firewall"} {
		if !strings.Contains(err.Error(), needle) {
			t.Fatalf("message %q is missing %q", err, needle)
		}
	}
}

// TestTunnelReporterNoticeWithoutRequestID: the field is optional and often
// absent, so the line must not quote an id the user never saw.
func TestTunnelReporterNoticeWithoutRequestID(t *testing.T) {
	resetGlobalFlags()
	t.Cleanup(resetGlobalFlags)
	noColorFlag = true
	applyColorSettings()

	r := &tunnelReporter{localURL: "http://localhost:8000"}
	out := captureStdout(t, func() {
		r.notice(tunnel.Notice{
			Type:    tunnel.TypeNotice,
			Code:    tunnel.NoticeRequestTooLarge,
			Message: "POST /upload was over the 10 MB limit",
		})
	})

	if !strings.Contains(out, "POST /upload") {
		t.Fatalf("notice should print the method and path the daemon put in the message, got:\n%s", out)
	}
	if strings.Contains(out, "request )") || strings.Contains(out, "request ,") {
		t.Fatalf("notice printed an empty request id:\n%s", out)
	}
	if !strings.Contains(out, "request_too_large") {
		t.Fatalf("notice should still name the code, got:\n%s", out)
	}
}

func TestTunnelReporterNoticeJSONOmitsAbsentRequestID(t *testing.T) {
	resetGlobalFlags()
	t.Cleanup(resetGlobalFlags)
	jsonOutput = true

	r := &tunnelReporter{localURL: "http://localhost:8000"}
	out := captureStdout(t, func() {
		r.notice(tunnel.Notice{Type: tunnel.TypeNotice, Code: tunnel.NoticeRequestTooLarge, Message: "POST /upload was too big"})
	})

	var entry map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &entry); err != nil {
		t.Fatalf("notice line is not JSON: %v (%s)", err, out)
	}
	if _, ok := entry["request_id"]; ok {
		t.Fatalf("request_id should be omitted when absent, got %v", entry)
	}
}
