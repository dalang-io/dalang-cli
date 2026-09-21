package tunnel

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/textproto"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

// hopByHop headers are meaningless end-to-end and must not be replayed. The
// daemon strips them before sending a `request`, but a tunnel that trusts the
// other side to have sanitised its input is one bug away from forwarding an
// `Upgrade:` and confusing the local server, so strip again here.
//
// Keyed in canonical form and always looked up through headerName: the casing
// on the wire is not the browser's (Pingora lowercases, the daemon re-emits
// Title-Case), so nothing here may compare names exactly.
var hopByHop = map[string]bool{
	"Connection":          true,
	"Keep-Alive":          true,
	"Transfer-Encoding":   true,
	"Upgrade":             true,
	"Te":                  true,
	"Trailer":             true,
	"Proxy-Connection":    true,
	"Proxy-Authenticate":  true,
	"Proxy-Authorization": true,
}

// headerName canonicalises a header name from the wire ("etag" and "ETAG" both
// become "Etag"). Every comparison and every map key in this file goes through
// it, because a JSON-decoded map[string][]string gets none of the
// canonicalisation http.Header would have applied.
func headerName(name string) string {
	return textproto.CanonicalMIMEHeaderKey(name)
}

// canonicalizeHeaders copies a wire header map into canonical Title-Case,
// merging names that differ only in case rather than letting one win.
func canonicalizeHeaders(in map[string][]string) map[string][]string {
	if in == nil {
		return nil
	}
	out := make(map[string][]string, len(in))
	for name, values := range in {
		canonical := headerName(name)
		out[canonical] = append(out[canonical], values...)
	}
	return out
}

// Forwarder replays tunnelled requests against the local server.
type Forwarder struct {
	// BaseURL is the normalized local address, without a trailing slash.
	BaseURL string
	// maxBodyBytes mirrors the daemon's limit from the `assigned` frame. A
	// larger local response cannot be delivered, so it is turned into a 413
	// rather than a frame the daemon would reject (which would strand the id).
	// It is atomic because `assigned` can arrive while requests are in flight
	// (and again after a reconnect).
	maxBodyBytes atomic.Int64

	client  *http.Client
	host    string
	timeout time.Duration
}

// SetMaxBodyBytes records the limit the daemon announced in `assigned`.
func (f *Forwarder) SetMaxBodyBytes(n int64) { f.maxBodyBytes.Store(n) }

// MaxBodyBytes is the limit currently in force.
func (f *Forwarder) MaxBodyBytes() int64 {
	if n := f.maxBodyBytes.Load(); n > 0 {
		return n
	}
	return defaultMaxBodyBytes
}

// NewForwarder builds a Forwarder for an already-normalized base URL.
func NewForwarder(baseURL string, timeout time.Duration) (*Forwarder, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid local url %q: %w", baseURL, err)
	}
	if timeout <= 0 {
		timeout = DefaultLocalTimeout
	}
	return &Forwarder{
		BaseURL: strings.TrimSuffix(baseURL, "/"),
		host:    u.Host,
		timeout: timeout,
		client: &http.Client{
			Timeout: timeout,
			// A tunnel must hand the browser the redirect the local server
			// actually sent; following it here would hide 302s from the client.
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
			Transport: &http.Transport{
				// The browser's own Accept-Encoding is forwarded verbatim, so
				// the encoded bytes must pass through untouched — no transparent
				// gzip, or Content-Encoding would lie about the body.
				DisableCompression:  true,
				MaxIdleConnsPerHost: 32,
				// Local server, so a connection attempt either succeeds fast or
				// is not going to.
				ResponseHeaderTimeout: timeout,
			},
		},
	}, nil
}

// Result is what the client reports to the UI for one forwarded request.
type Result struct {
	Response  Response
	Status    int
	Duration  time.Duration
	BodyBytes int
	// Err is set when the local server could not be reached or did not answer;
	// Response still carries a 502 in that case, because an unanswered id hangs
	// the browser at the other end.
	Err error
}

