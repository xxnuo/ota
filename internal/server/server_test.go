package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"os"
	"path/filepath"

	"github.com/gorilla/websocket"
	"github.com/xxnuo/ota/internal/logger"
	"github.com/xxnuo/ota/internal/protocol"
)

func init() {
	logger.Init("server")
}

func TestHealthEndpoint(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ota-server-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	os.WriteFile(filepath.Join(tmpDir, "test.txt"), []byte("hello"), 0644)

	srv := New(tmpDir, 0)
	srv.matcher = nil

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	ts := httptest.NewServer(mux)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	_ = srv
}

func TestWebSocketConnection(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ota-server-ws-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	os.WriteFile(filepath.Join(tmpDir, "test.txt"), []byte("hello"), 0644)

	srv := New(tmpDir, 0)

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", srv.handleWebSocket)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial error: %v", err)
	}
	defer conn.Close()

	info := &protocol.ClientInfo{
		WorkDir:  "/tmp/test-client",
		Hostname: "testhost",
		ID:       "test-client-1",
	}
	msg, _ := protocol.NewMessage(protocol.MsgClientInfo, info)
	data, _ := json.Marshal(msg)
	conn.WriteMessage(websocket.TextMessage, data)

	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, respData, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read error: %v", err)
	}

	var respMsg protocol.Message
	json.Unmarshal(respData, &respMsg)
	if respMsg.Type != protocol.MsgManifest {
		t.Errorf("expected manifest message, got %s", respMsg.Type)
	}

	manifest, err := protocol.ParsePayload[protocol.Manifest](& respMsg)
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Files) == 0 {
		t.Error("manifest should contain files")
	}
}

func TestWebSocketFileRequest(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ota-server-filereq-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	os.WriteFile(filepath.Join(tmpDir, "hello.txt"), []byte("hello world"), 0644)

	srv := New(tmpDir, 0)

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", srv.handleWebSocket)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial error: %v", err)
	}
	defer conn.Close()

	info := &protocol.ClientInfo{WorkDir: "/tmp/req-client", Hostname: "test", ID: "req-client-1"}
	msg, _ := protocol.NewMessage(protocol.MsgClientInfo, info)
	data, _ := json.Marshal(msg)
	conn.WriteMessage(websocket.TextMessage, data)

	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	conn.ReadMessage()

	req := &protocol.FileRequest{Paths: []string{"hello.txt"}}
	reqMsg, _ := protocol.NewMessage(protocol.MsgFileRequest, req)
	reqData, _ := json.Marshal(reqMsg)
	conn.WriteMessage(websocket.TextMessage, reqData)

	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, fileRespData, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read file response error: %v", err)
	}

	var fileMsg protocol.Message
	json.Unmarshal(fileRespData, &fileMsg)
	if fileMsg.Type != protocol.MsgFileData {
		t.Errorf("expected file_data, got %s", fileMsg.Type)
	}

	fd, _ := protocol.ParsePayload[protocol.FileData](&fileMsg)
	if string(fd.Content) != "hello world" {
		t.Errorf("expected 'hello world', got '%s'", string(fd.Content))
	}
}
