package client

import (
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/xxnuo/ota/internal/process"
	"github.com/xxnuo/ota/internal/protocol"
)

type Client struct {
	serverURL string
	workDir   string
	id        string
	conn      *websocket.Conn
	proc      *process.Manager
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
	log.Printf("[client] workDir: %s", c.workDir)

	for {
		select {
		case <-c.done:
			return nil
		default:
		}

		err := c.connectAndRun()
		if err == errDisconnected {
			log.Printf("[client] disconnected by server, exiting")
			return nil
		}

		select {
		case <-c.done:
			return nil
		default:
			log.Printf("[client] connection lost, reconnecting in 3s...")
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
		log.Printf("[client] connect failed: %v", err)
		return err
	}
	c.conn = conn
	log.Printf("[client] connected to %s", c.serverURL)

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

	c.sendLog("client", fmt.Sprintf("received %s (%d bytes)", payload.Filename, len(payload.Content)))

	var args []string
	if payload.Args != "" {
		args = strings.Fields(payload.Args)
	}

	c.proc = process.New(binPath, args, c.workDir, c.sendLog)
	if err := c.proc.Start(); err != nil {
		c.sendLog("client", fmt.Sprintf("start app error: %v", err))
		return
	}
	c.sendLog("client", fmt.Sprintf("started %s", payload.Filename))
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

func (c *Client) sendLog(source, line string) {
	log.Printf("[%s] %s", source, line)
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
