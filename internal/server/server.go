package server

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/xxnuo/ota/internal/protocol"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type clientConn struct {
	id   int
	name string
	conn *websocket.Conn
	addr string
	mu   sync.Mutex
}

func (c *clientConn) label() string {
	if c.name != "" {
		return fmt.Sprintf("#%d(%s)", c.id, c.name)
	}
	return fmt.Sprintf("#%d", c.id)
}

type Server struct {
	port     int
	clients  map[int]*clientConn
	nextID   int
	mu       sync.RWMutex
	listener net.Listener
	httpSrv  *http.Server
	done     chan struct{}
}

func New(port int) *Server {
	return &Server{
		port:    port,
		clients: make(map[int]*clientConn),
		nextID:  1,
		done:    make(chan struct{}),
	}
}

func (s *Server) StartAndGetPort() (int, error) {
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", s.handleWS)
	mux.HandleFunc("/send", s.handleSend)
	mux.HandleFunc("/disconnect", s.handleDisconnect)
	mux.HandleFunc("/ps", s.handlePs)

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

func (s *Server) Stop() {
	s.mu.Lock()
	for _, c := range s.clients {
		msg, _ := protocol.NewMsg(protocol.MsgDisconnect, nil)
		c.mu.Lock()
		c.conn.WriteMessage(websocket.TextMessage, msg)
		c.mu.Unlock()
		time.Sleep(100 * time.Millisecond)
		c.conn.Close()
	}
	s.clients = make(map[int]*clientConn)
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
	id := s.nextID
	s.nextID++
	cc := &clientConn{id: id, conn: conn, addr: conn.RemoteAddr().String()}
	s.clients[id] = cc
	s.mu.Unlock()

	log.Printf("[server] client #%d connected: %s", id, conn.RemoteAddr())

	defer func() {
		s.mu.Lock()
		delete(s.clients, id)
		s.mu.Unlock()
		conn.Close()
		log.Printf("[server] client #%d disconnected: %s", id, cc.addr)
	}()

	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				log.Printf("[server] client #%d read error: %v", id, err)
			}
			return
		}

		var msg protocol.Message
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}

		switch msg.Type {
		case protocol.MsgHello:
			hello, err := protocol.Parse[protocol.HelloPayload](&msg)
			if err == nil && hello.ID != "" {
				cc.mu.Lock()
				cc.name = hello.ID
				cc.mu.Unlock()
				log.Printf("[server] client %s identified", cc.label())
			}

		case protocol.MsgLog:
			payload, err := protocol.Parse[protocol.LogPayload](&msg)
			if err != nil {
				continue
			}
			if s.clientCount() > 1 {
				fmt.Printf("[%s %s] %s\n", cc.label(), payload.Source, payload.Line)
			} else {
				fmt.Printf("[%s] %s\n", payload.Source, payload.Line)
			}

		case protocol.MsgPong:
		}
	}
}

func (s *Server) clientCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.clients)
}

func (s *Server) getClient(idStr string) (*clientConn, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.clients) == 0 {
		return nil, fmt.Errorf("no client connected")
	}

	if idStr == "" || idStr == "0" {
		if len(s.clients) == 1 {
			for _, c := range s.clients {
				return c, nil
			}
		}
		return nil, fmt.Errorf("multiple clients connected, specify --id or --name (use 'ota ps' to list)")
	}

	if id, err := strconv.Atoi(idStr); err == nil {
		if c, ok := s.clients[id]; ok {
			return c, nil
		}
		return nil, fmt.Errorf("client #%d not found", id)
	}

	for _, c := range s.clients {
		if c.name == idStr {
			return c, nil
		}
	}
	return nil, fmt.Errorf("client '%s' not found", idStr)
}

func (s *Server) handleSend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}

	cc, err := s.getClient(r.URL.Query().Get("id"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
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

	cc.mu.Lock()
	err = cc.conn.WriteMessage(websocket.TextMessage, data)
	cc.mu.Unlock()

	if err != nil {
		http.Error(w, "send failed", http.StatusInternalServerError)
		return
	}

	log.Printf("[server] sent %s (%d bytes) to client #%d", filename, len(content), cc.id)
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "sent %s (%d bytes) to #%d\n", filename, len(content), cc.id)
}

func (s *Server) handleDisconnect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}

	cc, err := s.getClient(r.URL.Query().Get("id"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	data, _ := protocol.NewMsg(protocol.MsgDisconnect, nil)
	cc.mu.Lock()
	cc.conn.WriteMessage(websocket.TextMessage, data)
	cc.mu.Unlock()
	time.Sleep(200 * time.Millisecond)
	cc.conn.Close()

	s.mu.Lock()
	delete(s.clients, cc.id)
	s.mu.Unlock()

	log.Printf("[server] client %s disconnected by request", cc.label())
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "client %s disconnected\n", cc.label())
}

func (s *Server) handlePs(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.clients) == 0 {
		fmt.Fprintln(w, "no clients connected")
		return
	}

	fmt.Fprintf(w, "clients: %d\n", len(s.clients))
	for _, c := range s.clients {
		fmt.Fprintf(w, "  %-12s %s\n", c.label(), c.addr)
	}
}
