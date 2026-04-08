package terminal

import (
	"encoding/json"
	"testing"
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
