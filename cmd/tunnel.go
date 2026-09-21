package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"time"

	"github.com/dalang-io/dalang-cli/internal/config"
	"github.com/dalang-io/dalang-cli/internal/tunnel"
)

// tunnelArgs is the parsed, still-raw form of `dalang tunnel`'s flags.
// Normalization and validation happen in cmdTunnel so parsing stays testable
// without touching the network or the filesystem.
type tunnelArgs struct {
	localURL  string
	subdomain string
	server    string
	help      bool
}

func parseTunnelArgs(args []string) (tunnelArgs, error) {
	var out tunnelArgs

	needsValue := func(i *int, flag string) (string, error) {
		if *i+1 >= len(args) {
			return "", fmt.Errorf("%s needs a value", flag)
		}
		*i++
		return args[*i], nil
	}

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--help" || arg == "-h":
			out.help = true
		case arg == "--url":
			v, err := needsValue(&i, "--url")
			if err != nil {
				return out, err
			}
			out.localURL = v
		case strings.HasPrefix(arg, "--url="):
			out.localURL = strings.TrimPrefix(arg, "--url=")
		case arg == "--subdomain":
			v, err := needsValue(&i, "--subdomain")
			if err != nil {
				return out, err
			}
			out.subdomain = v
		case strings.HasPrefix(arg, "--subdomain="):
			out.subdomain = strings.TrimPrefix(arg, "--subdomain=")
		case arg == "--server":
			v, err := needsValue(&i, "--server")
			if err != nil {
				return out, err
			}
			out.server = v
		case strings.HasPrefix(arg, "--server="):
			out.server = strings.TrimPrefix(arg, "--server=")
		case strings.HasPrefix(arg, "-"):
			return out, fmt.Errorf("unknown option %q (run 'dalang help tunnel')", arg)
		default:
			// `dalang tunnel 8000` is what people type first; accept it as --url.
			if out.localURL != "" {
				return out, fmt.Errorf("unexpected argument %q — pass the local address once, e.g. --url http://localhost:8000", arg)
			}
			out.localURL = arg
		}
	}
	return out, nil
}

func cmdTunnel(args []string) error {
	parsed, err := parseTunnelArgs(args)
	if err != nil {
		return err
	}
	if parsed.help {
		printTunnelHelp()
		return nil
	}

	localURL, err := tunnel.NormalizeLocalURL(parsed.localURL)
	if err != nil {
		return err
	}

	// Validated locally and before dialling: a bad label is worth a message
	// now, not a round trip to be told no.
	label := ""
	if parsed.subdomain != "" {
		label, err = tunnel.NormalizeLabel(parsed.subdomain)
		if err != nil {
			return err
		}
	}

	// --subdomain is a *reclaim* flag: every new session draws a new address,
	// and an address only comes back to the machine that holds its token. The
	// request is still sent without one, because the daemon is the authority on
	// whether the reservation is live — but say so first, or the refusal that
	// follows looks like a bug.
	reclaimToken := ""
	if label != "" {
		if ticket, ok := config.GetTunnelReclaim(label); ok {
			reclaimToken = ticket.ReclaimToken
			PrintDebug("found a reclaim token for %s (estimated window ends %s)", label, ticket.UntilEstimate)
		} else if !quietOutput && !jsonOutput {
			printWarn("No reclaim token for %q on this machine — asking the server anyway.", label)
			printWarn("Addresses can only be reclaimed by the machine that held them, within 6 hours of the tunnel stopping.")
		}
	}

	serverURL := tunnel.DefaultServerURL
	if env := os.Getenv("DALANG_TUNNEL_URL"); env != "" {
		serverURL = tunnel.ControlURL(env)
	}
	if parsed.server != "" {
		serverURL = tunnel.ControlURL(parsed.server)
	}

	// A token is optional: anonymous tunnels work, they just expire sooner.
	token := ""
	if creds, cerr := config.LoadCredentials(); cerr == nil {
		token = creds.AccessToken
	}

	version := Version
	if version == "" {
		version = "dev"
	}

	reporter := &tunnelReporter{localURL: localURL, authenticated: token != ""}

	client, err := tunnel.New(tunnel.Options{
		ServerURL:      serverURL,
		LocalURL:       localURL,
		Token:          token,
		RequestedLabel: label,
		ReclaimToken:   reclaimToken,
		ClientName:     "dalang-cli/" + version,
		Events: tunnel.Events{
			OnAssigned: func(a tunnel.Assigned) {
				// Persist first: if the process dies a moment later, the token
				// is the only way back to the address just printed.
				persistReclaim(a)
				reporter.assigned(a)
			},
			OnNotice:    reporter.notice,
			OnRequest:   reporter.request,
			OnReconnect: reporter.reconnect,
			OnShutdown:  reporter.shutdown,
			OnDebug:     PrintDebug,
		},
	})
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, tunnelStopSignals()...)
	defer signal.Stop(sigChan)
	go func() {
		<-sigChan
		// Cancelling makes Run send the WebSocket close frame before exiting,
		// so the daemon frees the label instead of waiting for it to time out.
		cancel()
	}()

	if !quietOutput && !jsonOutput {
		printInfo("Opening tunnel to %s ...", localURL)
	}

	runErr := client.Run(ctx)

	// The reservation window runs from the close, which has just happened:
	// replace the worst-case deadline written at assignment time with one
	// measured from the real thing. Still an estimate — the daemon decides.
	if held := reporter.assignedLabel(); held != "" {
		if rerr := config.RefreshTunnelReclaimDeadline(held, time.Now()); rerr != nil {
			PrintDebug("could not refresh the reclaim deadline for %s: %v", held, rerr)
		}
	}

	var unreachable *tunnel.UnreachableError
	if errors.As(runErr, &unreachable) {
		return tunnelUnreachableMessage(unreachable)
	}

	var fatal *tunnel.FatalError
	if errors.As(runErr, &fatal) {
		if fatal.Code == tunnel.CodeLabelExpired && label != "" {
			// The reservation is gone for good; keeping the token would only
			// make the next attempt fail the same way.
			if derr := config.DeleteTunnelReclaim(label); derr != nil {
				PrintDebug("could not drop the stale reclaim token for %s: %v", label, derr)
			}
		}
		return tunnelFatalMessage(fatal, label)
	}
	if runErr != nil {
		return runErr
	}

	if !quietOutput && !jsonOutput {
		fmt.Println()
		printSuccess("Tunnel closed.")
		reporter.printReclaimHint()
	}
	return nil
}

