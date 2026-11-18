package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// wsEchoServer upgrades to WebSocket and answers JSON-RPC: initialize gets a
// valid MCP result, tools/list an empty list, anything else is echoed back
// as the result. Notifications are ignored.
func wsEchoServer(t *testing.T) *httptest.Server {
	t.Helper()
	up := websocket.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var msg JSONRPCMessage
			if err := json.Unmarshal(data, &msg); err != nil {
				continue
			}
			if msg.ID == nil {
				continue // notification
			}
			var result any
			switch msg.Method {
			case "initialize":
				result = map[string]any{
					"protocolVersion": "2024-11-05",
					"serverInfo":      map[string]any{"name": "ws-test", "version": "1.0"},
					"capabilities":    map[string]any{"tools": map[string]any{}},
				}
			case "tools/list":
				result = map[string]any{"tools": []any{}}
			default:
				result = map[string]any{"echo": msg.Method}
			}
			raw, _ := json.Marshal(result)
			resp := JSONRPCMessage{JSONRPC: "2.0", ID: msg.ID, Result: raw}
			out, _ := json.Marshal(resp)
			if err := conn.WriteMessage(websocket.TextMessage, out); err != nil {
				return
			}
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func wsURL(srv *httptest.Server) string {
	return "ws" + strings.TrimPrefix(srv.URL, "http")
}

func TestWSTransport_SendRecvRoundTrip(t *testing.T) {
	srv := wsEchoServer(t)
	tr, err := NewTransport(TransportWS, WSTransportConfig{URL: wsURL(srv)})
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer tr.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req := &JSONRPCMessage{JSONRPC: "2.0", ID: 1, Method: "ping"}
	if err := tr.Send(ctx, req); err != nil {
		t.Fatalf("send: %v", err)
	}
	resp, err := tr.Recv(ctx)
	if err != nil {
		t.Fatalf("recv: %v", err)
	}
	if resp.ID != float64(1) || !strings.Contains(string(resp.Result), "ping") {
		t.Errorf("unexpected response: %+v", resp)
	}
}

func TestWSTransport_RecvContextCancel(t *testing.T) {
	srv := wsEchoServer(t)
	tr, err := NewTransport(TransportWS, WSTransportConfig{URL: wsURL(srv)})
	if err != nil {
		t.Fatal(err)
	}
	defer tr.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := tr.Recv(ctx); err == nil {
		t.Error("cancelled ctx must abort Recv")
	}
}

func TestWSTransport_CloseIdempotentAndSendAfterClose(t *testing.T) {
	srv := wsEchoServer(t)
	tr, err := NewTransport(TransportWS, WSTransportConfig{URL: wsURL(srv)})
	if err != nil {
		t.Fatal(err)
	}
	if err := tr.Close(); err != nil {
		t.Errorf("first close: %v", err)
	}
	if err := tr.Close(); err != nil {
		t.Errorf("second close must be nil, got %v", err)
	}
	if err := tr.Send(context.Background(), &JSONRPCMessage{JSONRPC: "2.0", ID: 1, Method: "x"}); err == nil {
		t.Error("send after close must fail")
	}
}

func TestWSTransport_DialErrors(t *testing.T) {
	if _, err := NewTransport(TransportWS, WSTransportConfig{URL: ""}); err == nil {
		t.Error("empty URL must fail")
	}
	if _, err := NewTransport(TransportWS, WSTransportConfig{URL: "ws://127.0.0.1:1/nope"}); err == nil {
		t.Error("unreachable URL must fail")
	}
	if _, err := NewTransport(TransportWS, HTTPTransportConfig{}); err == nil {
		t.Error("wrong config type must fail")
	}
}

// TestWSPool_FullHandshake proves the whole stack over WebSocket:
// Pool.Connect → initialize handshake → tools/list → Disconnect.
func TestWSPool_FullHandshake(t *testing.T) {
	srv := wsEchoServer(t)
	pool := NewPool()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cfg := ServerConfig{Transport: TransportWS, WS: &WSTransportConfig{URL: wsURL(srv)}}
	if err := pool.Connect(ctx, "ws-srv", cfg); err != nil {
		t.Fatalf("connect: %v", err)
	}
	var conn *ServerConnection
	for _, c := range pool.GetConnected() {
		if c.Name == "ws-srv" {
			conn = c
		}
	}
	if conn == nil || conn.Client == nil {
		t.Fatal("expected connected ws server")
	}
	tools, err := conn.Client.ListTools(ctx)
	if err != nil {
		t.Fatalf("tools/list over ws: %v", err)
	}
	if len(tools) != 0 {
		t.Errorf("expected empty tool list, got %d", len(tools))
	}
	if err := pool.Disconnect("ws-srv"); err != nil {
		t.Errorf("disconnect: %v", err)
	}
}

func TestWSPool_NilConfig(t *testing.T) {
	pool := NewPool()
	err := pool.Connect(context.Background(), "bad", ServerConfig{Transport: TransportWS})
	if err == nil || !strings.Contains(err.Error(), "ws config is nil") {
		t.Errorf("expected nil-config error, got %v", err)
	}
}
