package netdial

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"
)

// A non-DNS dial error (connection refused) must pass through unchanged — the
// fallback path is only for DNS failures.
func TestDialContext_NonDNSErrorPassesThrough(t *testing.T) {
	// 127.0.0.1:1 — a literal IP (no DNS) that refuses connections.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err := DialContext(ctx, "tcp", "127.0.0.1:1")
	if err == nil {
		t.Fatal("expected a connection error dialing 127.0.0.1:1")
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		t.Fatalf("literal IP should not yield a DNS error, got %v", err)
	}
}

// The public fallback resolver must resolve a well-known name even when the
// system resolver would (this asserts the fallback resolver itself works).
func TestPublicResolver_ResolvesKnownHost(t *testing.T) {
	if testing.Short() {
		t.Skip("network-dependent; skipped in -short")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ips, err := publicResolver.LookupHost(ctx, "one.one.one.one")
	if err != nil {
		t.Skipf("public DNS unreachable in this environment: %v", err)
	}
	if len(ips) == 0 {
		t.Fatal("expected at least one IP for one.one.one.one")
	}
}