// persistReclaim stores the ticket that proves this address is ours. A failure
// is not fatal — the tunnel works fine — but it costs the user the ability to
// get this address back, so it is worth saying out loud.
//
// The deadline is derived here, not received: the wire carries a duration
// because the window runs from the moment the socket closes, and that has not
// happened yet. Writing `now + window` at assignment time is the correct worst
// case — if the process is killed in the next instant, that is exactly the
// deadline — and it is refreshed from the real close when the command exits.
func persistReclaim(a tunnel.Assigned) {
	if a.Label == "" || a.ReclaimToken == "" {
		return
	}
	estimate := ""
	if a.ReclaimWindowSeconds > 0 {
		estimate = time.Now().Add(time.Duration(a.ReclaimWindowSeconds) * time.Second).UTC().Format(time.RFC3339)
	}
	err := config.SaveTunnelReclaim(config.TunnelReclaim{
		Label:         a.Label,
		URL:           a.URL,
		ReclaimToken:  a.ReclaimToken,
		WindowSeconds: a.ReclaimWindowSeconds,
		UntilEstimate: estimate,
	})
	if err != nil {
		PrintDebug("saving reclaim token failed: %v", err)
		printWarn("Could not save the reclaim token (%v) — this address cannot be reclaimed after the tunnel stops.", err)
	}
}

// tunnelUnreachableMessage explains a first connection that never got through.
// Nothing was refused, so the advice is about the path, not about the request.
func tunnelUnreachableMessage(e *tunnel.UnreachableError) error {
	return fmt.Errorf("could not reach the tunnel server at %s — gave up after %d attempts (%s). Check your network, any firewall or proxy, and DALANG_TUNNEL_URL if you set it",
		e.ServerURL, e.Attempts, shortTunnelError(e.Err))
}

