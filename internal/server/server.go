package server

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/xxnuo/ota/internal/protocol"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type Server struct {
	port     int
	conn     *websocket.Conn
	mu       sync.Mutex
	listener net.Listener
	httpSrv  *http.Server
	done     chan struct{}
}

func New(port int) *Server {
	return &Server{
		port: port,
		done: make(chan struct{}),
	}
}

func (s *Server) StartAndGetPort() (int, error) {
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", s.handleWS)
	mux.HandleFunc("/send", s.handleSend)
	mux.HandleFunc("/disconnect", s.handleDisconnect)

	addr := fmt.Sprintf(":%d", s.port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return 0, fmt.Errorf("listen failed: %w", err)
	}
	s.listener = ln

	actualPort := ln.Addr().(*net.TCPAddr).Port
	s.port = actualPort

	s.httpSrv = &http.Server{Handler: mux}
	go func() {
		<-s.done
		s.httpSrv.Close()
	}()

	log.Printf("[server] listening on :%d", actualPort)
	return actualPort, nil
}

func (s *Server) Serve() error {
	err := s.httpSrv.Serve(s.listener)
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

func (s *Server) Port() int {
	return s.port
}

func (s *Server) Stop() {
	s.mu.Lock()
	if s.conn != nil {
		msg, _ := protocol.NewMsg(protocol.MsgDisconnect, nil)
		s.conn.WriteMessage(websocket.TextMessage, msg)
		time.Sleep(200 * time.Millisecond)
		s.conn.Close()
		s.conn = nil
	}
	s.mu.Unlock()
	close(s.done)
}

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[server] ws upgrade failed: %v", err)
		return
	}

	s.mu.Lock()
	if s.conn != nil {
		s.conn.Close()
	}
	s.conn = conn
	s.mu.Unlock()

	log.Printf("[server] client connected: %s", conn.RemoteAddr())

	defer func() {
		s.mu.Lock()
		if s.conn == conn {
			s.conn = nil
		}
		s.mu.Unlock()
		conn.Close()
		log.Printf("[server] client disconnected: %s", conn.RemoteAddr())
	}()

	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				log.Printf("[server] read error: %v", err)
			}
			return
		}

		var msg protocol.Message
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}

		switch msg.Type {
		case protocol.MsgLog:
			payload, err := protocol.Parse[protocol.LogPayload](&msg)
			if err != nil {
				continue
			}
			fmt.Printf("[%s] %s\n", payload.Source, payload.Line)

		case protocol.MsgPong:
		}
	}
}

func (s *Server) handleSend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}

	s.mu.Lock()
	conn := s.conn
	s.mu.Unlock()

	if conn == nil {
		http.Error(w, "no client connected", http.StatusServiceUnavailable)
		return
	}

	filename := r.URL.Query().Get("filename")
	args := r.URL.Query().Get("args")
	if filename == "" {
		filename = "app"
	}

	content, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read body failed", http.StatusBadRequest)
		return
	}

	payload := &protocol.BinaryPayload{
		Filename: filepath.Base(filename),
		Content:  content,
		Args:     args,
	}

	data, err := protocol.NewMsg(protocol.MsgBinary, payload)
	if err != nil {
		http.Error(w, "marshal failed", http.StatusInternalServerError)
		return
	}

	s.mu.Lock()
	err = conn.WriteMessage(websocket.TextMessage, data)
	s.mu.Unlock()

	if err != nil {
		http.Error(w, "send failed", http.StatusInternalServerError)
		return
	}

	log.Printf("[server] sent %s (%d bytes) to client", filename, len(content))
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "sent %s (%d bytes)\n", filename, len(content))
}

func (s *Server) handleDisconnect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}

	s.mu.Lock()
	conn := s.conn
	s.mu.Unlock()

	if conn == nil {
		http.Error(w, "no client connected", http.StatusServiceUnavailable)
		return
	}

	data, _ := protocol.NewMsg(protocol.MsgDisconnect, nil)
	s.mu.Lock()
	conn.WriteMessage(websocket.TextMessage, data)
	time.Sleep(200 * time.Millisecond)
	conn.Close()
	s.conn = nil
	s.mu.Unlock()

	log.Printf("[server] client disconnected by request")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintln(w, "disconnected")
}

func (s *Server) SendFile(filePath string, args string) error {
	s.mu.Lock()
	conn := s.conn
	s.mu.Unlock()

	if conn == nil {
		return fmt.Errorf("no client connected")
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}

	payload := &protocol.BinaryPayload{
		Filename: filepath.Base(filePath),
		Content:  content,
		Args:     args,
	}

	data, err := protocol.NewMsg(protocol.MsgBinary, payload)
	if err != nil {
		return err
	}

	s.mu.Lock()
	err = conn.WriteMessage(websocket.TextMessage, data)
	s.mu.Unlock()

	if err != nil {
		return fmt.Errorf("send failed: %w", err)
	}

	log.Printf("[server] sent %s (%d bytes)", filepath.Base(filePath), len(content))
	return nil
}
