package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// wsTransport implements Transport over a WebSocket connection (BF-16).
// JSON-RPC messages travel as text frames. A background reader goroutine
// feeds recvCh/errCh (stdio transport precedent) so Recv can honor context
// cancellation; writes are serialized (gorilla requires one concurrent
// writer). OAuth config is accepted for interface parity but the OAuth
// handshake is not implemented for any transport yet (documented divergence).
type wsTransport struct {
	conn   *websocket.Conn
	sendMu sync.Mutex
	mu     sync.Mutex
	closed bool

	recvCh chan wsRecvResult
	errCh  chan error
}

// wsRecvResult is one decoded message (or read error) from the reader loop.
type wsRecvResult struct {
	msg *JSONRPCMessage
	err error
}

// newWSTransport dials the WebSocket endpoint and starts the reader loop.
func newWSTransport(ctx context.Context, cfg WSTransportConfig) (*wsTransport, error) {
	if cfg.URL == "" {
		return nil, fmt.Errorf("mcp: ws: URL is required")
	}
	header := http.Header{}
	for k, v := range cfg.Headers {
		header.Set(k, v)
	}
	dialer := websocket.Dialer{HandshakeTimeout: 30 * time.Second}
	conn, _, err := dialer.DialContext(ctx, cfg.URL, header)
	if err != nil {
		return nil, fmt.Errorf("mcp: ws: dial %s: %w", cfg.URL, err)
	}
	t := &wsTransport{
		conn:   conn,
		recvCh: make(chan wsRecvResult, 16),
		errCh:  make(chan error, 1),
	}
	go t.readLoop()
	return t, nil
}

// readLoop decodes incoming text frames until the connection closes.
func (t *wsTransport) readLoop() {
	for {
		_, data, err := t.conn.ReadMessage()
		if err != nil {
			t.mu.Lock()
			closed := t.closed
			t.mu.Unlock()
			if closed {
				return
			}
			select {
			case t.errCh <- fmt.Errorf("mcp: ws: read: %w", err):
			default:
			}
			return
		}
		var msg JSONRPCMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			// Skip malformed frames (mirrors session store corrupt-line policy).
			continue
		}
		t.recvCh <- wsRecvResult{msg: &msg}
	}
}

// Send writes one JSON-RPC message as a text frame, honoring ctx via the
// write deadline.
func (t *wsTransport) Send(ctx context.Context, msg *JSONRPCMessage) error {
	t.mu.Lock()
	closed := t.closed
	t.mu.Unlock()
	if closed {
		return fmt.Errorf("mcp: ws: transport closed")
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("mcp: ws: marshal: %w", err)
	}
	t.sendMu.Lock()
	defer t.sendMu.Unlock()
	deadline := time.Now().Add(30 * time.Second)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	if err := t.conn.SetWriteDeadline(deadline); err != nil {
		return fmt.Errorf("mcp: ws: set deadline: %w", err)
	}
	if err := t.conn.WriteMessage(websocket.TextMessage, data); err != nil {
		return fmt.Errorf("mcp: ws: write: %w", err)
	}
	return nil
}

// Recv returns the next message, blocking until one arrives, the context is
// cancelled, or the connection fails.
func (t *wsTransport) Recv(ctx context.Context) (*JSONRPCMessage, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case r := <-t.recvCh:
		return r.msg, r.err
	case err := <-t.errCh:
		// Re-broadcast for other waiters (stdio precedent).
		select {
		case t.errCh <- err:
		default:
		}
		return nil, err
	}
}

// Close shuts down the connection; idempotent.
func (t *wsTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return nil
	}
	t.closed = true
	// Best-effort close frame; ignore errors on an already-dead connection.
	_ = t.conn.WriteControl(websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
		time.Now().Add(time.Second))
	return t.conn.Close()
}
