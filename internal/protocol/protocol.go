package protocol

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"time"
)

type MsgType string

const (
	MsgManifest     MsgType = "manifest"
	MsgFileRequest  MsgType = "file_request"
	MsgFileData     MsgType = "file_data"
	MsgFileChange   MsgType = "file_change"
	MsgFileDelete   MsgType = "file_delete"
	MsgExecRequest  MsgType = "exec_request"
	MsgExecResponse MsgType = "exec_response"
	MsgPing         MsgType = "ping"
	MsgPong         MsgType = "pong"
	MsgError        MsgType = "error"
	MsgClientInfo   MsgType = "client_info"
	MsgSyncDone     MsgType = "sync_done"
)

type Message struct {
	Type      MsgType         `json:"type"`
	Timestamp int64           `json:"timestamp"`
	Payload   json.RawMessage `json:"payload,omitempty"`
}

type FileInfo struct {
	Path    string      `json:"path"`
	Hash    string      `json:"hash"`
	Size    int64       `json:"size"`
	Mode    os.FileMode `json:"mode"`
	ModTime time.Time   `json:"mod_time"`
	IsDir   bool        `json:"is_dir"`
}

type Manifest struct {
	Files   []FileInfo `json:"files"`
	WorkDir string     `json:"work_dir"`
}

type FileRequest struct {
	Paths []string `json:"paths"`
}

type FileData struct {
	Path    string      `json:"path"`
	Content []byte      `json:"content"`
	Mode    os.FileMode `json:"mode"`
	IsDir   bool        `json:"is_dir"`
}

type FileChange struct {
	Path    string      `json:"path"`
	Content []byte      `json:"content"`
	Mode    os.FileMode `json:"mode"`
	IsDir   bool        `json:"is_dir"`
}

type FileDelete struct {
	Path string `json:"path"`
}

type ExecRequest struct {
	Command string `json:"command"`
	WorkDir string `json:"work_dir"`
}

type ExecResponse struct {
	ExitCode int    `json:"exit_code"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
}

type ClientInfo struct {
	WorkDir  string `json:"work_dir"`
	Hostname string `json:"hostname"`
	ID       string `json:"id"`
}

type ErrorPayload struct {
	Message string `json:"message"`
}

func NewMessage(t MsgType, payload interface{}) (*Message, error) {
	var raw json.RawMessage
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		raw = data
	}
	return &Message{
		Type:      t,
		Timestamp: time.Now().UnixMilli(),
		Payload:   raw,
	}, nil
}

func ParsePayload[T any](msg *Message) (*T, error) {
	var v T
	if err := json.Unmarshal(msg.Payload, &v); err != nil {
		return nil, err
	}
	return &v, nil
}

func HashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func HashBytes(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}
