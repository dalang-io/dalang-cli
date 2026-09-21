package tunnel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dalang-io/dalang-cli/internal/netdial"
	"github.com/gorilla/websocket"
)

const (
	// DefaultServerURL is the control endpoint. tunnel.try.dalang.io serves
	// nothing else (PROTOCOL.md §Reserved hosts).
	DefaultServerURL = "wss://tunnel.try.dalang.io/_tunnel/connect"

	// DefaultPingInterval is fixed by the protocol: Cloudflare drops an idle
	// proxied connection at ~100s, and 3 × 30s leaves headroom to notice.
	DefaultPingInterval = 30 * time.Second

	// DefaultMaxMissedPongs is the protocol's "three missed pongs and the CLI
	// reconnects", sending the label and its reclaim token.
	DefaultMaxMissedPongs = 3

	// DefaultLocalTimeout is pinned by the protocol's timing table: 60s, then
	// the CLI answers that id with a 504 rather than leaving it unanswered.
	DefaultLocalTimeout = 60 * time.Second

	// MaxInitialAttempts bounds a first connection that has never succeeded.
	// Once a session has reached `assigned` the client retries forever instead
	// — surviving a daemon restart under a running tunnel is the point — but
	// before that the likely causes are a typo, a firewall or an outage, and
	// retrying silently for an hour helps nobody.
	MaxInitialAttempts = 5

	// defaultMaxBodyBytes is the v1 limit, used until `assigned` states one.
	defaultMaxBodyBytes = 10 << 20

	// readLimit bounds a single frame. A 10 MB body is ~13.4 MB of base64 plus
	// headers; 32 MB leaves slack without letting a hostile frame eat the heap.
	readLimit = 32 << 20

	writeTimeout = 20 * time.Second
)

// Events are the UI hooks. All are optional and all are called from the
// client's goroutines, so implementations must be safe to call concurrently
// (OnRequest in particular fires once per forwarded request).
type Events struct {
	OnAssigned  func(Assigned)
	OnNotice    func(Notice)
	OnRequest   func(Request, Result)
	OnReconnect func(attempt int, delay time.Duration, cause error)
	OnConnected func(attempt int)
	OnShutdown  func(Shutdown)
	OnDebug     func(format string, args ...any)
}

// Options configures a Client.
type Options struct {
	ServerURL      string // defaults to DefaultServerURL
	LocalURL       string // already normalized (see NormalizeLocalURL)
	Token          string // optional bearer token; empty = anonymous tunnel
	RequestedLabel string // optional reclaim target, must satisfy LabelPattern
	ReclaimToken   string // proof that RequestedLabel is ours; required with it
	ClientName     string // e.g. "dalang-cli/1.18.2"
	PingInterval   time.Duration
	MaxMissedPongs int
	LocalTimeout   time.Duration
	Events         Events

	// backoff overrides the retry ladder. Unexported on purpose: the ladder is
	// pinned by the protocol, so only tests in this package may shorten it.
	backoff func(attempt int) time.Duration
}

// FatalError is a refusal or an end-of-tunnel that retrying cannot fix: the
// caller should print it and exit non-zero.
type FatalError struct {
	Code    string
	Message string
}

