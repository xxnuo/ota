package client

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/xxnuo/ota/internal/config"
	"github.com/xxnuo/ota/internal/logger"
	"github.com/xxnuo/ota/internal/process"
	"github.com/xxnuo/ota/internal/protocol"
	filesync "github.com/xxnuo/ota/internal/sync"
)

type Client struct {
	serverURL string
	workDir   string
	command   string
	clientID  string
	hostname  string
	conn      *websocket.Conn
	proc      *process.Manager
	stopCh    chan struct{}
	syncDone  bool
}

func New(serverURL, workDir, command string) *Client {
	hostname, _ := os.Hostname()
	absDir, _ := filepath.Abs(workDir)

	id := fmt.Sprintf("%s-%s-%d", hostname, filepath.Base(absDir), os.Getpid())

	return &Client{
		serverURL: serverURL,
		workDir:   absDir,
		command:   command,
		clientID:  id,
		hostname:  hostname,
		stopCh:    make(chan struct{}),
	}
}

func (c *Client) Start() error {
	os.MkdirAll(c.workDir, 0755)

	cfg := &config.ClientConfig{
		PID:       os.Getpid(),
		ServerURL: c.serverURL,
		WorkDir:   c.workDir,
		Command:   c.command,
	}
	config.SaveClientConfig(cfg)
	config.SavePid(config.ClientPidFile(), os.Getpid())

	logger.Log.Info().Str("server", c.serverURL).Str("workDir", c.workDir).Msg("client starting")

	for {
		select {
		case <-c.stopCh:
			return nil
		default:
		}

		if err := c.connect(); err != nil {
			logger.Log.Error().Err(err).Msg("connection failed, retrying in 3s")
			time.Sleep(3 * time.Second)
			continue
		}

		c.readLoop()

		select {
		case <-c.stopCh:
			return nil
		default:
			logger.Log.Warn().Msg("disconnected, reconnecting in 3s")
			time.Sleep(3 * time.Second)
		}
	}
}

func (c *Client) connect() error {
	u, err := url.Parse(c.serverURL)
	if err != nil {
		return err
	}

	wsURL := fmt.Sprintf("ws://%s/ws", u.Host)
	if u.Scheme == "wss" || u.Scheme == "https" {
		wsURL = fmt.Sprintf("wss://%s/ws", u.Host)
	}

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		return err
	}
	c.conn = conn

	info := &protocol.ClientInfo{
		WorkDir:  c.workDir,
		Hostname: c.hostname,
		ID:       c.clientID,
	}
	msg, _ := protocol.NewMessage(protocol.MsgClientInfo, info)
	data, _ := json.Marshal(msg)
	conn.WriteMessage(websocket.TextMessage, data)

	logger.Log.Info().Str("server", c.serverURL).Msg("connected to server")
	return nil
}

func (c *Client) readLoop() {
	pingTicker := time.NewTicker(25 * time.Second)
	defer pingTicker.Stop()

	go func() {
		for {
			select {
			case <-c.stopCh:
				return
			case <-pingTicker.C:
				if c.conn != nil {
					msg, _ := protocol.NewMessage(protocol.MsgPong, nil)
					data, _ := json.Marshal(msg)
					c.conn.WriteMessage(websocket.TextMessage, data)
				}
			}
		}
	}()

	for {
		_, data, err := c.conn.ReadMessage()
		if err != nil {
			if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				logger.Log.Error().Err(err).Msg("read error")
			}
			return
		}

		var msg protocol.Message
		if err := json.Unmarshal(data, &msg); err != nil {
			logger.Log.Error().Err(err).Msg("invalid message")
			continue
		}

		c.handleMessage(&msg)
	}
}