// tunnelFatalMessage turns a protocol refusal into something the user can act
// on; the daemon's own `message` is appended when it adds anything.
func tunnelFatalMessage(fatal *tunnel.FatalError, label string) error {
	detail := ""
	if fatal.Message != "" {
		detail = ": " + fatal.Message
	}
	switch fatal.Code {
	case tunnel.CodeLabelTaken:
		if label != "" {
			return fmt.Errorf("%s is in use, or the reclaim token for it is not the current one — drop --subdomain to get a new address%s", tunnelAddress(label), detail)
		}
		return fmt.Errorf("that address is already in use%s", detail)
	case tunnel.CodeLabelExpired:
		if label != "" {
			return fmt.Errorf("%s is not yours to reclaim — addresses are released 6 hours after a tunnel stops. Drop --subdomain and you will be given a new one%s", tunnelAddress(label), detail)
		}
		return fmt.Errorf("that address is not yours to reclaim — addresses are released 6 hours after a tunnel stops%s", detail)
	case tunnel.CodePoolExhausted:
		return fmt.Errorf("the tunnel server has no free addresses right now — try again in a moment%s", detail)
	case tunnel.CodeHandshakeRejected:
		// Not a protocol refusal: something in front of the daemon turned the
		// WebSocket upgrade away before a frame could be exchanged.
		return fmt.Errorf("the WebSocket handshake was rejected before reaching the tunnel server%s", detail)
	case tunnel.CodeRateLimited:
		return fmt.Errorf("rate limited by the tunnel server — one tunnel per IP anonymously, three per account%s", detail)
	case tunnel.CodeUnsupportedVersion:
		return fmt.Errorf("this CLI speaks tunnel protocol v%d and the server does not — run 'dalang update'%s", tunnel.ProtocolVersion, detail)
	case tunnel.ReasonExpired:
		return fmt.Errorf("tunnel expired%s", detail)
	case tunnel.CodeBadRequest:
		// A protocol code, not an unknown one. The daemon uses it for a token
		// api.dalang.io refused, and its message already says what to do, so
		// this arm exists to stop the default arm telling the user their CLI is
		// out of date when it is their token that is.
		if fatal.Message != "" {
			return fmt.Errorf("%s", fatal.Message)
		}
		return fmt.Errorf("the tunnel server rejected the request")
	default:
		// Fail safe on anything this CLI does not know: a code it cannot name
		// is still a reason to stop, and is most likely a server newer than it.
		return fmt.Errorf("the tunnel server ended the session with an unrecognised code %q%s — this CLI may be older than the server; try 'dalang update'", fatal.Code, detail)
	}
}

// tunnelAddress renders a label the way the user saw it, so an error about
// "kucing-makan-ikan" names the URL they actually pasted somewhere.
func tunnelAddress(label string) string {
	return label + ".try.dalang.io"
}

// tunnelReporter renders everything the tunnel wants to say. It is called from
// the client's goroutines, so stdout writes are serialised by mu.
type tunnelReporter struct {
	localURL      string
	authenticated bool

	mu            sync.Mutex
	downOnce      sync.Once
	public        string
	label         string
	reclaimWindow time.Duration
}

func (r *tunnelReporter) assigned(a tunnel.Assigned) {
	r.mu.Lock()
	defer r.mu.Unlock()

	reconnected := r.public == a.URL
	r.public = a.URL
	r.label = a.Label
	r.reclaimWindow = time.Duration(a.ReclaimWindowSeconds) * time.Second

	if jsonOutput {
		r.emitJSON(map[string]any{
			"type":           "assigned",
			"label":          a.Label,
			"url":            a.URL,
			"local_url":      r.localURL,
			"expires_at":     a.ExpiresAt,
			"max_body_bytes": a.MaxBodyBytes,
			"reconnected":    reconnected,
		})
		return
	}

	if quietOutput {
		// Minimal output still has to carry the URL — it is the whole point of
		// the command, and `dalang tunnel -q --url :8000 | head -1` should work.
		fmt.Println(a.URL)
		return
	}

	if reconnected {
		fmt.Println()
		printSuccess("Reconnected — %s is live again", a.URL)
		return
	}

	fmt.Println()
	fmt.Printf("  %sForwarding%s  %s%s%s  →  %s\n", colorBold, colorReset, colorGreen+colorBold, a.URL, colorReset, r.localURL)
	if exp := formatTunnelExpiry(a.ExpiresAt); exp != "" {
		fmt.Printf("  %sExpires%s     %s\n", colorBold, colorReset, exp)
	}
	if a.MaxBodyBytes > 0 {
		fmt.Printf("  %sMax body%s    %s per request and response\n", colorBold, colorReset, formatBytes(a.MaxBodyBytes))
	}
	if !r.authenticated {
		fmt.Printf("  %sAccount%s     anonymous — run %sdalang auth%s for longer tunnels\n", colorBold, colorReset, colorCyan, colorReset)
	}
	fmt.Println()
	fmt.Printf("  Press %sCtrl+C%s to stop.\n\n", colorBold, colorReset)
}

