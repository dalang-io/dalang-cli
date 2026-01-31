package terminal

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/gorilla/websocket"
	"golang.org/x/term"
)

// Terminal handles WebSocket terminal connection
type Terminal struct {
	conn     *websocket.Conn
	oldState *term.State
	done     chan struct{}
	mu       sync.Mutex
}

// ResizeMessage represents a terminal resize message
type ResizeMessage struct {
	Type string `json:"type"`
	Cols int    `json:"cols"`
	Rows int    `json:"rows"`
}

// NewTerminal creates a new terminal connection
func NewTerminal(wsURL string) (*Terminal, error) {
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
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

	// Start goroutines for reading and writing
	errChan := make(chan error, 2)

	// Read from WebSocket, write to stdout
	go func() {
		errChan <- t.readLoop()
	}()

	// Read from stdin, write to WebSocket
	go func() {
		errChan <- t.writeLoop()
	}()

	// Wait for either to finish
	select {
	case err := <-errChan:
		return err
	case <-t.done:
		return nil
	}
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
				if websocket.IsCloseError(err, websocket.CloseNormalClosure) {
					return nil
				}
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

// Close closes the terminal connection
func (t *Terminal) Close() {
	select {
	case <-t.done:
		// Already closed
		return
	default:
		close(t.done)
	}

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
