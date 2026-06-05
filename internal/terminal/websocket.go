package terminal

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"golang.org/x/term"
)

// Terminal handles WebSocket terminal connection
type Terminal struct {
	conn        *websocket.Conn
	oldState    *term.State
	done        chan struct{}
	mu          sync.Mutex
	restoreOnce sync.Once
	closeOnce   sync.Once
	escapeState int // 0=normal, 1=after newline, 2=after tilde
}

// ResizeMessage represents a terminal resize message
type ResizeMessage struct {
	Type string `json:"type"`
	Cols int    `json:"cols"`
	Rows int    `json:"rows"`
}

// deriveOrigin derives the frontend Origin from the WebSocket URL.
// wss://api.dalang.io -> https://dalang.io
// wss://test-api.dalang.io -> https://test.dalang.io
func deriveOrigin(wsURL string) string {
	scheme := "https"
	if strings.HasPrefix(wsURL, "ws://") {
		scheme = "http"
	}
	parts := strings.SplitN(wsURL, "/", 4)
	host := "dalang.io"
	if len(parts) >= 3 {
		host = parts[2]
	}
	if strings.HasPrefix(host, "api.") {
		host = strings.TrimPrefix(host, "api.")
	} else if strings.HasPrefix(host, "test-api.") {
		host = "test." + strings.TrimPrefix(host, "test-api.")
	}
	return scheme + "://" + host
}

// NewTerminal creates a new terminal connection.
// If token is non-empty it is sent via the Authorization header
// instead of being included in the URL query string, so that
// proxy / CDN access-logs never capture the credential.
func NewTerminal(wsURL string, token string) (*Terminal, error) {
	headers := http.Header{}
	headers.Set("Origin", deriveOrigin(wsURL))
	headers.Set("User-Agent", "dalang-cli/1.0")
	if token != "" {
		headers.Set("Authorization", "Bearer "+token)
	}

	wsDialer := &websocket.Dialer{
		HandshakeTimeout: 45 * time.Second,
	}

	conn, resp, err := wsDialer.Dial(wsURL, headers)
	if err != nil {
		if resp != nil {
			return nil, fmt.Errorf("failed to connect to WebSocket: %w (status: %d)", err, resp.StatusCode)
		}
		return nil, fmt.Errorf("failed to connect to WebSocket: %w", err)
	}

	t := &Terminal{
		conn: conn,
		done: make(chan struct{}),
	}

	return t, nil
}

// Run starts the terminal session
func (t *Terminal) Run() error {
	// Put terminal in raw mode
	fd := int(os.Stdin.Fd())
	if term.IsTerminal(fd) {
		oldState, err := term.MakeRaw(fd)
		if err != nil {
			return fmt.Errorf("failed to set raw mode: %w", err)
		}
		t.oldState = oldState
		defer t.restore()
	}

	// Send initial size
	t.SendResize()

	// Start writeLoop in background (reads from stdin, writes to WebSocket)
	go t.writeLoop()

	// Start keepalive ping every 30s to prevent Cloudflare idle timeout
	go t.keepAlive()

	// Read from WebSocket, write to stdout (this is the main loop)
	// When WebSocket closes, this returns and we exit
	err := t.readLoop()

	// Signal writeLoop to stop (it may be blocked on stdin)
	t.closeDone()

	return err
}

// readLoop reads from WebSocket and writes to stdout
func (t *Terminal) readLoop() error {
	for {
		_, message, err := t.conn.ReadMessage()
		if err != nil {
			// If the disconnect was initiated locally (Ctrl+C or the ~. escape
			// closed the connection to unblock this read), exit cleanly rather
			// than surfacing the resulting read error.
			select {
			case <-t.done:
				return nil
			default:
			}
			t.closeDone()
			fmt.Println("\r\nConnection closed.")
			// A read error means the session is over — the remote closed it
			// (e.g. you typed `exit`) or the network dropped. Either way return
			// to the local shell cleanly instead of surfacing a "terminal error".
			return nil
		}

		// Check if it's an error message
		var errMsg struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(message, &errMsg) == nil && errMsg.Error != "" {
			fmt.Fprintf(os.Stderr, "\r\nError: %s\r\n", errMsg.Error)
			continue
		}

		// Write to stdout
		os.Stdout.Write(message)
	}
}

