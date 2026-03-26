package server

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	gosync "sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/gorilla/websocket"
	"github.com/xxnuo/ota/internal/config"
	"github.com/xxnuo/ota/internal/ignore"
	"github.com/xxnuo/ota/internal/logger"
	"github.com/xxnuo/ota/internal/protocol"
	filesync "github.com/xxnuo/ota/internal/sync"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type clientConn struct {
	conn     *websocket.Conn
	info     protocol.ClientInfo
	mu       gosync.Mutex
	sendCh   chan *protocol.Message
	execCh   chan *protocol.ExecResponse
	closeCh  chan struct{}
}

type Server struct {
	workDir  string
	port     int
	state    *config.ServerState
	clients  map[string]*clientConn
	mu       gosync.RWMutex
	watcher  *fsnotify.Watcher
	matcher  *ignore.Matcher
	stopCh   chan struct{}
	listener net.Listener
}

func New(workDir string, port int) *Server {
	return &Server{
		workDir: workDir,
		port:    port,
		state:   config.NewServerState(),
		clients: make(map[string]*clientConn),
		stopCh:  make(chan struct{}),
	}
}

func (s *Server) Start() error {
	absDir, err := filepath.Abs(s.workDir)
	if err != nil {
		return err
	}
	s.workDir = absDir

	s.matcher = ignore.New(s.workDir)

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("failed to create watcher: %w", err)
	}
	s.watcher = watcher

	if err := s.addWatchDirs(s.workDir); err != nil {
		return fmt.Errorf("failed to watch directory: %w", err)
	}

	go s.watchFiles()

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", s.handleWebSocket)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	addr := fmt.Sprintf("0.0.0.0:%d", s.port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", addr, err)
	}
	s.listener = listener

	actualPort := listener.Addr().(*net.TCPAddr).Port
	s.port = actualPort

	cfg := &config.ServerConfig{
		PID:     os.Getpid(),
		Address: fmt.Sprintf("0.0.0.0:%d", actualPort),
		WorkDir: s.workDir,
		Port:    actualPort,
	}
	config.SaveServerConfig(cfg)
	config.SavePid(config.ServerPidFile(), os.Getpid())

	logger.Log.Info().Str("address", cfg.Address).Str("workDir", s.workDir).Msg("server started")

	server := &http.Server{Handler: mux}
	go func() {
		<-s.stopCh
		server.Close()
	}()

	return server.Serve(listener)
}

func (s *Server) addWatchDirs(root string) error {
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() {
			return nil
		}
		relPath, _ := filepath.Rel(s.workDir, path)
		if relPath != "." && s.matcher.Match(relPath, true) {
			return filepath.SkipDir
		}
		return s.watcher.Add(path)
	})
}

func (s *Server) watchFiles() {
	debounce := make(map[string]time.Time)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	pending := make(map[string]fsnotify.Event)
	var mu gosync.Mutex

	go func() {
		for {
			select {
			case <-s.stopCh:
				return
			case <-ticker.C:
				mu.Lock()
				now := time.Now()
				for path, t := range debounce {
					if now.Sub(t) > 200*time.Millisecond {
						if evt, ok := pending[path]; ok {
							s.handleFileEvent(evt)
							delete(pending, path)
						}
						delete(debounce, path)
					}
				}
				mu.Unlock()
			}
		}
	}()

	for {
		select {
		case <-s.stopCh:
			return
		case event, ok := <-s.watcher.Events:
			if !ok {
				return
			}
			relPath, err := filepath.Rel(s.workDir, event.Name)
			if err != nil {
				continue
			}
			info, _ := os.Stat(event.Name)
			isDir := info != nil && info.IsDir()
			if s.matcher.Match(relPath, isDir) {
				continue
			}
			mu.Lock()
			debounce[event.Name] = time.Now()
			pending[event.Name] = event
			mu.Unlock()
		case err, ok := <-s.watcher.Errors:
			if !ok {
				return
			}
			logger.Log.Error().Err(err).Msg("watcher error")
		}
	}
}

func (s *Server) handleFileEvent(event fsnotify.Event) {
	relPath, err := filepath.Rel(s.workDir, event.Name)
	if err != nil {
		return
	}

	if event.Op&fsnotify.Remove != 0 || event.Op&fsnotify.Rename != 0 {
		msg, _ := protocol.NewMessage(protocol.MsgFileDelete, &protocol.FileDelete{Path: relPath})
		s.broadcast(msg)
		logger.Log.Debug().Str("path", relPath).Msg("file deleted")
		return
	}

	if event.Op&(fsnotify.Create|fsnotify.Write) != 0 {
		info, err := os.Stat(event.Name)
		if err != nil {
			return
		}

		if info.IsDir() {
			s.watcher.Add(event.Name)
			return
		}

		fd, err := filesync.ReadFile(s.workDir, relPath)
		if err != nil {
			logger.Log.Error().Err(err).Str("path", relPath).Msg("failed to read file for sync")
			return
		}

		change := &protocol.FileChange{
			Path:    relPath,
			Content: fd.Content,
			Mode:    fd.Mode,
			IsDir:   false,
		}
		msg, _ := protocol.NewMessage(protocol.MsgFileChange, change)
		s.broadcast(msg)
		logger.Log.Debug().Str("path", relPath).Int64("size", info.Size()).Msg("file changed")
	}
}