func (r *tunnelReporter) request(req tunnel.Request, res tunnel.Result) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if quietOutput {
		return
	}

	if jsonOutput {
		entry := map[string]any{
			"type":        "request",
			"id":          req.ID,
			"method":      req.Method,
			"path":        req.Path,
			"status":      res.Status,
			"duration_ms": res.Duration.Milliseconds(),
			"bytes":       res.BodyBytes,
			"remote_ip":   req.RemoteIP,
		}
		if res.Err != nil {
			entry["error"] = res.Err.Error()
		}
		r.emitJSON(entry)
		return
	}

	// Padded so the duration column lines up while someone is watching the log
	// scroll — a long path pushes it out, which is better than truncating it.
	line := fmt.Sprintf("  %s%3d%s  %-6s %-36s %7s",
		tunnelStatusColor(res.Status), res.Status, colorReset,
		req.Method, req.Path, formatTunnelDuration(res.Duration))
	if res.Err != nil {
		line += fmt.Sprintf("  %s(%s)%s", colorRed, shortTunnelError(res.Err), colorReset)
	}
	fmt.Println(line)

	if res.Err != nil {
		// Say it plainly the first time: a 502 here is the user's own app being
		// down, not the tunnel failing, and that distinction saves a support
		// ticket.
		r.downOnce.Do(func() {
			printWarn("Could not reach %s — is your local server running?", r.localURL)
		})
	}
}

// notice renders an advisory frame. It never ends the session: the daemon uses
// it for things that concern one request (a body it refused) rather than the
// tunnel as a whole.
func (r *tunnelReporter) notice(n tunnel.Notice) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if jsonOutput {
		entry := map[string]any{
			"type":    "notice",
			"code":    n.Code,
			"message": n.Message,
		}
		if n.RequestID != "" {
			entry["request_id"] = n.RequestID
		}
		r.emitJSON(entry)
		return
	}
	if quietOutput {
		return
	}

	msg := n.Message
	if msg == "" {
		msg = tunnelNoticeFallback(n.Code)
	}
	if n.RequestID != "" {
		printWarn("%s (%s, request %s)", msg, n.Code, n.RequestID)
		return
	}
	printWarn("%s (%s)", msg, n.Code)
}

// tunnelNoticeFallback keeps the output useful if the daemon sends a bare code.
func tunnelNoticeFallback(code string) string {
	switch code {
	case tunnel.NoticeRequestTooLarge:
		return "A request body was over the 10 MB limit and the caller got a 413"
	case tunnel.NoticeResponseDropped:
		return "A response could not be delivered"
	case tunnel.NoticeNearingExpiry:
		return "This tunnel is close to its expiry time"
	default:
		return "Notice from the tunnel server"
	}
}

// assignedLabel reports the label this session ended up holding, if any.
func (r *tunnelReporter) assignedLabel() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.label
}

// printReclaimHint tells the user how to get this address back. The window is
// six hours and the token is on this machine, so the command is only useful
// here — which is exactly why it is worth printing rather than documenting.
func (r *tunnelReporter) printReclaimHint() {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.label == "" {
		return
	}
	// Computed from now, which is when the socket closed — the window the
	// daemon announced runs from the close, not from the assignment. It is an
	// estimate on purpose: the daemon decides on its own clock.
	window := "for a while"
	if r.reclaimWindow > 0 {
		window = fmt.Sprintf("for about %s (until roughly %s)",
			r.reclaimWindow.Round(time.Minute),
			time.Now().Add(r.reclaimWindow).Local().Format("15:04 MST"))
	}
	fmt.Println()
	fmt.Printf("  %s is held for you %s. To take it back:\n", tunnelAddress(r.label), window)
	fmt.Printf("      %sdalang tunnel --url %s --subdomain %s%s\n\n", colorCyan, r.localURL, r.label, colorReset)
}

func (r *tunnelReporter) reconnect(attempt int, delay time.Duration, cause error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if jsonOutput {
		entry := map[string]any{
			"type":     "reconnect",
			"attempt":  attempt,
			"delay_ms": delay.Milliseconds(),
		}
		if cause != nil {
			entry["cause"] = cause.Error()
		}
		r.emitJSON(entry)
		return
	}
	if quietOutput {
		return
	}
	reason := "connection lost"
	if cause != nil {
		reason = shortTunnelError(cause)
	}
	printWarn("Reconnecting in %s (attempt %d): %s", delay.Round(time.Second), attempt, reason)
}