func (c *Client) handleMessage(msg *protocol.Message) {
	switch msg.Type {
	case protocol.MsgManifest:
		manifest, err := protocol.ParsePayload[protocol.Manifest](msg)
		if err != nil {
			logger.Log.Error().Err(err).Msg("invalid manifest")
			return
		}
		c.handleManifest(manifest)

	case protocol.MsgFileData:
		fd, err := protocol.ParsePayload[protocol.FileData](msg)
		if err != nil {
			logger.Log.Error().Err(err).Msg("invalid file data")
			return
		}
		if err := filesync.ApplyFile(c.workDir, fd); err != nil {
			logger.Log.Error().Err(err).Str("path", fd.Path).Msg("failed to apply file")
		} else {
			logger.Log.Debug().Str("path", fd.Path).Msg("file synced")
		}

	case protocol.MsgFileChange:
		change, err := protocol.ParsePayload[protocol.FileChange](msg)
		if err != nil {
			return
		}
		fd := &protocol.FileData{
			Path:    change.Path,
			Content: change.Content,
			Mode:    change.Mode,
			IsDir:   change.IsDir,
		}
		if err := filesync.ApplyFile(c.workDir, fd); err != nil {
			logger.Log.Error().Err(err).Str("path", change.Path).Msg("failed to apply change")
		} else {
			logger.Log.Info().Str("path", change.Path).Msg("file updated")
		}

	case protocol.MsgFileDelete:
		del, err := protocol.ParsePayload[protocol.FileDelete](msg)
		if err != nil {
			return
		}
		if err := filesync.DeleteFile(c.workDir, del.Path); err != nil {
			logger.Log.Error().Err(err).Str("path", del.Path).Msg("failed to delete file")
		} else {
			logger.Log.Info().Str("path", del.Path).Msg("file deleted")
		}

	case protocol.MsgSyncDone:
		logger.Log.Info().Msg("initial sync complete")
		c.syncDone = true
		if c.command != "" && c.proc == nil {
			c.startCommand()
		}

	case protocol.MsgExecRequest:
		req, err := protocol.ParsePayload[protocol.ExecRequest](msg)
		if err != nil {
			return
		}
		c.handleExec(req)

	case protocol.MsgPing:
		reply, _ := protocol.NewMessage(protocol.MsgPong, nil)
		data, _ := json.Marshal(reply)
		c.conn.WriteMessage(websocket.TextMessage, data)

	default:
		logger.Log.Warn().Str("type", string(msg.Type)).Msg("unknown message type")
	}
}

func (c *Client) handleManifest(remote *protocol.Manifest) {
	logger.Log.Info().Int("files", len(remote.Files)).Msg("received server manifest")

	local, err := filesync.BuildManifest(c.workDir)
	if err != nil {
		logger.Log.Error().Err(err).Msg("failed to build local manifest")
		local = &protocol.Manifest{}
	}

	needFiles, deleteFiles := filesync.DiffManifest(local, remote)

	for _, path := range deleteFiles {
		filesync.DeleteFile(c.workDir, path)
		logger.Log.Debug().Str("path", path).Msg("deleted local file")
	}

	if len(needFiles) > 0 {
		logger.Log.Info().Int("count", len(needFiles)).Msg("requesting files")
		req := &protocol.FileRequest{Paths: needFiles}
		msg, _ := protocol.NewMessage(protocol.MsgFileRequest, req)
		data, _ := json.Marshal(msg)
		c.conn.WriteMessage(websocket.TextMessage, data)
	} else {
		logger.Log.Info().Msg("already in sync")
		c.syncDone = true
		if c.command != "" && c.proc == nil {
			c.startCommand()
		}
	}
}

func (c *Client) startCommand() {
	c.proc = process.New(c.command, c.workDir)
	if err := c.proc.Start(); err != nil {
		logger.Log.Error().Err(err).Str("command", c.command).Msg("failed to start command")
	}
}

func (c *Client) handleExec(req *protocol.ExecRequest) {
	logger.Log.Info().Str("command", req.Command).Msg("executing remote command")

	parts := strings.Fields(req.Command)
	if len(parts) == 0 {
		return
	}

	cmd := exec.Command(parts[0], parts[1:]...)
	cmd.Dir = c.workDir

	output, err := cmd.CombinedOutput()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
		}
	}

	resp := &protocol.ExecResponse{
		ExitCode: exitCode,
		Stdout:   string(output),
	}
	msg, _ := protocol.NewMessage(protocol.MsgExecResponse, resp)
	data, _ := json.Marshal(msg)
	c.conn.WriteMessage(websocket.TextMessage, data)
}

func (c *Client) Stop() {
	close(c.stopCh)
	if c.proc != nil {
		c.proc.Stop()
	}
	if c.conn != nil {
		c.conn.Close()
	}
	config.RemovePid(config.ClientPidFile())
	logger.Log.Info().Msg("client stopped")
}

func (c *Client) CmdStart() error {
	if c.proc == nil {
		c.proc = process.New(c.command, c.workDir)
	}
	return c.proc.Start()
}

func (c *Client) CmdStop() error {
	if c.proc == nil {
		return fmt.Errorf("no process")
	}
	return c.proc.Stop()
}

func (c *Client) CmdKill() error {
	if c.proc == nil {
		return fmt.Errorf("no process")
	}
	return c.proc.Kill()
}

func (c *Client) CmdRestart() error {
	if c.proc == nil {
		return fmt.Errorf("no process")
	}
	return c.proc.Restart()
}