// writeLoop reads from stdin and writes to WebSocket.
// It detects the SSH-style escape sequence ~. (tilde-dot after newline)
// and signals termination through the done channel instead of calling os.Exit.
// The tilde is buffered until the next character is known, so it is never
// sent to the remote when it is part of an escape sequence.
func (t *Terminal) writeLoop() error {
	buf := make([]byte, 1024)
	t.escapeState = 1 // Start after "newline" to allow ~. at very beginning

	for {
		select {
		case <-t.done:
			return nil
		default:
			n, err := os.Stdin.Read(buf)
			if err != nil {
				if err == io.EOF {
					return nil
				}
				return err
			}

			// Build output, filtering escape-sequence bytes.
			// The tilde after a newline is held back: if the next char
			// is '.', we disconnect; otherwise the held tilde is flushed.
			out := make([]byte, 0, n)

			for i := 0; i < n; i++ {
				ch := buf[i]
				switch t.escapeState {
				case 1: // After newline
					if ch == '~' {
						t.escapeState = 2
						// Hold the tilde — do not append yet
						continue
					} else if ch == '\r' || ch == '\n' {
						t.escapeState = 1
					} else {
						t.escapeState = 0
					}
					out = append(out, ch)
				case 2: // After tilde (held)
					if ch == '.' {
						// Escape sequence detected — disconnect gracefully.
						// Close() closes the WebSocket, which unblocks readLoop's
						// ReadMessage so Run() returns control to the local
						// terminal. Just closing `done` is not enough: readLoop
						// is blocked inside ReadMessage and never sees it.
						fmt.Print("\r\nConnection closed.\r\n")
						t.Close()
						return nil
					}
					// Not an escape — flush the held tilde, then this char
					out = append(out, '~')
					if ch == '\r' || ch == '\n' {
						t.escapeState = 1
					} else {
						t.escapeState = 0
					}
					out = append(out, ch)
				default: // Normal
					if ch == '\r' || ch == '\n' {
						t.escapeState = 1
					}
					out = append(out, ch)
				}
			}

			if len(out) > 0 {
				t.mu.Lock()
				err = t.conn.WriteMessage(websocket.TextMessage, out)
				t.mu.Unlock()

				if err != nil {
					return err
				}
			}
		}
	}
}

// keepAlive sends WebSocket ping frames every 30s to prevent Cloudflare idle timeout
func (t *Terminal) keepAlive() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-t.done:
			return
		case <-ticker.C:
			t.mu.Lock()
			t.conn.WriteMessage(websocket.PingMessage, nil)
			t.mu.Unlock()
		}
	}
}

// SendResize sends terminal size to the server
func (t *Terminal) SendResize() {
	fd := int(os.Stdout.Fd())
	if !term.IsTerminal(fd) {
		return
	}

	width, height, err := term.GetSize(fd)
	if err != nil {
		return
	}

	msg := ResizeMessage{
		Type: "resize",
		Cols: width,
		Rows: height,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return
	}

	t.mu.Lock()
	t.conn.WriteMessage(websocket.TextMessage, data)
	t.mu.Unlock()
}

// closeDone safely closes the done channel once
func (t *Terminal) closeDone() {
	t.closeOnce.Do(func() {
		close(t.done)
	})
}

// Close closes the terminal connection
func (t *Terminal) Close() {
	t.closeDone()
	t.restore()

	t.mu.Lock()
	if t.conn != nil {
		// Bound the courtesy close-frame write so a stuck/half-open socket can
		// never make disconnect (~. / Ctrl+C) hang. conn.Close() below is what
		// actually unblocks readLoop, so we proceed to it regardless.
		t.conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
		t.conn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
		t.conn.Close()
	}
	t.mu.Unlock()
}

// restore restores terminal state. Safe to call from multiple goroutines;
// the actual restore happens at most once.
func (t *Terminal) restore() {
	t.restoreOnce.Do(func() {
		if t.oldState != nil {
			fd := int(os.Stdin.Fd())
			term.Restore(fd, t.oldState)
			t.oldState = nil
		}
	})
}