func (e *FatalError) Error() string {
	if e.Message == "" {
		return e.Code
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// UnreachableError is a first connection that never got through. It is separate
// from FatalError because nothing was refused — the server was simply not
// reachable — and the advice that follows from it is different.
type UnreachableError struct {
	ServerURL string
	Attempts  int
	Err       error
}

func (e *UnreachableError) Error() string {
	return fmt.Sprintf("could not reach the tunnel server at %s after %d attempts: %v", e.ServerURL, e.Attempts, e.Err)
}

func (e *UnreachableError) Unwrap() error { return e.Err }

// Client owns one tunnel: a control WebSocket, its keepalive, and the local
// forwarder behind it. It reconnects on its own and keeps its label so the
// public URL the user already pasted somewhere keeps working.
type Client struct {
	opts  Options
	fwd   *Forwarder
	label string // sticky across reconnects
	// reclaim is reissued with every `assigned`, so the newest one is the only
	// one that works; a reconnect that sends a stale token is refused.
	reclaim string
}

// New validates options and builds a Client. It does not dial.
func New(opts Options) (*Client, error) {
	if opts.ServerURL == "" {
		opts.ServerURL = DefaultServerURL
	}
	if opts.PingInterval <= 0 {
		opts.PingInterval = DefaultPingInterval
	}
	if opts.MaxMissedPongs <= 0 {
		opts.MaxMissedPongs = DefaultMaxMissedPongs
	}
	if opts.ClientName == "" {
		opts.ClientName = "dalang-cli"
	}
	if opts.LocalURL == "" {
		return nil, errors.New("tunnel: LocalURL is required")
	}
	if opts.RequestedLabel != "" && !LabelPattern.MatchString(opts.RequestedLabel) {
		return nil, fmt.Errorf("tunnel: invalid label %q", opts.RequestedLabel)
	}
	fwd, err := NewForwarder(opts.LocalURL, opts.LocalTimeout)
	if err != nil {
		return nil, err
	}
	return &Client{opts: opts, fwd: fwd, label: opts.RequestedLabel, reclaim: opts.ReclaimToken}, nil
}

// Run holds the tunnel open until ctx is cancelled (clean shutdown, nil), a
// FatalError arrives, or a first connection gives up. Transport-level failures
// are retried with backoff.
func (c *Client) Run(ctx context.Context) error {
	attempt := 0 // consecutive failures
	everAssigned := false

	for {
		assigned, err := c.session(ctx, attempt+1)
		if ctx.Err() != nil {
			return nil // Ctrl-C: the close frame was already sent
		}
		if assigned {
			everAssigned = true
			// The session worked for a while; start the backoff ladder over
			// rather than punishing a long-lived tunnel for one drop.
			attempt = 0
		}

		var fatal *FatalError
		if errors.As(err, &fatal) {
			return err
		}

		attempt++
		if !everAssigned && attempt >= MaxInitialAttempts {
			return &UnreachableError{ServerURL: c.opts.ServerURL, Attempts: attempt, Err: err}
		}

		delay := c.backoffFor(attempt)
		c.emitReconnect(attempt, delay, err)
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(delay):
		}
	}
}

// Label reports the label currently held (empty before the first `assigned`).
func (c *Client) Label() string { return c.label }

// ReclaimToken reports the newest reclaim token (empty before the first
// `assigned` unless one was supplied up front).
func (c *Client) ReclaimToken() string { return c.reclaim }

// session runs exactly one WebSocket connection. It returns whether the tunnel
// was ever assigned (used to reset backoff) and why the connection ended.
func (c *Client) session(ctx context.Context, attempt int) (bool, error) {
	conn, err := c.dial(ctx)
	if err != nil {
		return false, err
	}
	conn.SetReadLimit(readLimit)

	s := &session{conn: conn, client: c}
	sessCtx, cancel := context.WithCancel(ctx)

	// ReadMessage below cannot be interrupted by a context, so closing the
	// connection is what unblocks it — on Ctrl-C and on missed pongs alike.
	var closeOnce sync.Once
	closeConn := func(normal bool) {
		closeOnce.Do(func() {
			if normal {
				s.sendClose()
			}
			conn.Close()
		})
	}
	go func() {
		<-sessCtx.Done()
		// A cancelled parent context is the user's Ctrl-C: say goodbye
		// properly, since the protocol has no client-side shutdown frame.
		closeConn(ctx.Err() != nil)
	}()

	var handlers sync.WaitGroup
	// Order matters: cancel first so in-flight local requests abort, then close,
	// then wait — otherwise a slow local server delays reconnection by its whole
	// timeout.
	defer func() {
		cancel()
		closeConn(false)
		handlers.Wait()
	}()

	// A label is only ever sent together with its reclaim token; the daemon
	// refuses a bare label, because otherwise naming a reserved label would be
	// enough to take a stranger's address the moment they stopped their tunnel.
	hello := Hello{
		Type:           TypeHello,
		Version:        ProtocolVersion,
		Client:         c.opts.ClientName,
		LocalURL:       c.opts.LocalURL,
		Token:          c.opts.Token,
		RequestedLabel: c.label,
		ReclaimToken:   c.reclaim,
	}
	if err := s.send(hello); err != nil {
		return false, fmt.Errorf("sending hello: %w", err)
	}
	c.debug("hello sent (label=%q reclaim=%v local=%s)", c.label, c.reclaim != "", c.opts.LocalURL)

	c.emitConnected(attempt)
	go s.keepAlive(sessCtx)

	var assigned bool

	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			if ctx.Err() != nil {
				return assigned, nil
			}
			if reason := s.deadReason(); reason != nil {
				return assigned, reason
			}
			return assigned, fmt.Errorf("connection lost: %w", err)
		}

		var env envelope
		if err := json.Unmarshal(data, &env); err != nil {
			c.debug("ignoring unparseable frame: %v", err)
			continue
		}

		switch env.Type {
		case TypeAssigned:
			var a Assigned
			if err := json.Unmarshal(data, &a); err != nil {
				return assigned, fmt.Errorf("malformed assigned frame: %w", err)
			}
			assigned = true
			c.label = a.Label
			// Overwrite unconditionally: the previous token stopped working the
			// instant this frame was sent.
			c.reclaim = a.ReclaimToken
			if a.MaxBodyBytes > 0 {
				c.fwd.SetMaxBodyBytes(a.MaxBodyBytes)
			}
			c.emitAssigned(a)

		case TypeError:
			var e ErrorFrame
			if err := json.Unmarshal(data, &e); err != nil {
				return assigned, fmt.Errorf("malformed error frame: %w", err)
			}
			return assigned, &FatalError{Code: e.Code, Message: e.Message}

		case TypeRequest:
			var r Request
			if err := json.Unmarshal(data, &r); err != nil {
				c.debug("ignoring malformed request frame: %v", err)
				continue
			}
			handlers.Add(1)
			// One goroutine per request: the daemon caps in-flight requests at
			// 32 per tunnel, so this cannot run away.
			go func() {
				defer handlers.Done()
				s.handleRequest(sessCtx, r)
			}()

		case TypeNotice:
			var n Notice
			if err := json.Unmarshal(data, &n); err != nil {
				c.debug("ignoring malformed notice frame: %v", err)
				continue
			}
			// Advisory by definition: print and carry on, never end the session.
			c.emitNotice(n)

		case TypePong:
			s.missedPongs.Store(0)

		case TypePing:
			var p PingFrame
			_ = json.Unmarshal(data, &p)
			if err := s.send(PingFrame{Type: TypePong, T: p.T}); err != nil {
				c.debug("failed to answer server ping: %v", err)
			}

		case TypeShutdown:
			var sd Shutdown
			if err := json.Unmarshal(data, &sd); err != nil {
				return assigned, fmt.Errorf("malformed shutdown frame: %w", err)
			}
			c.emitShutdown(sd)
			if sd.Reason == ReasonServerRestart {
				// The daemon is coming back; keep the label and wait it out.
				return assigned, fmt.Errorf("tunnel server restarting")
			}
			return assigned, &FatalError{Code: sd.Reason, Message: sd.Message}

		default:
			c.debug("ignoring unknown frame type %q", env.Type)
		}
	}
}

