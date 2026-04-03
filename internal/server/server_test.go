package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/xxnuo/ota/internal/protocol"
)

func startTestServer(t *testing.T) (*Server, int) {
	t.Helper()

	srv := New(0)
	port, err := srv.StartAndGetPort()
	if err != nil {
		t.Fatalf("start server: %v", err)
	}

	go func() {
		_ = srv.Serve()
	}()

	return srv, port
}

func dialTestClient(t *testing.T, port int) *websocket.Conn {
	t.Helper()

	conn, _, err := websocket.DefaultDialer.Dial(fmt.Sprintf("ws://127.0.0.1:%d/ws", port), nil)
	if err != nil {
		t.Fatalf("dial ws: %v", err)
	}

	return conn
}

func TestStopDoesNotSendDisconnect(t *testing.T) {
	srv, port := startTestServer(t)
	conn := dialTestClient(t, port)
	defer conn.Close()

	srv.Stop()

	if err := conn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}

	if _, _, err := conn.ReadMessage(); err == nil {
		t.Fatal("expected connection close without disconnect message")
	}
}

func TestDisconnectEndpointSendsDisconnect(t *testing.T) {
	srv, port := startTestServer(t)
	defer srv.Stop()

	conn := dialTestClient(t, port)
	defer conn.Close()

	resp, err := http.Post(fmt.Sprintf("http://127.0.0.1:%d/disconnect", port), "text/plain", nil)
	if err != nil {
		t.Fatalf("post disconnect: %v", err)
	}
	resp.Body.Close()

	if err := conn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}

	_, data, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read disconnect: %v", err)
	}

	var msg protocol.Message
	if err := json.Unmarshal(data, &msg); err != nil {
		t.Fatalf("unmarshal message: %v", err)
	}

	if msg.Type != protocol.MsgDisconnect {
		t.Fatalf("expected %q, got %q", protocol.MsgDisconnect, msg.Type)
	}
}
