package terminal

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"

	"github.com/gorilla/websocket"
	"golang.org/x/term"
)

// Terminal handles WebSocket terminal connection
type Terminal struct {
	conn         *websocket.Conn
	oldState     *term.State
	done         chan struct{}
	mu           sync.Mutex
	closeOnce    sync.Once
	escapeState  int  // 0=normal, 1=after newline, 2=after tilde
}

// ResizeMessage represents a terminal resize message
type ResizeMessage struct {
	Type string `json:"type"`
	Cols int    `json:"cols"`
	Rows int    `json:"rows"`
}

// NewTerminal creates a new terminal connection
func NewTerminal(wsURL string) (*Terminal, error) {
	headers := http.Header{}
	headers.Set("Origin", "https://dalang.io")
	headers.Set("User-Agent", "dalang-cli/1.0")

	conn, resp, err := websocket.DefaultDialer.Dial(wsURL, headers)
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
		select {
		case <-t.done:
			return nil
		default:
			_, message, err := t.conn.ReadMessage()
			if err != nil {
				// Restore terminal first
				t.restore()
				t.closeDone()

				fmt.Println("\r\nConnection closed.")

				// Force exit - stdin read is blocking and can't be interrupted
				os.Exit(0)
				return err
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
}

// writeLoop reads from stdin and writes to WebSocket
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

			// Check for SSH-style escape sequence: ~. after newline
			// State machine: 0=normal, 1=after newline, 2=after tilde
			for i := 0; i < n; i++ {
				switch t.escapeState {
				case 1: // After newline
					if buf[i] == '~' {
						t.escapeState = 2
						continue
					} else if buf[i] == '\r' || buf[i] == '\n' {
						t.escapeState = 1
					} else {
						t.escapeState = 0
					}
				case 2: // After tilde
					if buf[i] == '.' {
						// Escape sequence detected - disconnect
						t.restore()
						fmt.Println("\r\nConnection closed.")
						os.Exit(0)
						return nil
					} else if buf[i] == '\r' || buf[i] == '\n' {
						t.escapeState = 1
					} else {
						t.escapeState = 0
					}
				default: // Normal
					if buf[i] == '\r' || buf[i] == '\n' {
						t.escapeState = 1
					}
				}
			}

			t.mu.Lock()
			err = t.conn.WriteMessage(websocket.TextMessage, buf[:n])
			t.mu.Unlock()

			if err != nil {
				return err
			}
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
		t.conn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
		t.conn.Close()
	}
	t.mu.Unlock()
}

// restore restores terminal state
func (t *Terminal) restore() {
	if t.oldState != nil {
		fd := int(os.Stdin.Fd())
		term.Restore(fd, t.oldState)
		t.oldState = nil
	}
}