func (c *Client) dial(ctx context.Context) (*websocket.Conn, error) {
	headers := http.Header{}
	headers.Set("User-Agent", c.opts.ClientName)
	if c.opts.Token != "" {
		// The token also travels in the hello frame, as PROTOCOL.md specifies.
		// Sending the handshake header too matches the terminal client's
		// contract (internal/terminal/websocket.go) and, unlike a ?token=
		// query param, never lands in Cloudflare's access logs — this
		// connection is proxied, so that is not hypothetical.
		headers.Set("Authorization", "Bearer "+c.opts.Token)
	}

	dialer := &websocket.Dialer{
		HandshakeTimeout: 30 * time.Second,
		// Public-DNS fallback for hosts without /etc/resolv.conf (Termux).
		NetDialContext: netdial.DialContext,
	}

	c.debug("dialing %s", c.opts.ServerURL)
	conn, resp, err := dialer.DialContext(ctx, c.opts.ServerURL, headers)
	if err != nil {
		if resp != nil {
			if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
				// Refused at the HTTP layer, so no `error` frame can arrive and
				// no protocol code applies — retrying will not change it.
				return nil, &FatalError{
					Code:    CodeHandshakeRejected,
					Message: fmt.Sprintf("status %d from %s", resp.StatusCode, c.opts.ServerURL),
				}
			}
			return nil, fmt.Errorf("connecting to %s: %w (status %d)", c.opts.ServerURL, err, resp.StatusCode)
		}
		return nil, fmt.Errorf("connecting to %s: %w", c.opts.ServerURL, err)
	}
	return conn, nil
}

// backoffFor is the ladder in force for this client.
func (c *Client) backoffFor(attempt int) time.Duration {
	if c.opts.backoff != nil {
		return c.opts.backoff(attempt)
	}
	return backoffFor(attempt)
}

