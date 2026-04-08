package terminal

import "testing"

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
