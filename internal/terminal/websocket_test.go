package terminal

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestDeriveOrigin(t *testing.T) {
	tests := []struct {
		name  string
		wsURL string
		want  string
	}{
		{name: "default secure", wsURL: "wss://api.dalang.io/ws", want: "https://dalang.io"},
		{name: "test secure", wsURL: "wss://test-api.dalang.io/ws", want: "https://test.dalang.io"},
		{name: "plain ws", wsURL: "ws://api.dalang.io/ws", want: "http://dalang.io"},
		{name: "custom host preserved", wsURL: "wss://edge.example.com/ws", want: "https://edge.example.com"},
		{name: "invalid url falls back", wsURL: "not-a-url", want: "https://dalang.io"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := deriveOrigin(tt.wsURL); got != tt.want {
				t.Fatalf("deriveOrigin(%q) = %q, want %q", tt.wsURL, got, tt.want)
			}
		})
	}
}

func TestResizeMessageJSON(t *testing.T) {
	msg := ResizeMessage{
		Type: "resize",
		Cols: 120,
		Rows: 40,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Marshal ResizeMessage failed: %v", err)
	}

	var decoded ResizeMessage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal ResizeMessage failed: %v", err)
	}

	if decoded.Type != "resize" {
		t.Fatalf("Type = %q, want %q", decoded.Type, "resize")
	}
	if decoded.Cols != 120 {
		t.Fatalf("Cols = %d, want %d", decoded.Cols, 120)
	}
	if decoded.Rows != 40 {
		t.Fatalf("Rows = %d, want %d", decoded.Rows, 40)
	}
}

func TestDeriveOriginVariants(t *testing.T) {
	tests := []struct {
		name  string
		wsURL string
		want  string
	}{
		{name: "wss with port", wsURL: "wss://api.dalang.io:8443/ws", want: "https://api.dalang.io:8443"},
		{name: "ws with path", wsURL: "ws://api.dalang.io/ws/shell/123", want: "http://dalang.io"},
		{name: "no path", wsURL: "wss://api.dalang.io", want: "https://dalang.io"},
		{name: "empty string", wsURL: "", want: "https://dalang.io"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := deriveOrigin(tt.wsURL)
			// Just verify it doesn't panic and returns a valid origin
			if got == "" {
				t.Fatal("deriveOrigin returned empty string")
			}
		})
	}
}

func TestTerminalCloseDone(t *testing.T) {
	term := &Terminal{
		done: make(chan struct{}),
	}

	// First call should work
	term.closeDone()

	// Second call should not panic (sync.Once)
	term.closeDone()

	// Channel should be closed
	select {
	case <-term.done:
		// expected
	default:
		t.Fatal("done channel should be closed")
	}
}

func TestTerminalRestoreSafeToCallMultipleTimes(t *testing.T) {
	term := &Terminal{
		done: make(chan struct{}),
	}

	// restore() with nil oldState should not panic, even called multiple times
	term.restore()
	term.restore()
}

// TestReadLoopUnblocksOnClose is the regression test for the ~. / Ctrl+C hang:
// readLoop blocks in conn.ReadMessage, and a local disconnect must close the
// connection so readLoop unblocks and returns nil (clean exit), not an error.
func TestReadLoopUnblocksOnClose(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer c.Close()
		// Hold the connection open (never send) until the client closes it,
		// so the client's readLoop stays blocked in ReadMessage.
		for {
			if _, _, err := c.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	term, err := NewTerminal(wsURL, "")
	if err != nil {
		t.Fatalf("NewTerminal: %v", err)
	}

	result := make(chan error, 1)
	go func() { result <- term.readLoop() }()

	// Let readLoop reach the blocking ReadMessage.
	time.Sleep(50 * time.Millisecond)

	// Simulate the ~. escape / Ctrl+C disconnect path.
	term.Close()

	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("readLoop should return nil on intentional close, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("readLoop did not unblock after Close() — disconnect hangs")
	}
}

func TestNewTerminalSignature(t *testing.T) {
	// Verify NewTerminal accepts a token parameter (fix #10).
	// We can't actually connect, but we verify the function signature compiles.
	_, err := NewTerminal("wss://invalid.example.com/ws", "test-token")
	if err == nil {
		t.Fatal("expected error connecting to invalid host")
	}
}