func backoffFor(attempt int) time.Duration {
	delays := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 15 * time.Second}
	if attempt-1 < len(delays) && attempt > 0 {
		return delays[attempt-1]
	}
	return 30 * time.Second
}

// session holds the per-connection state. gorilla/websocket allows exactly one
// concurrent writer, so every write goes through send().
type session struct {
	conn   *websocket.Conn
	client *Client

	writeMu     sync.Mutex
	missedPongs atomic.Int32
	dead        atomic.Pointer[error]
}

func (s *session) send(frame any) error {
	data, err := json.Marshal(frame)
	if err != nil {
		return err
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_ = s.conn.SetWriteDeadline(time.Now().Add(writeTimeout))
	return s.conn.WriteMessage(websocket.TextMessage, data)
}

// sendClose tells the server we are going away. PROTOCOL.md has no client-side
// shutdown frame, so the WebSocket close frame is the goodbye.
func (s *session) sendClose() {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_ = s.conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
	_ = s.conn.WriteMessage(websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, "client shutdown"))
}

// keepAlive sends a protocol `ping` every PingInterval and tears the connection
// down after MaxMissedPongs unanswered ones so Run can reconnect. WebSocket
// control pings are not used: the protocol defines its own JSON ping, and only
// that one proves the daemon (not just Cloudflare) is still there.
func (s *session) keepAlive(ctx context.Context) {
	ticker := time.NewTicker(s.client.opts.PingInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.send(PingFrame{Type: TypePing, T: time.Now().Unix()}); err != nil {
				s.die(fmt.Errorf("keepalive write failed: %w", err))
				s.conn.Close()
				return
			}
			if missed := s.missedPongs.Add(1); int(missed) >= s.client.opts.MaxMissedPongs {
				s.die(fmt.Errorf("no pong after %d pings — connection is dead", missed))
				// Unblocks ReadMessage; Run() then reconnects with the same
				// label, so the user's public URL survives.
				s.conn.Close()
				return
			}
		}
	}
}

func (s *session) die(err error) {
	s.dead.CompareAndSwap(nil, &err)
}

// deadReason returns the keepalive's diagnosis, if it is what killed the
// connection, so the read error ("use of closed network connection") does not
// mask the real cause.
func (s *session) deadReason() error {
	if p := s.dead.Load(); p != nil {
		return *p
	}
	return nil
}

func (s *session) handleRequest(ctx context.Context, req Request) {
	res := s.client.fwd.Do(ctx, req)
	s.client.emitRequest(req, res)
	if err := s.send(res.Response); err != nil {
		// The id is now unanswered and the browser at the other end will hang
		// until the daemon times it out; nothing more we can do on a dead
		// socket, but say so under -v.
		s.client.debug("failed to send response for %s: %v", req.ID, err)
	}
}

func (c *Client) debug(format string, args ...any) {
	if c.opts.Events.OnDebug != nil {
		c.opts.Events.OnDebug(format, args...)
	}
}

func (c *Client) emitAssigned(a Assigned) {
	if c.opts.Events.OnAssigned != nil {
		c.opts.Events.OnAssigned(a)
	}
}

func (c *Client) emitNotice(n Notice) {
	if c.opts.Events.OnNotice != nil {
		c.opts.Events.OnNotice(n)
	}
}

func (c *Client) emitRequest(req Request, res Result) {
	if c.opts.Events.OnRequest != nil {
		c.opts.Events.OnRequest(req, res)
	}
}

func (c *Client) emitReconnect(attempt int, delay time.Duration, cause error) {
	if c.opts.Events.OnReconnect != nil {
		c.opts.Events.OnReconnect(attempt, delay, cause)
	}
}

func (c *Client) emitConnected(attempt int) {
	if c.opts.Events.OnConnected != nil {
		c.opts.Events.OnConnected(attempt)
	}
}

func (c *Client) emitShutdown(sd Shutdown) {
	if c.opts.Events.OnShutdown != nil {
		c.opts.Events.OnShutdown(sd)
	}
}

// ControlURL turns an https/http tunnel endpoint into its wss/ws form so
// DALANG_TUNNEL_URL can be set to either.
func ControlURL(raw string) string {
	switch {
	case strings.HasPrefix(raw, "https://"):
		return "wss://" + strings.TrimPrefix(raw, "https://")
	case strings.HasPrefix(raw, "http://"):
		return "ws://" + strings.TrimPrefix(raw, "http://")
	default:
		return raw
	}
}