// Do performs one tunnelled request against the local server. It always returns
// a Response for req.ID — there is no path on which a frame is dropped.
func (f *Forwarder) Do(ctx context.Context, req Request) Result {
	start := time.Now()

	body, err := base64.StdEncoding.DecodeString(req.BodyB64)
	if err != nil {
		return f.fail(req, start, fmt.Errorf("malformed request body from tunnel server: %w", err))
	}

	target := f.BaseURL + req.Path
	if req.Path == "" {
		target = f.BaseURL + "/"
	} else if !strings.HasPrefix(req.Path, "/") {
		target = f.BaseURL + "/" + req.Path
	}

	method := req.Method
	if method == "" {
		method = http.MethodGet
	}

	httpReq, err := http.NewRequestWithContext(ctx, method, target, bytes.NewReader(body))
	if err != nil {
		return f.fail(req, start, err)
	}

	// Default to the local server's own host so virtual-hosted apps answer;
	// an explicit Host header from the daemon (which already rewrites it) wins.
	httpReq.Host = f.host
	for name, values := range canonicalizeHeaders(req.Headers) {
		if hopByHop[name] {
			continue
		}
		switch name {
		case "Host":
			if len(values) > 0 && values[0] != "" {
				httpReq.Host = values[0]
			}
		case "Content-Length":
			// Set by net/http from the buffered body; a forwarded one could
			// disagree with the bytes we actually have.
		default:
			for _, v := range values {
				httpReq.Header.Add(name, v)
			}
		}
	}

	resp, err := f.client.Do(httpReq)
	if err != nil {
		return f.fail(req, start, err)
	}

	defer resp.Body.Close()

	limit := f.MaxBodyBytes()
	// Read one byte past the limit so an oversized body is detected rather than
	// silently truncated into a response that claims to be complete.
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return f.fail(req, start, fmt.Errorf("reading response from %s: %w", f.BaseURL, err))
	}
	if int64(len(respBody)) > limit {
		return Result{
			Response:  tooLargeResponse(req.ID, f.BaseURL, limit),
			Status:    http.StatusRequestEntityTooLarge,
			Duration:  time.Since(start),
			BodyBytes: len(respBody),
			Err:       fmt.Errorf("local response exceeds the %d-byte tunnel limit", limit),
		}
	}

	// Emitted in canonical Title-Case. The daemon compares case-insensitively
	// too, but sending one shape consistently is what keeps that true.
	headers := make(map[string][]string, len(resp.Header))
	for name, values := range resp.Header {
		canonical := headerName(name)
		if hopByHop[canonical] {
			continue
		}
		if canonical == "Content-Length" {
			// The body is buffered whole; trust the bytes, not the claim.
			headers[canonical] = []string{strconv.Itoa(len(respBody))}
			continue
		}
		headers[canonical] = append(headers[canonical], values...)
	}

	return Result{
		Response: Response{
			Type:    TypeResponse,
			ID:      req.ID,
			Status:  resp.StatusCode,
			Headers: headers,
			BodyB64: base64.StdEncoding.EncodeToString(respBody),
		},
		Status:    resp.StatusCode,
		Duration:  time.Since(start),
		BodyBytes: len(respBody),
	}
}

// fail answers a request the local server did not serve: 504 when it took the
// call but never finished (the protocol pins this at the 60s local timeout),
// 502 when it could not be reached at all. Either way the id is answered —
// dropping the frame is what hangs the browser.
func (f *Forwarder) fail(req Request, start time.Time, err error) Result {
	status := http.StatusBadGateway
	body := badGatewayHTML(f.BaseURL, err)
	if isTimeout(err) {
		status = http.StatusGatewayTimeout
		body = gatewayTimeoutHTML(f.BaseURL, f.timeout)
	}
	return Result{
		Response: Response{
			Type:   TypeResponse,
			ID:     req.ID,
			Status: status,
			Headers: map[string][]string{
				"Content-Type":   {"text/html; charset=utf-8"},
				"Content-Length": {strconv.Itoa(len(body))},
			},
			BodyB64: base64.StdEncoding.EncodeToString(body),
		},
		Status:    status,
		Duration:  time.Since(start),
		BodyBytes: len(body),
		Err:       err,
	}
}

// isTimeout distinguishes "your app is slow" from "your app is not there".
// A cancelled session context is neither, and lands on 502 by default.
func isTimeout(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout()
}

func gatewayTimeoutHTML(localURL string, timeout time.Duration) []byte {
	return []byte(`<!doctype html>
<html><head><meta charset="utf-8"><title>504 — local server did not answer</title></head>
<body style="font-family:system-ui,sans-serif;max-width:40rem;margin:4rem auto;padding:0 1rem">
<h1>504 Gateway Timeout</h1>
<p><code>` + htmlEscape(localURL) + `</code> accepted the request but did not answer within ` + timeout.String() + `.</p>
<p>The tunnel buffers whole responses in v1, so a streaming or long-polling endpoint will always time out here.</p>
</body></html>
`)
}

func badGatewayHTML(localURL string, err error) []byte {
	reason := ""
	if err != nil {
		reason = "<p><code>" + htmlEscape(shortError(err)) + "</code></p>"
	}
	return []byte(`<!doctype html>
<html><head><meta charset="utf-8"><title>502 — local server unreachable</title></head>
<body style="font-family:system-ui,sans-serif;max-width:40rem;margin:4rem auto;padding:0 1rem">
<h1>502 Bad Gateway</h1>
<p>The Dalang tunnel is up, but nothing answered at <code>` + htmlEscape(localURL) + `</code>.</p>
` + reason + `
<p>Start the local server and reload.</p>
</body></html>
`)
}

func tooLargeResponse(id, localURL string, limit int64) Response {
	body := []byte(`<!doctype html>
<html><head><meta charset="utf-8"><title>413 — response too large</title></head>
<body style="font-family:system-ui,sans-serif;max-width:40rem;margin:4rem auto;padding:0 1rem">
<h1>413 Payload Too Large</h1>
<p><code>` + htmlEscape(localURL) + `</code> returned more than ` + strconv.FormatInt(limit, 10) + ` bytes.</p>
<p>The tunnel buffers whole bodies in v1, so larger responses cannot be delivered.</p>
</body></html>
`)
	return Response{
		Type:   TypeResponse,
		ID:     id,
		Status: http.StatusRequestEntityTooLarge,
		Headers: map[string][]string{
			"Content-Type":   {"text/html; charset=utf-8"},
			"Content-Length": {strconv.Itoa(len(body))},
		},
		BodyB64: base64.StdEncoding.EncodeToString(body),
	}
}

// shortError trims net/http's wrapping down to something a user can act on
// ("connection refused") instead of a full Go error chain.
func shortError(err error) string {
	msg := err.Error()
	if i := strings.LastIndex(msg, ": "); i >= 0 && i+2 < len(msg) {
		return msg[i+2:]
	}
	return msg
}

func htmlEscape(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")
	return r.Replace(s)
}
