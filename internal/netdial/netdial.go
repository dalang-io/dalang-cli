// Package netdial provides a network dialer that falls back to public DNS when
// the system has no usable resolver configuration.
//
// The CLI ships as a static (CGO_ENABLED=0) binary, so it uses Go's pure-Go DNS
// resolver, which reads /etc/resolv.conf. Android/Termux has no /etc/resolv.conf,
// so the resolver falls back to localhost and every lookup fails with:
//
//	dial tcp: lookup api.dalang.io on [::1]:53: read: connection refused
//
// DialContext dials normally first — honoring /etc/hosts and a working system
// resolver — and only when the system produces a DNS error does it re-resolve
// the host against public DNS. So configured environments are unaffected; the
// fallback only rescues hosts (notably phones) that have no resolver config.
package netdial

import (
	"context"
	"errors"
	"net"
	"time"
)

// publicDNS are queried, in order, only as a fallback after a system-DNS failure.
var publicDNS = []string{"1.1.1.1:53", "8.8.8.8:53", "9.9.9.9:53"}

// publicResolver resolves names by talking to publicDNS directly, bypassing the
// (missing) system resolver configuration.
var publicResolver = &net.Resolver{
	PreferGo: true,
	Dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
		d := net.Dialer{Timeout: 5 * time.Second}
		var lastErr error
		for _, srv := range publicDNS {
			if c, err := d.DialContext(ctx, "udp", srv); err == nil {
				return c, nil
			} else {
				lastErr = err
			}
		}
		return nil, lastErr
	},
}

// DialContext is a drop-in for http.Transport.DialContext and
// websocket.Dialer.NetDialContext. It dials addr via the system resolver, and if
// that fails specifically because of DNS, re-resolves the host over public DNS
// and dials the resulting address(es). Non-DNS errors are returned unchanged.
func DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	d := &net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}

	conn, err := d.DialContext(ctx, network, addr)
	if err == nil {
		return conn, nil
	}

	// Only intervene on DNS failures — leave connection/refused/timeout as-is.
	var dnsErr *net.DNSError
	if !errors.As(err, &dnsErr) {
		return nil, err
	}

	host, port, splitErr := net.SplitHostPort(addr)
	if splitErr != nil {
		return nil, err
	}

	ips, rerr := publicResolver.LookupHost(ctx, host)
	if rerr != nil || len(ips) == 0 {
		return nil, err // keep the original, more descriptive error
	}

	var lastErr error
	for _, ip := range ips {
		if c, derr := d.DialContext(ctx, network, net.JoinHostPort(ip, port)); derr == nil {
			return c, nil
		} else {
			lastErr = derr
		}
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, err
}
