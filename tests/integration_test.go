package integration

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/xxnuo/ota/internal/logger"
	"github.com/xxnuo/ota/internal/protocol"
	"github.com/xxnuo/ota/internal/server"
	filesync "github.com/xxnuo/ota/internal/sync"
)

func init() {
	logger.Init("server")
}

func TestFullSyncFlow(t *testing.T) {
	serverDir, _ := os.MkdirTemp("", "ota-integ-server-*")
	clientDir, _ := os.MkdirTemp("", "ota-integ-client-*")
	defer os.RemoveAll(serverDir)
	defer os.RemoveAll(clientDir)

	os.WriteFile(filepath.Join(serverDir, "main.go"), []byte("package main\nfunc main() {}"), 0644)
	os.MkdirAll(filepath.Join(serverDir, "pkg"), 0755)
	os.WriteFile(filepath.Join(serverDir, "pkg", "util.go"), []byte("package pkg"), 0644)
	os.WriteFile(filepath.Join(serverDir, "README.md"), []byte("# Test"), 0644)

	srv := server.New(serverDir, 0)
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", srv.HandleWebSocketForTest)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	info := &protocol.ClientInfo{WorkDir: clientDir, Hostname: "test", ID: "integ-1"}
	msg, _ := protocol.NewMessage(protocol.MsgClientInfo, info)
	data, _ := json.Marshal(msg)
	conn.WriteMessage(websocket.TextMessage, data)

	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, respData, _ := conn.ReadMessage()
	var manifestMsg protocol.Message
	json.Unmarshal(respData, &manifestMsg)

	remoteManifest, _ := protocol.ParsePayload[protocol.Manifest](&manifestMsg)

	localManifest := &protocol.Manifest{}
	needFiles, _ := filesync.DiffManifest(localManifest, remoteManifest)

	if len(needFiles) == 0 {
		t.Fatal("should need files")
	}

	req := &protocol.FileRequest{Paths: needFiles}
	reqMsg, _ := protocol.NewMessage(protocol.MsgFileRequest, req)
	reqData, _ := json.Marshal(reqMsg)
	conn.WriteMessage(websocket.TextMessage, reqData)

	received := 0
	for {
		conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		_, fileData, err := conn.ReadMessage()
		if err != nil {
			break
		}
		var fileMsg protocol.Message
		json.Unmarshal(fileData, &fileMsg)

		if fileMsg.Type == protocol.MsgSyncDone {
			break
		}

		if fileMsg.Type == protocol.MsgFileData {
			fd, _ := protocol.ParsePayload[protocol.FileData](&fileMsg)
			filesync.ApplyFile(clientDir, fd)
			received++
		}
	}

	if received == 0 {
		t.Error("should have received files")
	}

	for _, path := range []string{"main.go", "README.md", filepath.Join("pkg", "util.go")} {
		serverContent, _ := os.ReadFile(filepath.Join(serverDir, path))
		clientContent, _ := os.ReadFile(filepath.Join(clientDir, path))
		if string(serverContent) != string(clientContent) {
			t.Errorf("file %s content mismatch", path)
		}
	}
}
