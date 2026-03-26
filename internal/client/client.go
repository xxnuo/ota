package client

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/xxnuo/ota/internal/clog"
	"github.com/xxnuo/ota/internal/process"
	"github.com/xxnuo/ota/internal/protocol"
)

type Client struct {
	serverURL string
	workDir   string
	id        string
	conn      *websocket.Conn
	proc      *process.Manager
	lastBin   *protocol.BinaryPayload
	done      chan struct{}
}

func New(serverURL, workDir, id string) *Client {
	absDir, _ := filepath.Abs(workDir)
	return &Client{
		serverURL: serverURL,
		workDir:   absDir,
		id:        id,
		done:      make(chan struct{}),
	}
}

func (c *Client) Start() error {
	os.MkdirAll(c.workDir, 0755)
	clog.Client("workDir: %s", c.workDir)

	for {
		select {
		case <-c.done:
			return nil
		default:
		}

		err := c.connectAndRun()
		if err == errDisconnected {
			clog.Client("disconnected by server, exiting")
			return nil
		}

		select {
		case <-c.done:
			return nil
		default:
			clog.Client("connection lost, reconnecting in 3s...")
			time.Sleep(3 * time.Second)
		}
	}
}

var errDisconnected = fmt.Errorf("disconnected")

func (c *Client) connectAndRun() error {
	u, err := url.Parse(c.serverURL)
	if err != nil {
		return err
	}

	wsScheme := "ws"
	if u.Scheme == "wss" || u.Scheme == "https" {
		wsScheme = "wss"
	}
	wsURL := fmt.Sprintf("%s://%s/ws", wsScheme, u.Host)

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		clog.Error("connect failed: %v", err)
		return err
	}
	c.conn = conn
	clog.Client("connected to %s", c.serverURL)

	if c.id != "" {
		hello, _ := protocol.NewMsg(protocol.MsgHello, &protocol.HelloPayload{ID: c.id})
		conn.WriteMessage(websocket.TextMessage, hello)
	}

	c.sendLog("client", "connected")

	defer func() {
		conn.Close()
		c.conn = nil
	}()

	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return err
		}

		var msg protocol.Message
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}

		switch msg.Type {
		case protocol.MsgBinary:
			payload, err := protocol.Parse[protocol.BinaryPayload](&msg)
			if err != nil {
				c.sendLog("client", fmt.Sprintf("parse binary msg error: %v", err))
				continue
			}
			c.handleBinary(payload)

		case protocol.MsgStop:
			c.sendLog("client", "stop requested")
			c.stopApp()

		case protocol.MsgKill:
			c.sendLog("client", "kill requested")
			c.killApp()

		case protocol.MsgRestart:
			c.sendLog("client", "restart requested")
			c.restartApp()

		case protocol.MsgDisconnect:
			c.stopApp()
			return errDisconnected

		case protocol.MsgPing:
			pong, _ := protocol.NewMsg(protocol.MsgPong, nil)
			conn.WriteMessage(websocket.TextMessage, pong)
		}
	}
}

func (c *Client) handleBinary(payload *protocol.BinaryPayload) {
	c.stopApp()

	binPath := filepath.Join(c.workDir, payload.Filename)
	if err := os.WriteFile(binPath, payload.Content, 0755); err != nil {
		c.sendLog("client", fmt.Sprintf("write binary error: %v", err))
		return
	}

	c.lastBin = &protocol.BinaryPayload{
		Filename: payload.Filename,
		Args:     payload.Args,
	}

	c.sendLog("client", fmt.Sprintf("received %s (%d bytes)", payload.Filename, len(payload.Content)))

	c.startProc()
}

func (c *Client) startProc() {
	if c.lastBin == nil {
		c.sendLog("client", "no binary to start")
		return
	}

	binPath := filepath.Join(c.workDir, c.lastBin.Filename)

	var args []string
	if c.lastBin.Args != "" {
		args = strings.Fields(c.lastBin.Args)
	}

	c.proc = process.New(binPath, args, c.workDir, c.sendLog)
	if err := c.proc.Start(); err != nil {
		c.sendLog("client", fmt.Sprintf("start app error: %v", err))
		return
	}
	c.sendLog("client", fmt.Sprintf("started %s", c.lastBin.Filename))
}

func (c *Client) stopApp() {
	if c.proc != nil && c.proc.IsRunning() {
		c.sendLog("client", "stopping app...")
		c.proc.Stop()
		time.Sleep(500 * time.Millisecond)
		if c.proc.IsRunning() {
			c.proc.Kill()
		}
	}
}

func (c *Client) killApp() {
	if c.proc != nil && c.proc.IsRunning() {
		c.proc.Kill()
		c.sendLog("client", "app killed")
	}
}

func (c *Client) restartApp() {
	c.stopApp()
	c.startProc()
}

func (c *Client) sendLog(source, line string) {
	clog.App(source, line)
	if c.conn == nil {
		return
	}
	payload := &protocol.LogPayload{Source: source, Line: line}
	data, err := protocol.NewMsg(protocol.MsgLog, payload)
	if err != nil {
		return
	}
	c.conn.WriteMessage(websocket.TextMessage, data)
}

func (c *Client) Stop() {
	c.stopApp()
	if c.conn != nil {
		c.conn.Close()
	}
	select {
	case <-c.done:
	default:
		close(c.done)
	}
}
