package protocol

import (
	"encoding/json"
	"time"
)

type MsgType string

const (
	MsgBinary     MsgType = "binary"
	MsgLog        MsgType = "log"
	MsgHello      MsgType = "hello"
	MsgDisconnect MsgType = "disconnect"
	MsgStop       MsgType = "stop"
	MsgKill       MsgType = "kill"
	MsgRestart    MsgType = "restart"
	MsgExec       MsgType = "exec"
	MsgPing       MsgType = "ping"
	MsgPong       MsgType = "pong"
)

type Message struct {
	Type      MsgType         `json:"type"`
	Timestamp int64           `json:"ts"`
	Payload   json.RawMessage `json:"p,omitempty"`
}

type BinaryPayload struct {
	Filename string `json:"filename"`
	Content  []byte `json:"content"`
	Args     string `json:"args,omitempty"`
}

type LogPayload struct {
	Source string `json:"src"`
	Line   string `json:"line"`
}

type HelloPayload struct {
	ID string `json:"id"`
}

type ExecPayload struct {
	Cmd string `json:"cmd"`
}

func NewMsg(t MsgType, payload interface{}) ([]byte, error) {
	var raw json.RawMessage
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		raw = data
	}
	msg := &Message{
		Type:      t,
		Timestamp: time.Now().UnixMilli(),
		Payload:   raw,
	}
	return json.Marshal(msg)
}

func Parse[T any](msg *Message) (*T, error) {
	var v T
	if err := json.Unmarshal(msg.Payload, &v); err != nil {
		return nil, err
	}
	return &v, nil
}