func (r *tunnelReporter) shutdown(sd tunnel.Shutdown) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if jsonOutput {
		r.emitJSON(map[string]any{
			"type":    "shutdown",
			"reason":  sd.Reason,
			"message": sd.Message,
		})
		return
	}
	if quietOutput {
		return
	}
	msg := sd.Message
	if msg == "" {
		msg = sd.Reason
	}
	printWarn("Tunnel server says: %s (%s)", msg, sd.Reason)
}

// emitJSON writes one JSON object per line (NDJSON) so the stream stays
// parseable while the tunnel is still running. Callers hold r.mu.
func (r *tunnelReporter) emitJSON(v map[string]any) {
	data, err := json.Marshal(v)
	if err != nil {
		return
	}
	fmt.Println(string(data))
}

func tunnelStatusColor(status int) string {
	switch {
	case status >= 500:
		return colorRed
	case status >= 400:
		return colorYellow
	case status >= 300:
		return colorCyan
	default:
		return colorGreen
	}
}

func formatTunnelDuration(d time.Duration) string {
	switch {
	case d < time.Millisecond:
		return fmt.Sprintf("%dµs", d.Microseconds())
	case d < time.Second:
		return fmt.Sprintf("%dms", d.Milliseconds())
	default:
		return fmt.Sprintf("%.2fs", d.Seconds())
	}
}

// formatTunnelExpiry renders the server's RFC3339 expiry with the remaining
// time, and degrades to the raw string if it is not parseable.
func formatTunnelExpiry(raw string) string {
	if raw == "" {
		return ""
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return raw
	}
	left := time.Until(t)
	if left <= 0 {
		return t.Local().Format("2006-01-02 15:04:05 MST")
	}
	return fmt.Sprintf("%s (in %s)", t.Local().Format("2006-01-02 15:04:05 MST"), left.Round(time.Minute))
}

func shortTunnelError(err error) string {
	msg := err.Error()
	if i := strings.LastIndex(msg, ": "); i >= 0 && i+2 < len(msg) {
		return msg[i+2:]
	}
	return msg
}

func printTunnelHelp() {
	fmt.Printf(`%sdalang tunnel%s - Expose a local HTTP server on a public URL

%sUSAGE:%s
    dalang tunnel --url <local-address> [--subdomain <label>]

%sDESCRIPTION:%s
    Opens a tunnel to tunnel.try.dalang.io and prints a public HTTPS URL.
    Every request to that URL is forwarded to your local server and the
    response is sent back. Nothing is installed on your machine and no
    ports are opened in your firewall.

    The tunnel lives as long as the command runs. Press Ctrl+C to stop it.

%sOPTIONS:%s
    --url <addr>          Local address to expose (required). Accepts
                          8000, :8000, localhost:8000 and http://localhost:8000
    --subdomain <label>   Reclaim an address this machine held earlier
                          (see ADDRESSES below) — not a way to pick a name
    --server <url>        Override the tunnel control endpoint
                          (also: DALANG_TUNNEL_URL)

%sADDRESSES:%s
    Every run gets a new address; you cannot choose one. When a tunnel stops,
    its address is held for you for 6 hours, and --subdomain takes it back:

        dalang tunnel --url 8000 --subdomain kucing-makan-ikan

    That only works from the machine that held it, because the proof is a
    reclaim token stored in ~/.dalang/tunnels.json (mode 0600). Asking for an
    address you never held is refused — it may belong to someone else.

%sEXAMPLES:%s
    dalang tunnel --url http://localhost:8000
    dalang tunnel --url 3000
    dalang tunnel --url :5173 --subdomain kucing-makan-ikan   # reclaim
    dalang tunnel --url 8000 --quiet          # prints only the public URL
    dalang tunnel --url 8000 --json           # one JSON object per line

%sLIMITS (v1):%s
    - Request and response bodies are buffered, 10 MB each
    - No streaming, Server-Sent Events, or WebSockets through the tunnel
    - 2 hours per tunnel anonymously, 8 hours when logged in
    - 1 concurrent tunnel per IP anonymously, 3 per account

%sNOTE:%s
    - Log in with 'dalang auth' for longer tunnels
    - A 502 in the request log means your local server was not reachable;
      a 504 means it accepted the request but did not answer within 60s
    - The public URL is reachable by anyone who has it
`,
		colorCyan, colorReset,
		colorYellow, colorReset,
		colorYellow, colorReset,
		colorYellow, colorReset,
		colorYellow, colorReset,
		colorYellow, colorReset,
		colorYellow, colorReset,
		colorYellow, colorReset,
	)
}