func (s *Server) broadcast(msg *protocol.Message) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, c := range s.clients {
		select {
		case c.sendCh <- msg:
		default:
			logger.Log.Warn().Str("client", c.info.ID).Msg("send channel full, dropping message")
		}
	}
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.Log.Error().Err(err).Msg("websocket upgrade failed")
		return
	}

	client := &clientConn{
		conn:    conn,
		sendCh:  make(chan *protocol.Message, 256),
		execCh:  make(chan *protocol.ExecResponse, 1),
		closeCh: make(chan struct{}),
	}

	go s.writePump(client)
	s.readPump(client)
}

func (s *Server) writePump(c *clientConn) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.closeCh:
			return
		case msg := <-c.sendCh:
			c.mu.Lock()
			data, _ := json.Marshal(msg)
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			err := c.conn.WriteMessage(websocket.TextMessage, data)
			c.mu.Unlock()
			if err != nil {
				logger.Log.Error().Err(err).Msg("write error")
				return
			}
		case <-ticker.C:
			msg, _ := protocol.NewMessage(protocol.MsgPing, nil)
			c.mu.Lock()
			data, _ := json.Marshal(msg)
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			err := c.conn.WriteMessage(websocket.TextMessage, data)
			c.mu.Unlock()
			if err != nil {
				return
			}
		}
	}
}

func (s *Server) readPump(c *clientConn) {
	defer func() {
		close(c.closeCh)
		c.conn.Close()
		if c.info.ID != "" {
			s.mu.Lock()
			delete(s.clients, c.info.ID)
			s.mu.Unlock()
			s.state.RemoveClient(c.info.ID)
			s.state.Save()
			logger.Log.Info().Str("client", c.info.ID).Msg("client disconnected")
		}
	}()

	c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, data, err := c.conn.ReadMessage()
		if err != nil {
			if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				logger.Log.Error().Err(err).Msg("read error")
			}
			return
		}

		c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))

		var msg protocol.Message
		if err := json.Unmarshal(data, &msg); err != nil {
			logger.Log.Error().Err(err).Msg("invalid message")
			continue
		}

		s.handleMessage(c, &msg)
	}
}

func (s *Server) handleMessage(c *clientConn, msg *protocol.Message) {
	switch msg.Type {
	case protocol.MsgClientInfo:
		info, err := protocol.ParsePayload[protocol.ClientInfo](msg)
		if err != nil {
			logger.Log.Error().Err(err).Msg("invalid client info")
			return
		}
		c.info = *info
		s.mu.Lock()
		s.clients[info.ID] = c
		s.mu.Unlock()
		s.state.AddClient(config.ClientInfo{
			ID:       info.ID,
			Hostname: info.Hostname,
			WorkDir:  info.WorkDir,
			Addr:     c.conn.RemoteAddr().String(),
		})
		s.state.Save()
		logger.Log.Info().Str("id", info.ID).Str("hostname", info.Hostname).Str("workDir", info.WorkDir).Msg("client connected")

		manifest, err := filesync.BuildManifest(s.workDir)
		if err != nil {
			logger.Log.Error().Err(err).Msg("failed to build manifest")
			return
		}
		reply, _ := protocol.NewMessage(protocol.MsgManifest, manifest)
		c.sendCh <- reply

	case protocol.MsgFileRequest:
		req, err := protocol.ParsePayload[protocol.FileRequest](msg)
		if err != nil {
			return
		}
		for _, path := range req.Paths {
			fd, err := filesync.ReadFile(s.workDir, path)
			if err != nil {
				logger.Log.Error().Err(err).Str("path", path).Msg("failed to read requested file")
				continue
			}
			reply, _ := protocol.NewMessage(protocol.MsgFileData, fd)
			c.sendCh <- reply
		}
		done, _ := protocol.NewMessage(protocol.MsgSyncDone, nil)
		c.sendCh <- done

	case protocol.MsgPong:

	case protocol.MsgExecResponse:
		resp, err := protocol.ParsePayload[protocol.ExecResponse](msg)
		if err != nil {
			return
		}
		select {
		case c.execCh <- resp:
		default:
		}

	default:
		logger.Log.Warn().Str("type", string(msg.Type)).Msg("unknown message type")
	}
}

func (s *Server) Stop() {
	close(s.stopCh)
	if s.watcher != nil {
		s.watcher.Close()
	}
	if s.listener != nil {
		s.listener.Close()
	}
	config.RemovePid(config.ServerPidFile())
	logger.Log.Info().Msg("server stopped")
}

func (s *Server) SendExec(workDir, command string) (*protocol.ExecResponse, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, c := range s.clients {
		if workDir == "" || c.info.WorkDir == workDir || strings.HasSuffix(c.info.WorkDir, workDir) {
			req := &protocol.ExecRequest{
				Command: command,
				WorkDir: c.info.WorkDir,
			}
			msg, _ := protocol.NewMessage(protocol.MsgExecRequest, req)
			c.sendCh <- msg

			select {
			case resp := <-c.execCh:
				return resp, nil
			case <-time.After(30 * time.Second):
				return nil, fmt.Errorf("exec timeout")
			}
		}
	}
	return nil, fmt.Errorf("no client found for workDir: %s", workDir)
}

func (s *Server) GetClients() []config.ClientInfo {
	return s.state.GetClients()
}
