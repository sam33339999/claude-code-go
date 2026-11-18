package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// notifyWSServer is an MCP server that pushes notifications:
// - right after the initialize response (tests handler-before-connect)
// - after responding to tools/list (tests handler-after-connect)
type notifyWSServer struct {
	srv *httptest.Server
}

func newNotifyWSServer(t *testing.T) *notifyWSServer {
	t.Helper()
	up := websocket.Upgrader{}
	s := &notifyWSServer{}
	s.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		writeMsg := func(v any) {
			raw, _ := json.Marshal(v)
			_ = conn.WriteMessage(websocket.TextMessage, raw)
		}
		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var msg JSONRPCMessage
			if json.Unmarshal(data, &msg) != nil || msg.ID == nil {
				continue
			}
			switch msg.Method {
			case "initialize":
				writeMsg(map[string]any{
					"jsonrpc": "2.0", "id": msg.ID,
					"result": map[string]any{
						"protocolVersion": "2024-11-05",
						"serverInfo":      map[string]any{"name": "notify-test", "version": "1.0"},
						"capabilities":    map[string]any{},
					},
				})
				// Push a notification right after the handshake.
				writeMsg(map[string]any{
					"jsonrpc": "2.0",
					"method":  "notifications/message",
					"params":  map[string]any{"data": "early-event"},
				})
			case "tools/list":
				writeMsg(map[string]any{
					"jsonrpc": "2.0", "id": msg.ID,
					"result": map[string]any{"tools": []any{}},
				})
				// Push another notification after tools/list.
				writeMsg(map[string]any{
					"jsonrpc": "2.0",
					"method":  "notifications/message",
					"params":  map[string]any{"data": "late-event"},
				})
			default:
				writeMsg(map[string]any{
					"jsonrpc": "2.0", "id": msg.ID,
					"result": map[string]any{},
				})
			}
		}
	}))
	t.Cleanup(s.srv.Close)
	return s
}

func (s *notifyWSServer) wsConfig() ServerConfig {
	return ServerConfig{
		Transport: TransportWS,
		WS:        &WSTransportConfig{URL: "ws" + strings.TrimPrefix(s.srv.URL, "http")},
	}
}

// collector gathers notifications thread-safely.
type collector struct {
	mu     sync.Mutex
	events []string
}

func (c *collector) handler() ServerNotificationHandler {
	return func(server, method string, params json.RawMessage) {
		c.mu.Lock()
		defer c.mu.Unlock()
		c.events = append(c.events, server+"|"+method+"|"+string(params))
	}
}

func (c *collector) snapshot() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.events...)
}

func waitForEvents(t *testing.T, c *collector, n int) []string {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if ev := c.snapshot(); len(ev) >= n {
			return ev
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d notifications, got %v", n, c.snapshot())
	return nil
}

func TestEV01_NotificationBeforeConnect(t *testing.T) {
	srv := newNotifyWSServer(t)
	pool := NewPool()
	col := &collector{}
	pool.SetNotificationHandler(col.handler())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := pool.Connect(ctx, "svc", srv.wsConfig()); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Disconnect("svc")

	events := waitForEvents(t, col, 1)
	if !strings.Contains(events[0], "svc|notifications/message|") || !strings.Contains(events[0], "early-event") {
		t.Errorf("notification must carry server name + method + params, got %q", events[0])
	}
}

func TestEV01_NotificationAfterConnect(t *testing.T) {
	srv := newNotifyWSServer(t)
	pool := NewPool()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := pool.Connect(ctx, "svc", srv.wsConfig()); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Disconnect("svc")

	// Install the handler AFTER the connection exists.
	col := &collector{}
	pool.SetNotificationHandler(col.handler())

	// tools/list triggers the server's second notification; also proves
	// normal request/response routing still works with a handler installed.
	conn := pool.connections["svc"]
	tools, err := conn.Client.ListTools(ctx)
	if err != nil {
		t.Fatalf("tools/list must still work: %v", err)
	}
	if len(tools) != 0 {
		t.Errorf("expected empty tool list, got %d", len(tools))
	}
	// The early-event notification may arrive late (scheduling); wait for the
	// late-event specifically rather than assuming it is the first collected.
	deadline := time.Now().Add(3 * time.Second)
	var events []string
	for time.Now().Before(deadline) {
		events = col.snapshot()
		found := false
		for _, e := range events {
			if strings.Contains(e, "late-event") {
				found = true
			}
		}
		if found {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	foundLate := false
	for _, e := range events {
		if strings.Contains(e, "late-event") {
			foundLate = true
		}
	}
	if !foundLate {
		t.Errorf("expected late-event notification, got %v", events)
	}
}

func TestEV01_NilHandlerSafe(t *testing.T) {
	srv := newNotifyWSServer(t)
	pool := NewPool()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := pool.Connect(ctx, "svc", srv.wsConfig()); err != nil {
		t.Fatalf("connect with nil handler: %v", err)
	}
	defer pool.Disconnect("svc")
	if _, err := pool.connections["svc"].Client.ListTools(ctx); err != nil {
		t.Errorf("notifications with nil handler must not break requests: %v", err)
	}
}

func TestEV01_ReconnectKeepsHandler(t *testing.T) {
	srv := newNotifyWSServer(t)
	pool := NewPool()
	col := &collector{}
	pool.SetNotificationHandler(col.handler())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := pool.Connect(ctx, "svc", srv.wsConfig()); err != nil {
		t.Fatal(err)
	}
	waitForEvents(t, col, 1)
	if err := pool.Reconnect(ctx, "svc"); err != nil {
		t.Fatalf("reconnect: %v", err)
	}
	defer pool.Disconnect("svc")
	// The fresh connection must push early-event again to the same handler.
	events := waitForEvents(t, col, 2)
	if !strings.Contains(events[1], "early-event") {
		t.Errorf("reconnect must preserve the notification handler, got %v", events)
	}
}
