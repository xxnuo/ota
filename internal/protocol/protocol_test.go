package protocol

import (
	"os"
	"testing"
)

func TestNewMessage(t *testing.T) {
	msg, err := NewMessage(MsgPing, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg.Type != MsgPing {
		t.Errorf("expected type %s, got %s", MsgPing, msg.Type)
	}
	if msg.Timestamp == 0 {
		t.Error("timestamp should not be zero")
	}
}

func TestNewMessageWithPayload(t *testing.T) {
	info := &ClientInfo{
		WorkDir:  "/tmp/test",
		Hostname: "testhost",
		ID:       "test-id",
	}
	msg, err := NewMessage(MsgClientInfo, info)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg.Payload == nil {
		t.Error("payload should not be nil")
	}

	parsed, err := ParsePayload[ClientInfo](msg)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if parsed.WorkDir != "/tmp/test" {
		t.Errorf("expected workDir /tmp/test, got %s", parsed.WorkDir)
	}
	if parsed.Hostname != "testhost" {
		t.Errorf("expected hostname testhost, got %s", parsed.Hostname)
	}
}

func TestHashBytes(t *testing.T) {
	h1 := HashBytes([]byte("hello"))
	h2 := HashBytes([]byte("hello"))
	h3 := HashBytes([]byte("world"))

	if h1 != h2 {
		t.Error("same input should produce same hash")
	}
	if h1 == h3 {
		t.Error("different input should produce different hash")
	}
}

func TestHashFile(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "ota-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())

	tmpFile.WriteString("test content")
	tmpFile.Close()

	hash, err := HashFile(tmpFile.Name())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hash == "" {
		t.Error("hash should not be empty")
	}

	expected := HashBytes([]byte("test content"))
	if hash != expected {
		t.Errorf("file hash %s != bytes hash %s", hash, expected)
	}
}

func TestParsePayloadManifest(t *testing.T) {
	manifest := &Manifest{
		WorkDir: "/test",
		Files: []FileInfo{
			{Path: "a.txt", Hash: "abc", Size: 100},
			{Path: "b.txt", Hash: "def", Size: 200},
		},
	}
	msg, _ := NewMessage(MsgManifest, manifest)

	parsed, err := ParsePayload[Manifest](msg)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Files) != 2 {
		t.Errorf("expected 2 files, got %d", len(parsed.Files))
	}
	if parsed.Files[0].Path != "a.txt" {
		t.Errorf("expected path a.txt, got %s", parsed.Files[0].Path)
	}
}

func TestParsePayloadFileRequest(t *testing.T) {
	req := &FileRequest{
		Paths: []string{"a.txt", "b.txt", "dir/c.go"},
	}
	msg, _ := NewMessage(MsgFileRequest, req)

	parsed, err := ParsePayload[FileRequest](msg)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Paths) != 3 {
		t.Errorf("expected 3 paths, got %d", len(parsed.Paths))
	}
}

func TestParsePayloadExecRequest(t *testing.T) {
	req := &ExecRequest{
		Command: "ls -la",
		WorkDir: "/tmp/work",
	}
	msg, _ := NewMessage(MsgExecRequest, req)

	parsed, err := ParsePayload[ExecRequest](msg)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Command != "ls -la" {
		t.Errorf("expected 'ls -la', got '%s'", parsed.Command)
	}
}
