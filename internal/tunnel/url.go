package tunnel

import (
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// LabelPattern is the label regex from PROTOCOL.md §Labels. It is duplicated in
// the daemon; both sides must agree or a locally-accepted `--subdomain` costs a
// round trip to be refused.
var LabelPattern = regexp.MustCompile(`^[a-z]+-[a-z]+-[a-z]+$`)

// tunnelDomainSuffix is accepted (and stripped) so pasting back a previously
// issued URL works as a `--subdomain` value.
const tunnelDomainSuffix = ".try.dalang.io"

var schemePrefix = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9+.\-]*://`)

// NormalizeLocalURL turns the many things people type for `--url` into an
// absolute base URL the forwarder can append a request path to.
//
//	8000              -> http://localhost:8000
//	:8000             -> http://localhost:8000
//	localhost:8000    -> http://localhost:8000
//	0.0.0.0:8000      -> http://127.0.0.1:8000   (0.0.0.0 is a bind address, not
//	                                              a destination — dialing it
//	                                              fails outright on Windows)
//	http://host:8000/ -> http://host:8000        (trailing slash trimmed so the
//	                                              request path can be appended)
//
// The result never ends in "/" and never carries a query or fragment.
func NormalizeLocalURL(in string) (string, error) {
	s := strings.TrimSpace(in)
	if s == "" {
		return "", fmt.Errorf("--url is required, e.g. --url http://localhost:8000")
	}

	// Bare port shorthands, before any URL parsing: "8000" parses as a scheme-
	// less opaque URL and ":8000" does not parse at all.
	if isAllDigits(s) {
		s = "localhost:" + s
	} else if strings.HasPrefix(s, ":") && isAllDigits(s[1:]) {
		s = "localhost" + s
	}

	if !schemePrefix.MatchString(s) {
		// "localhost:8000" has a colon but no scheme; url.Parse would read
		// "localhost" as the scheme, so prefix http:// before parsing.
		s = "http://" + s
	}

	u, err := url.Parse(s)
	if err != nil {
		return "", fmt.Errorf("invalid --url %q: %w", in, err)
	}

	switch u.Scheme {
	case "http", "https":
	default:
		return "", fmt.Errorf("invalid --url %q: only http and https are supported, got %q", in, u.Scheme)
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("invalid --url %q: a query string or fragment does not belong in the local address", in)
	}
	if u.Host == "" {
		return "", fmt.Errorf("invalid --url %q: no host", in)
	}

	// Hostnames are case-insensitive; folding them keeps the printed URL and the
	// Host header we send to the local server identical run to run.
	host := strings.ToLower(u.Hostname())
	port := u.Port()
	if host == "" {
		return "", fmt.Errorf("invalid --url %q: no host", in)
	}
	if u.Port() == "" && strings.Contains(u.Host, ":") {
		// url.Parse tolerates "localhost:abc" by treating it as an empty port.
		return "", fmt.Errorf("invalid --url %q: port must be a number", in)
	}
	if port != "" {
		p, perr := strconv.Atoi(port)
		if perr != nil || p < 1 || p > 65535 {
			return "", fmt.Errorf("invalid --url %q: port must be between 1 and 65535", in)
		}
	}

	// A wildcard bind address means "my machine" to the person typing it.
	switch host {
	case "0.0.0.0":
		host = "127.0.0.1"
	case "::", "::0":
		host = "::1"
	}

	hostPort := host
	if strings.Contains(host, ":") { // IPv6 literal needs brackets back
		hostPort = "[" + host + "]"
	}
	if port != "" {
		hostPort = net.JoinHostPort(host, port)
	}

	path := strings.TrimSuffix(u.EscapedPath(), "/")
	return u.Scheme + "://" + hostPort + path, nil
}

// NormalizeLabel cleans up and validates a `--subdomain` value. Validation is
// local and happens before dialling: being told "no" after a round trip to
// Jakarta is a worse experience than being told immediately.
func NormalizeLabel(in string) (string, error) {
	s := strings.ToLower(strings.TrimSpace(in))
	s = strings.TrimSuffix(s, ".")
	s = strings.TrimSuffix(s, tunnelDomainSuffix)
	// Tolerate a pasted full URL, e.g. https://kucing-makan-ikan.try.dalang.io
	if i := strings.Index(s, "://"); i >= 0 {
		s = s[i+3:]
		s = strings.TrimSuffix(strings.TrimSuffix(s, "/"), tunnelDomainSuffix)
	}
	if s == "" {
		return "", fmt.Errorf("--subdomain needs a value, e.g. --subdomain kucing-makan-ikan")
	}
	if !LabelPattern.MatchString(s) {
		return "", fmt.Errorf("invalid --subdomain %q: must be three lowercase a-z words joined by dashes (%s), e.g. kucing-makan-ikan",
			in, LabelPattern.String())
	}
	return s, nil
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
