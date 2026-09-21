// Package tunnel implements the client half of the Dalang HTTP tunnel
// (`dalang tunnel`): it holds one WebSocket to the tunnel daemon, receives
// `request` frames for the public URL, replays them against the user's local
// server and sends `response` frames back.
//
// The wire format is frozen in dalang-tunnel/PROTOCOL.md (v1). The daemon is
// built against the same file, so nothing here may deviate silently: bodies are
// base64 inside JSON text frames, every `request` id must be answered exactly
// once, and the CLI — not the daemon — is the only side that can reach
// `local_url`.
package tunnel

// ProtocolVersion is the `version` field of the hello frame. A daemon that
// speaks something else answers `error` with code `unsupported_version`.
const ProtocolVersion = 1

// Frame type names, exactly as they appear on the wire.
const (
	TypeHello    = "hello"
	TypeAssigned = "assigned"
	TypeError    = "error"
	TypeRequest  = "request"
	TypeResponse = "response"
	TypePing     = "ping"
	TypePong     = "pong"
	TypeNotice   = "notice"
	TypeShutdown = "shutdown"
)

// Error codes the daemon may send in an `error` frame.
//
// `unauthorized` was removed from the protocol once both implementations found
// nothing that could produce it: every refusal keys on label *state*, and v1
// does not validate bearer tokens at all. An unrecognised code must still fail
// safe rather than be treated as success.
const (
	CodeLabelTaken         = "label_taken"
	CodeLabelExpired       = "label_expired"
	CodePoolExhausted      = "pool_exhausted"
	CodeRateLimited        = "rate_limited"
	CodeBadRequest         = "bad_request"
	CodeUnsupportedVersion = "unsupported_version"
)

// CodeHandshakeRejected is **not** a protocol code. It is synthesised when the
// WebSocket handshake itself is refused with 401/403 — something in front of the
// daemon (Cloudflare Access, a corporate proxy) rather than the daemon, which
// cannot answer with a frame at that point. Retrying will not help, so it is
// fatal.
const CodeHandshakeRejected = "handshake_rejected"

// Notice codes the daemon may send in an advisory `notice` frame.
const (
	NoticeRequestTooLarge = "request_too_large"
	NoticeResponseDropped = "response_dropped"
	NoticeNearingExpiry   = "nearing_expiry"
)

// Shutdown reasons the daemon may send in a `shutdown` frame.
//
// `evicted` was removed: it described a session displaced by a reconnect, which
// the label lifecycle makes impossible — an `active` label is refused even to
// the correct token, so nothing can take a live session's place.
const (
	ReasonExpired       = "expired"
	ReasonServerRestart = "server_restart"
)

// envelope peeks at the `type` field so the frame can be decoded into the
// concrete struct below.
type envelope struct {
	Type string `json:"type"`
}

// Hello is the first frame, client → server.
//
// RequestedLabel is a *reclaim* request, not a wish: a hello without one is
// always a fresh draw, and one with a label but no ReclaimToken is always
// refused — otherwise anyone could name a reserved label and take a stranger's
// address the moment they stopped their tunnel.
type Hello struct {
	Type           string `json:"type"`
	Version        int    `json:"version"`
	Client         string `json:"client"`
	LocalURL       string `json:"local_url"`
	Token          string `json:"token,omitempty"`
	RequestedLabel string `json:"requested_label,omitempty"`
	ReclaimToken   string `json:"reclaim_token,omitempty"`
}

// Assigned is the daemon's reply to Hello: the public URL is now live.
//
// ReclaimToken is the only proof that the label is ours. It is reissued on every
// `assigned` and the previous one stops working the instant the new one is sent,
// so the CLI must overwrite what it stored every time, not only the first.
//
// ReclaimWindowSeconds is a duration, deliberately not a deadline: the window
// runs from the moment the connection *closes*, which has not happened when this
// frame is sent. The CLI computes its own estimate from when its socket actually
// closed; the daemon decides on its own clock whatever the client displayed.
type Assigned struct {
	Type                 string `json:"type"`
	Label                string `json:"label"`
	URL                  string `json:"url"`
	ExpiresAt            string `json:"expires_at"`
	MaxBodyBytes         int64  `json:"max_body_bytes"`
	ReclaimToken         string `json:"reclaim_token"`
	ReclaimWindowSeconds int64  `json:"reclaim_window_seconds"`
}

// ErrorFrame is a refusal; the daemon closes the connection after sending it.
type ErrorFrame struct {
	Type    string `json:"type"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Request is one inbound HTTP request the daemon received on the public URL.
// Path already carries the query string, and Headers preserves repeats.
//
// Header names must be compared case-insensitively. Pingora stores headers in an
// http::HeaderMap, which lowercases every name on insert, so the browser's
// casing is gone before the daemon sees it; the daemon re-emits canonical
// Title-Case (`ETag` comes back as `Etag`). Headers here is a plain map decoded
// by encoding/json, which — unlike http.Header — gets no canonicalisation from
// Go, so an exact-match lookup on a guessed name can miss.
type Request struct {
	Type     string              `json:"type"`
	ID       string              `json:"id"`
	Method   string              `json:"method"`
	Path     string              `json:"path"`
	Headers  map[string][]string `json:"headers"`
	BodyB64  string              `json:"body_b64"`
	RemoteIP string              `json:"remote_ip"`
}

// Response answers exactly one Request. Dropping one hangs the browser at the
// other end, so every failure path still produces one of these.
type Response struct {
	Type    string              `json:"type"`
	ID      string              `json:"id"`
	Status  int                 `json:"status"`
	Headers map[string][]string `json:"headers"`
	BodyB64 string              `json:"body_b64"`
}

// PingFrame is the keepalive. Cloudflare drops an idle proxied connection at
// roughly 100s, so this is load-bearing, not a nicety.
type PingFrame struct {
	Type string `json:"type"`
	T    int64  `json:"t"`
}

// Notice is advisory and never fatal: the CLI prints it and carries on. It is
// how the daemon reports things that concern one request (a body it refused)
// rather than the tunnel as a whole, which `error` cannot express because the
// server closes after sending one.
//
// RequestID is optional and often absent: `request_too_large` is decided while
// the body is still being read, before the request has an id, and the message
// carries the method and path instead.
type Notice struct {
	Type      string `json:"type"`
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id,omitempty"`
}

// Shutdown tells the client the tunnel is going away and why.
type Shutdown struct {
	Type    string `json:"type"`
	Reason  string `json:"reason"`
	Message string `json:"message"`
}
