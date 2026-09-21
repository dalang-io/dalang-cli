package tunnel

import "testing"

func TestNormalizeLocalURL(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "bare port", in: "8000", want: "http://localhost:8000"},
		{name: "colon port", in: ":8000", want: "http://localhost:8000"},
		{name: "host and port", in: "localhost:8000", want: "http://localhost:8000"},
		{name: "full url", in: "http://localhost:8000", want: "http://localhost:8000"},
		{name: "https preserved", in: "https://localhost:8443", want: "https://localhost:8443"},
		{name: "trailing slash trimmed", in: "http://localhost:8000/", want: "http://localhost:8000"},
		{name: "path preserved", in: "localhost:8000/api", want: "http://localhost:8000/api"},
		{name: "path trailing slash trimmed", in: "http://localhost:8000/api/", want: "http://localhost:8000/api"},
		{name: "ipv4", in: "127.0.0.1:3000", want: "http://127.0.0.1:3000"},
		{name: "wildcard bind becomes loopback", in: "0.0.0.0:8000", want: "http://127.0.0.1:8000"},
		{name: "ipv6 wildcard becomes loopback", in: "http://[::]:8000", want: "http://[::1]:8000"},
		{name: "ipv6 literal", in: "http://[::1]:8000", want: "http://[::1]:8000"},
		{name: "no port", in: "localhost", want: "http://localhost"},
		{name: "remote host", in: "dev.internal:9000", want: "http://dev.internal:9000"},
		{name: "whitespace tolerated", in: "  :5173  ", want: "http://localhost:5173"},
		{name: "uppercase host lowercased by url parse", in: "http://LOCALHOST:8000", want: "http://localhost:8000"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeLocalURL(tt.in)
			if err != nil {
				t.Fatalf("NormalizeLocalURL(%q) returned error: %v", tt.in, err)
			}
			if got != tt.want {
				t.Fatalf("NormalizeLocalURL(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestNormalizeLocalURLErrors(t *testing.T) {
	tests := []struct {
		name string
		in   string
	}{
		{name: "empty", in: ""},
		{name: "whitespace only", in: "   "},
		{name: "unsupported scheme", in: "ftp://localhost:21"},
		{name: "websocket scheme", in: "ws://localhost:8000"},
		{name: "non numeric port", in: "localhost:abc"},
		{name: "port out of range", in: "localhost:70000"},
		{name: "port zero", in: "localhost:0"},
		{name: "query string", in: "http://localhost:8000/?a=b"},
		{name: "fragment", in: "http://localhost:8000/#top"},
		{name: "no host", in: "http://"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeLocalURL(tt.in)
			if err == nil {
				t.Fatalf("NormalizeLocalURL(%q) = %q, want an error", tt.in, got)
			}
		})
	}
}

func TestNormalizeLabel(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "plain label", in: "kucing-makan-ikan", want: "kucing-makan-ikan"},
		{name: "uppercase folded", in: "Kucing-Makan-Ikan", want: "kucing-makan-ikan"},
		{name: "whitespace trimmed", in: "  kucing-makan-ikan ", want: "kucing-makan-ikan"},
		{name: "domain suffix stripped", in: "kucing-makan-ikan." + Domain, want: "kucing-makan-ikan"},
		{name: "pasted url", in: "https://kucing-makan-ikan.try.dalang.io/", want: "kucing-makan-ikan"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeLabel(tt.in)
			if err != nil {
				t.Fatalf("NormalizeLabel(%q) returned error: %v", tt.in, err)
			}
			if got != tt.want {
				t.Fatalf("NormalizeLabel(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestNormalizeLabelErrors pins the client-side half of the protocol's label
// regex: anything rejected here must never cost a round trip to the daemon.
func TestNormalizeLabelAcceptsBothDomains(t *testing.T) {
	// The service moved from try.dalang.io to its own registrable domain. Links
	// from before the move are still in chat messages and tickets, so pasting
	// either one back as --subdomain has to work — nobody should have to know
	// which era their link came from.
	for _, in := range []string{
		"kucing-makan-ikan",
		"kucing-makan-ikan." + Domain,
		"https://kucing-makan-ikan." + Domain,
		"https://kucing-makan-ikan." + Domain + "/",
		"kucing-makan-ikan." + LegacyDomains[0],
		"https://kucing-makan-ikan." + LegacyDomains[0],
		"KUCING-MAKAN-IKAN." + Domain,
	} {
		got, err := NormalizeLabel(in)
		if err != nil {
			t.Fatalf("NormalizeLabel(%q) errored: %v", in, err)
		}
		if got != "kucing-makan-ikan" {
			t.Fatalf("NormalizeLabel(%q) = %q, want kucing-makan-ikan", in, got)
		}
	}
}

func TestDefaultServerURLTracksTheDomain(t *testing.T) {
	// A domain spelled out in several places is a migration that half-happens.
	// This fails if the control URL is ever hardcoded away from Domain again.
	want := "wss://tunnel." + Domain + "/_tunnel/connect"
	if DefaultServerURL != want {
		t.Fatalf("DefaultServerURL = %q, want %q", DefaultServerURL, want)
	}
}

func TestNormalizeLabelErrors(t *testing.T) {
	tests := []struct {
		name string
		in   string
	}{
		{name: "empty", in: ""},
		{name: "two words", in: "kucing-makan"},
		{name: "four words", in: "kucing-makan-ikan-besar"},
		{name: "digits", in: "kucing-makan-ikan2"},
		{name: "underscore", in: "kucing_makan_ikan"},
		{name: "leading dash", in: "-kucing-makan-ikan"},
		{name: "trailing dash", in: "kucing-makan-ikan-"},
		{name: "spaces inside", in: "kucing makan ikan"},
		{name: "non ascii", in: "kucing-makan-ikán"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, err := NormalizeLabel(tt.in); err == nil {
				t.Fatalf("NormalizeLabel(%q) = %q, want an error", tt.in, got)
			}
		})
	}
}

func TestControlURL(t *testing.T) {
	tests := []struct{ in, want string }{
		{in: "https://tunnel.try.dalang.io/_tunnel/connect", want: "wss://tunnel.try.dalang.io/_tunnel/connect"},
		{in: "http://127.0.0.1:8080/_tunnel/connect", want: "ws://127.0.0.1:8080/_tunnel/connect"},
		{in: "wss://tunnel.try.dalang.io/_tunnel/connect", want: "wss://tunnel.try.dalang.io/_tunnel/connect"},
		{in: "ws://127.0.0.1:8080/_tunnel/connect", want: "ws://127.0.0.1:8080/_tunnel/connect"},
	}
	for _, tt := range tests {
		if got := ControlURL(tt.in); got != tt.want {
			t.Fatalf("ControlURL(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
